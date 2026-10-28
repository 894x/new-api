/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import {
  act,
  fireEvent,
  render,
  screen,
  waitFor,
  within,
} from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { beforeEach, expect, test, vi } from 'vitest'

import { PerformanceAnalytics } from '../index'
import type {
  ChannelAnalyticsData,
  PerformanceAnalyticsData,
  PerformanceAnalyticsOptions,
  PerformanceAnalyticsResponse,
} from '../types'

vi.mock('react-i18next', () => ({
  useTranslation: () => ({
    t: (key: string) => key,
    i18n: { language: 'en', resolvedLanguage: 'en' },
  }),
}))

vi.mock('../echarts-runtime', () => ({
  init: () => ({
    setOption: vi.fn(),
    on: vi.fn(),
    resize: vi.fn(),
    dispatchAction: vi.fn(),
    dispose: vi.fn(),
  }),
  joinChartGroup: () => vi.fn(),
}))

const {
  getPerformanceAnalytics,
  getPerformanceAnalyticsOptions,
  getChannelPerformanceAnalytics,
  getHTTPTransportMetrics,
} = vi.hoisted(() => ({
  getPerformanceAnalytics: vi.fn(),
  getPerformanceAnalyticsOptions: vi.fn(),
  getChannelPerformanceAnalytics: vi.fn(),
  getHTTPTransportMetrics: vi.fn(),
}))

vi.mock('../api', () => ({
  getPerformanceAnalytics,
  getPerformanceAnalyticsOptions,
  getChannelPerformanceAnalytics,
  getHTTPTransportMetrics,
}))

const allOptions: PerformanceAnalyticsOptions = {
  models: ['gpt-test'],
  channels: [
    { id: 7, name: 'Primary channel' },
    { id: 8, name: 'Backup channel' },
  ],
  users: [
    { id: 1, username: 'admin' },
    { id: 2, username: 'alice' },
  ],
  tokens: [],
}

const emptyAnalytics: PerformanceAnalyticsData = {
  model_name: 'gpt-test',
  scanned_logs: 0,
  truncated: false,
  error_groups: [],
  effective_start_timestamp: 100,
  effective_end_timestamp: 200,
  summary: {
    request_count: 0,
    error_count: 0,
    success_rate: 0,
    rpm: 0,
    tpm: 0,
    cache_hit_rate: 0,
    ttft: { p50_ms: 0, p90_ms: 0, p99_ms: 0, sample_count: 0 },
    tpot: { p50_ms: 0, p90_ms: 0, p99_ms: 0, sample_count: 0 },
  },
  series: [],
}

const emptyLatency = { p50_ms: 0, p95_ms: 0, p99_ms: 0, sample_count: 0 }
const emptyChannelAnalytics: ChannelAnalyticsData = {
  channel_id: 0,
  scanned_logs: 0,
  truncated: false,
  error_groups: [],
  transport_groups: [],
  effective_start_timestamp: 100,
  effective_end_timestamp: 200,
  summary: {
    ts: 100,
    request_count: 0,
    error_count: 0,
    success_rate: 0,
    active_concurrency: { average: 0, maximum: 0 },
    latency: {
      acquire_ms: emptyLatency,
      write_ms: emptyLatency,
      total_ms: emptyLatency,
      body_read_ms: emptyLatency,
      upstream_queue_ms: emptyLatency,
      upload_ms: emptyLatency,
      provider_wait_ms: emptyLatency,
      headers_to_first_response_ms: emptyLatency,
      downstream_ms: emptyLatency,
    },
  },
  series: [],
}

function response<T>(data: T): PerformanceAnalyticsResponse<T> {
  return { success: true, data }
}

function renderAnalytics(standalone = false, isAdmin = true) {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false, gcTime: 0 } },
  })
  return render(
    <QueryClientProvider client={queryClient}>
      <PerformanceAnalytics isAdmin={isAdmin} standalone={standalone} />
    </QueryClientProvider>
  )
}

beforeEach(() => {
  getPerformanceAnalytics.mockResolvedValue(response(emptyAnalytics))
  getChannelPerformanceAnalytics.mockResolvedValue(
    response(emptyChannelAnalytics)
  )
  getHTTPTransportMetrics.mockResolvedValue(response([]))
  getPerformanceAnalyticsOptions.mockImplementation(
    (_: boolean, userId?: number) => {
      if (!userId) return Promise.resolve(response(allOptions))
      return new Promise(() => {})
    }
  )
})

test.each([
  { standalone: false, hours: 3 },
  { standalone: false, hours: 6 },
  { standalone: false, hours: 12 },
  { standalone: true, hours: 3 },
  { standalone: true, hours: 6 },
  { standalone: true, hours: 12 },
])(
  'queries the last $hours hours from the shortcut with standalone=$standalone',
  async ({ standalone, hours }) => {
    vi.spyOn(Date, 'now').mockReturnValue(1_800_000_000_000)
    getPerformanceAnalyticsOptions.mockResolvedValue(response(allOptions))
    const user = userEvent.setup()
    renderAnalytics(standalone)
    const shortcut = await screen.findByRole('tab', { name: `${hours} Hours` })
    await user.click(shortcut)
    await waitFor(() =>
      expect(getPerformanceAnalytics).toHaveBeenLastCalledWith(
        expect.objectContaining({
          start_timestamp: 1_800_000_000 - hours * 3600,
          end_timestamp: 1_800_000_000,
        }),
        true,
        expect.any(AbortSignal)
      )
    )
    expect(shortcut).toHaveAttribute('aria-selected', 'true')
  }
)

test('queries an exact custom interval, keeps it on refresh, and returns to the latest preset', async () => {
  const now = vi.spyOn(Date, 'now').mockReturnValue(1_800_000_000_000)
  const user = userEvent.setup()
  renderAnalytics()
  await user.click(await screen.findByRole('button', { name: 'Date Range' }))
  fireEvent.change(screen.getByLabelText('Start Time'), {
    target: { value: '2026-10-28T09:15' },
  })
  fireEvent.change(screen.getByLabelText('End Time'), {
    target: { value: '2026-10-28T12:45' },
  })
  await user.click(screen.getByRole('button', { name: 'Confirm' }))
  const expected = {
    start_timestamp: new Date('2026-10-28T09:15').getTime() / 1000,
    end_timestamp: new Date('2026-10-28T12:45').getTime() / 1000,
  }
  await waitFor(() =>
    expect(getPerformanceAnalytics).toHaveBeenLastCalledWith(
      expect.objectContaining(expected),
      true,
      expect.any(AbortSignal)
    )
  )
  expect(screen.getByRole('button', { name: 'Back to latest' })).toBeVisible()
  expect(
    screen
      .getAllByRole('tab')
      .every((tab) => tab.getAttribute('aria-selected') === 'false')
  ).toBe(true)
  now.mockReturnValue(1_800_000_060_000)
  await user.click(screen.getByRole('button', { name: 'Refresh' }))
  await waitFor(() =>
    expect(getPerformanceAnalytics).toHaveBeenLastCalledWith(
      expect.objectContaining(expected),
      true,
      expect.any(AbortSignal)
    )
  )
  await user.click(screen.getByRole('button', { name: 'Back to latest' }))
  await waitFor(() =>
    expect(getPerformanceAnalytics).toHaveBeenLastCalledWith(
      expect.objectContaining({
        start_timestamp: 1_799_913_660,
        end_timestamp: 1_800_000_060,
      }),
      true,
      expect.any(AbortSignal)
    )
  )
  expect(screen.getByRole('tab', { name: '1 Day' })).toHaveAttribute(
    'aria-selected',
    'true'
  )
})

test.each([
  { start: '', end: '2026-10-28T12:00' },
  { start: '2026-10-28T12:00', end: '2026-10-28T09:00' },
  { start: '2026-10-28T12:00', end: '2026-10-28T12:00' },
  { start: '2026-09-01T09:00', end: '2026-10-28T12:00' },
])(
  'rejects an invalid custom interval $start to $end without changing the query',
  async ({ start, end }) => {
    const user = userEvent.setup()
    renderAnalytics()
    await user.click(await screen.findByRole('button', { name: 'Date Range' }))
    const requests = getPerformanceAnalytics.mock.calls.length
    fireEvent.change(screen.getByLabelText('Start Time'), {
      target: { value: start },
    })
    fireEvent.change(screen.getByLabelText('End Time'), {
      target: { value: end },
    })
    await user.click(screen.getByRole('button', { name: 'Confirm' }))
    expect(screen.getByRole('alert')).toHaveTextContent(
      'Select a valid time range of up to 30 days.'
    )
    expect(getPerformanceAnalytics).toHaveBeenCalledTimes(requests)
    expect(screen.getByRole('tab', { name: '1 Day' })).toHaveAttribute(
      'aria-selected',
      'true'
    )
  }
)

test('automatically refreshes the main panel, freezes on pause, and resumes at the latest window', async () => {
  vi.useFakeTimers()
  vi.setSystemTime(1_800_000_000_000)
  const view = renderAnalytics()
  try {
    await act(async () => {
      await vi.advanceTimersByTimeAsync(1)
    })
    await act(async () => {
      await vi.advanceTimersByTimeAsync(15_000)
    })
    expect(getPerformanceAnalytics).toHaveBeenLastCalledWith(
      expect.objectContaining({ end_timestamp: 1_800_000_015 }),
      true,
      expect.any(AbortSignal)
    )
    fireEvent.click(screen.getByRole('button', { name: 'Pause live updates' }))
    expect(screen.getByText('Paused · exploring history')).toBeVisible()
    const requests = getPerformanceAnalytics.mock.calls.length
    await act(async () => {
      await vi.advanceTimersByTimeAsync(30_000)
    })
    expect(getPerformanceAnalytics).toHaveBeenCalledTimes(requests)
    fireEvent.click(screen.getByRole('button', { name: 'Back to latest' }))
    await act(async () => {
      await vi.advanceTimersByTimeAsync(1)
    })
    expect(screen.getByText('Live · every 15 seconds')).toBeVisible()
    expect(getPerformanceAnalytics).toHaveBeenLastCalledWith(
      expect.objectContaining({ end_timestamp: 1_800_000_045 }),
      true,
      expect.any(AbortSignal)
    )
  } finally {
    view.unmount()
    vi.useRealTimers()
  }
})

test('retains the last statistics and filters after a refresh fails', async () => {
  getPerformanceAnalytics.mockResolvedValue(
    response({
      ...emptyAnalytics,
      summary: { ...emptyAnalytics.summary, request_count: 1 },
    })
  )
  const user = userEvent.setup()
  renderAnalytics()
  await screen.findByText('Average RPM')
  getPerformanceAnalytics.mockRejectedValue({ response: { status: 500 } })
  await user.click(screen.getAllByRole('button', { name: 'Refresh' })[0])
  await screen.findByText(
    'Performance data could not be refreshed. Showing the last result.'
  )
  expect(screen.getByText('Average RPM')).toBeVisible()
  expect(screen.getByRole('combobox', { name: /User/ })).toBeVisible()
})

test('retries an initial analytics failure without losing the filters', async () => {
  getPerformanceAnalytics.mockRejectedValue({ response: { status: 500 } })
  const user = userEvent.setup()
  renderAnalytics()
  await screen.findByRole('button', { name: 'Retry' })
  getPerformanceAnalytics.mockResolvedValue(
    response({
      ...emptyAnalytics,
      summary: { ...emptyAnalytics.summary, request_count: 1 },
    })
  )
  await user.click(screen.getByRole('button', { name: 'Retry' }))
  await screen.findByText('Average RPM')
  expect(screen.getByRole('combobox', { name: /Model/ })).toHaveTextContent(
    'gpt-test'
  )
})

test('shows missing measurements as unknown while retaining explicitly measured zeroes', async () => {
  getPerformanceAnalytics.mockResolvedValue(
    response({
      ...emptyAnalytics,
      summary: {
        ...emptyAnalytics.summary,
        request_count: 1,
        request_body_samples: 0,
        request_body_avg_bytes: null,
        cache_read_samples: 0,
        cache_write_samples: 0,
      },
    })
  )
  const user = userEvent.setup()
  renderAnalytics()
  await screen.findByText('Average RPM')
  expect(screen.getAllByText('Unknown')).toHaveLength(5)
  getPerformanceAnalytics.mockResolvedValue(
    response({
      ...emptyAnalytics,
      summary: {
        ...emptyAnalytics.summary,
        request_count: 1,
        request_body_samples: 1,
        request_body_avg_bytes: 0,
        request_body_max_bytes: 0,
        cache_read_samples: 1,
        cache_read_tokens: 0,
        cache_write_samples: 1,
        cache_write_tokens: 0,
      },
    })
  )
  await user.click(screen.getAllByRole('button', { name: 'Refresh' })[0])
  await waitFor(() =>
    expect(screen.queryByText('Unknown')).not.toBeInTheDocument()
  )
  expect(screen.getAllByText('0 B')).toHaveLength(3)
})

test('groups the eight charts and keeps six summary statistics in all five percentile headers', async () => {
  getPerformanceAnalytics.mockResolvedValue(
    response({
      ...emptyAnalytics,
      summary: {
        ...emptyAnalytics.summary,
        request_count: 1,
        ttft: {
          p50_ms: 1000,
          p90_ms: 2000,
          p95_ms: 3000,
          p99_ms: 4000,
          max_ms: 4500,
          sample_count: 1,
        },
        tpot: {
          p50_ms: 10,
          p90_ms: 20,
          p95_ms: 30,
          p99_ms: 40,
          max_ms: null,
          sample_count: 1,
        },
      },
    })
  )
  renderAnalytics()
  await screen.findByText('Average RPM')
  expect(screen.getAllByText('P95')).toHaveLength(5)
  expect(screen.getAllByText('MAX')).toHaveLength(5)
  expect(screen.getByText('3 s')).toBeInTheDocument()
  expect(screen.getByText('4.5 s')).toBeInTheDocument()
  expect(screen.getAllByText('AVG')).toHaveLength(5)
  const rows = [
    ['Latency and cache performance', ['TTFT', 'TPOT', 'Cache hit rate']],
    ['Request payload and cache writes', ['Request body size', 'Cache Write']],
    ['Throughput and errors', ['RPM', 'TPM', 'Errors']],
  ] as const
  for (const [name, titles] of rows) {
    const row = screen.getByRole('group', { name })
    expect(row).toHaveClass('xl:grid-cols-3')
    expect(
      [...row.querySelectorAll('[data-slot=card-title]')].map(
        (node) => node.textContent
      )
    ).toEqual(titles)
    for (const description of row.querySelectorAll(
      '[data-slot=card-description]'
    )) {
      expect(description).toHaveClass('text-xs')
    }
  }

  for (const value of screen.getAllByText('P95')) {
    expect(value.closest('[data-slot=card-header]')).not.toBeNull()
  }
})

test('keeps the first selected user while scoped options are loading', async () => {
  const user = userEvent.setup()
  renderAnalytics()

  const userSelect = await screen.findByRole('combobox', { name: /User/ })
  await user.click(userSelect)
  await user.click(await screen.findByRole('option', { name: 'alice' }))

  await waitFor(() => expect(userSelect).toHaveTextContent('alice'))
  expect(getPerformanceAnalyticsOptions).toHaveBeenCalledWith(
    true,
    2,
    expect.any(AbortSignal)
  )
  await waitFor(() =>
    expect(getPerformanceAnalytics).toHaveBeenCalledWith(
      expect.objectContaining({ user_id: 2 }),
      true,
      expect.any(AbortSignal)
    )
  )
})

test('offers a one-hour range and sends an hour-wide query', async () => {
  vi.spyOn(Date, 'now').mockReturnValue(1_800_000_000_000)
  const user = userEvent.setup()
  renderAnalytics()

  const ranges = (await screen.findAllByRole('tablist')).find((list) =>
    within(list).queryByRole('tab', { name: '29 Days' })
  )
  expect(ranges).toBeDefined()
  if (!ranges) throw new Error('Performance percentile ranges are missing')
  await user.click(within(ranges).getByRole('tab', { name: '1 Hour' }))

  await waitFor(() =>
    expect(getPerformanceAnalytics).toHaveBeenCalledWith(
      expect.objectContaining({
        start_timestamp: 1_799_996_400,
        end_timestamp: 1_800_000_000,
      }),
      true,
      expect.any(AbortSignal)
    )
  )
})

test('moves the monitoring window forward on each automatic refresh', async () => {
  vi.useFakeTimers()
  vi.setSystemTime(1_800_000_000_000)
  getPerformanceAnalyticsOptions.mockResolvedValue(response(allOptions))
  const view = renderAnalytics(true)

  try {
    await act(async () => {
      await vi.advanceTimersByTimeAsync(1)
    })
    expect(getPerformanceAnalytics).toHaveBeenCalledWith(
      expect.objectContaining({
        user_id: 1,
        start_timestamp: 1_799_999_400,
        end_timestamp: 1_800_000_000,
      }),
      true,
      expect.any(AbortSignal)
    )

    await act(async () => {
      await vi.advanceTimersByTimeAsync(15_000)
    })
    expect(getPerformanceAnalytics).toHaveBeenLastCalledWith(
      expect.objectContaining({
        user_id: 1,
        start_timestamp: 1_799_999_415,
        end_timestamp: 1_800_000_015,
      }),
      true,
      expect.any(AbortSignal)
    )
  } finally {
    view.unmount()
    vi.useRealTimers()
  }
})

test('admin channel selection composes with the other filters and can return to all channels', async () => {
  const user = userEvent.setup()
  renderAnalytics()
  const channel = await screen.findByRole('combobox', { name: /Channel/ })
  await user.click(channel)
  await user.click(
    await screen.findByRole('option', { name: 'Primary channel (#7)' })
  )
  await waitFor(() =>
    expect(getPerformanceAnalytics).toHaveBeenLastCalledWith(
      expect.objectContaining({ channel_id: 7, model: 'gpt-test' }),
      true,
      expect.any(AbortSignal)
    )
  )
  await user.click(channel)
  await user.click(await screen.findByRole('option', { name: 'All channels' }))
  await waitFor(() =>
    expect(getPerformanceAnalytics.mock.lastCall?.[0]).not.toHaveProperty(
      'channel_id'
    )
  )
})

test('ordinary users keep their self performance view without channel or user selectors', async () => {
  getPerformanceAnalytics.mockResolvedValue(
    response({
      ...emptyAnalytics,
      summary: { ...emptyAnalytics.summary, request_count: 1 },
    })
  )
  renderAnalytics(false, false)
  await screen.findByText('Average RPM')
  expect(
    screen.queryByRole('combobox', { name: /Channel/ })
  ).not.toBeInTheDocument()
  expect(
    screen.queryByRole('combobox', { name: /User/ })
  ).not.toBeInTheDocument()
  expect(getPerformanceAnalyticsOptions).toHaveBeenCalledWith(
    false,
    undefined,
    expect.any(AbortSignal)
  )
  expect(getPerformanceAnalytics).toHaveBeenLastCalledWith(
    expect.not.objectContaining({ channel_id: expect.anything() }),
    false,
    expect.any(AbortSignal)
  )
})

test('does not display all-channel statistics when a newly selected channel fails to load', async () => {
  getPerformanceAnalytics.mockResolvedValue(
    response({
      ...emptyAnalytics,
      summary: { ...emptyAnalytics.summary, request_count: 1 },
    })
  )
  const user = userEvent.setup()
  renderAnalytics()
  await screen.findByText('Average RPM')
  getPerformanceAnalytics.mockRejectedValue({ response: { status: 500 } })
  await user.click(screen.getByRole('combobox', { name: /Channel/ }))
  await user.click(
    await screen.findByRole('option', { name: 'Primary channel (#7)' })
  )
  await screen.findByText('Failed to load performance data')
  expect(screen.queryByText('Average RPM')).not.toBeInTheDocument()
})
