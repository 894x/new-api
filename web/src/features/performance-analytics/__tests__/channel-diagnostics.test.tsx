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
import type { ComponentProps } from 'react'
import { beforeEach, expect, test, vi } from 'vitest'

import { ChannelPerformanceDiagnostics } from '../channel-diagnostics'
import { buildPerformanceChartOption } from '../chart-options'
import type { PerformanceChart } from '../performance-chart'
import type { ChannelAnalyticsData } from '../types'

vi.mock('react-i18next', () => ({
  useTranslation: () => ({
    t: (key: string) => key,
    i18n: { language: 'en', resolvedLanguage: 'en' },
  }),
}))
const {
  getChannelPerformanceAnalytics,
  getHTTPTransportMetrics,
  chartSetOption,
} = vi.hoisted(() => ({
  getChannelPerformanceAnalytics: vi.fn(),
  getHTTPTransportMetrics: vi.fn(),
  chartSetOption: vi.fn(),
}))
vi.mock('../api', () => ({
  getChannelPerformanceAnalytics,
  getHTTPTransportMetrics,
}))
// Exercise the real chart adapter without starting a canvas renderer in jsdom.
vi.mock('../performance-chart', () => ({
  PerformanceChart: (props: ComponentProps<typeof PerformanceChart>) => {
    chartSetOption(
      buildPerformanceChartOption({
        ...props,
        start: 0,
        end: 1000,
        dark: false,
      })
    )
    return <div role='img' aria-label={props.title} />
  },
}))

const latency = {
  avg_ms: 0.125,
  p50_ms: 1,
  p90_ms: 2,
  p95_ms: 3,
  p99_ms: 4,
  max_ms: 0.375,
  sample_count: 2,
}
const data: ChannelAnalyticsData = {
  channel_id: 0,
  scanned_logs: 2,
  truncated: false,
  error_groups: [],
  transport_groups: [],
  effective_start_timestamp: 100,
  effective_end_timestamp: 700,
  summary: {
    ts: 100,
    request_count: 2,
    error_count: 0,
    success_rate: 100,
    active_concurrency: { average: 0.125, maximum: 1 },
    latency: {
      total_ms: latency,
      body_read_ms: latency,
      upstream_queue_ms: latency,
      upload_ms: latency,
      provider_wait_ms: latency,
      downstream_ms: latency,
      acquire_ms: latency,
      write_ms: latency,
      headers_to_first_response_ms: latency,
    },
  },
  series: [],
}
function renderDiagnostics() {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false, gcTime: 0 } },
  })
  return render(
    <QueryClientProvider client={client}>
      <ChannelPerformanceDiagnostics />
    </QueryClientProvider>
  )
}

function upstreamBodySizeCard() {
  const card = screen
    .getByText('Upstream request body size')
    .closest<HTMLElement>('[data-slot=card]')
  if (!card) throw new Error('Upstream body size chart card is missing')
  return card
}
beforeEach(() => {
  getChannelPerformanceAnalytics.mockResolvedValue({ success: true, data })
  getHTTPTransportMetrics.mockResolvedValue({ success: true, data: [] })
})

test.each([3, 6, 12])(
  'queries a rolling %s hour channel window',
  async (hours) => {
    vi.spyOn(Date, 'now').mockReturnValue(1_800_000_000_000)
    const user = userEvent.setup()
    renderDiagnostics()
    await user.click(await screen.findByRole('tab', { name: `${hours} Hours` }))
    await waitFor(() =>
      expect(getChannelPerformanceAnalytics).toHaveBeenLastCalledWith(
        expect.objectContaining({
          start_timestamp: 1_800_000_000 - hours * 3600,
          end_timestamp: 1_800_000_000,
        }),
        expect.any(AbortSignal)
      )
    )
  }
)

test('keeps exact custom bounds on refresh and rejects windows longer than 24 hours', async () => {
  const user = userEvent.setup()
  renderDiagnostics()
  await user.click(await screen.findByRole('button', { name: 'Date Range' }))
  expect(
    screen.queryByRole('button', { name: '7 Days' })
  ).not.toBeInTheDocument()
  expect(screen.getByRole('button', { name: 'Today' })).toBeVisible()
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
    expect(getChannelPerformanceAnalytics).toHaveBeenLastCalledWith(
      expect.objectContaining(expected),
      expect.any(AbortSignal)
    )
  )
  expect(screen.getByRole('button', { name: 'Back to latest' })).toBeVisible()
  await user.click(screen.getByRole('button', { name: 'Refresh' }))
  await waitFor(() =>
    expect(getChannelPerformanceAnalytics).toHaveBeenLastCalledWith(
      expect.objectContaining(expected),
      expect.any(AbortSignal)
    )
  )
  const count = getChannelPerformanceAnalytics.mock.calls.length
  await user.click(screen.getByRole('button', { name: /2026-10-28 09:15/ }))
  fireEvent.change(screen.getByLabelText('End Time'), {
    target: { value: '2026-10-29T12:45' },
  })
  await user.click(screen.getByRole('button', { name: 'Confirm' }))
  expect(screen.getByRole('alert')).toHaveTextContent(
    'Select a valid time range of up to 24 hours.'
  )
  expect(getChannelPerformanceAnalytics).toHaveBeenCalledTimes(count)
})

test('refreshes both queries every 15 seconds and freezes them while paused', async () => {
  vi.useFakeTimers()
  vi.setSystemTime(1_800_000_000_000)
  const view = renderDiagnostics()
  try {
    await act(async () => {
      await vi.advanceTimersByTimeAsync(15_001)
    })
    expect(getChannelPerformanceAnalytics).toHaveBeenLastCalledWith(
      expect.objectContaining({ end_timestamp: 1_800_000_015 }),
      expect.any(AbortSignal)
    )
    fireEvent.click(screen.getByRole('button', { name: 'Pause live updates' }))
    const count = getChannelPerformanceAnalytics.mock.calls.length
    const transportCount = getHTTPTransportMetrics.mock.calls.length
    await act(async () => {
      await vi.advanceTimersByTimeAsync(30_000)
    })
    expect(getChannelPerformanceAnalytics).toHaveBeenCalledTimes(count)
    expect(getHTTPTransportMetrics).toHaveBeenCalledTimes(transportCount)
    fireEvent.click(screen.getByRole('button', { name: 'Back to latest' }))
    await act(async () => {
      await vi.advanceTimersByTimeAsync(1)
    })
    expect(getChannelPerformanceAnalytics).toHaveBeenLastCalledWith(
      expect.objectContaining({ end_timestamp: 1_800_000_045 }),
      expect.any(AbortSignal)
    )
  } finally {
    view.unmount()
    vi.useRealTimers()
  }
})

test('retains same-channel results after refresh failure but never leaks them to a new channel', async () => {
  const user = userEvent.setup()
  renderDiagnostics()
  await screen.findByText('Average total latency')
  getChannelPerformanceAnalytics.mockRejectedValue(new Error('Unavailable'))
  await user.click(screen.getByRole('button', { name: 'Refresh' }))
  await screen.findByText(
    'Performance data could not be refreshed. Showing the last result.'
  )
  expect(screen.getByText('Average total latency')).toBeVisible()
  fireEvent.change(screen.getByRole('textbox', { name: 'Channel ID' }), {
    target: { value: '7' },
  })
  await screen.findByText('Failed to load performance data')
  expect(screen.queryByText('Average total latency')).not.toBeInTheDocument()
  expect(getChannelPerformanceAnalytics).toHaveBeenLastCalledWith(
    expect.objectContaining({ channel_id: 7 }),
    expect.any(AbortSignal)
  )
  getChannelPerformanceAnalytics.mockResolvedValue({
    success: true,
    data: { ...data, channel_id: 7 },
  })
  await user.click(screen.getByRole('button', { name: 'Retry' }))
  await screen.findByText('Average total latency')
})

test('shows all nine stage summaries with fractional averages and measured maxima', async () => {
  renderDiagnostics()
  await screen.findByText('Average total latency')
  const stages = screen.getByRole('group', { name: 'Request-stage latency' })
  expect(stages).toHaveClass('xl:grid-cols-3')
  expect(stages.querySelectorAll('[data-slot=card-title]')).toHaveLength(9)
  expect(stages.querySelectorAll('dl')).toHaveLength(9)
  expect(screen.getAllByText('0.125 ms')).toHaveLength(10)
  expect(screen.getAllByText('0.375 ms')).toHaveLength(10)
  expect(screen.getAllByText('0.13')).toHaveLength(2)
  expect(
    screen.getByRole('group', { name: 'Concurrency and live transport' })
  ).toHaveClass('xl:grid-cols-3')
})

test('shows six upstream body size statistics and curves with gaps for missing buckets and real zeroes', async () => {
  const stats = {
    avg: 1536,
    p50: 1024,
    p90: 2048,
    p95: 3072,
    p99: 4096,
    max: 8192,
    sample_count: 2,
  }
  getChannelPerformanceAnalytics.mockResolvedValue({
    success: true,
    data: {
      ...data,
      summary: { ...data.summary, request_body_bytes: stats },
      series: [
        { ...data.summary, ts: 100 },
        {
          ...data.summary,
          ts: 160,
          request_body_bytes: { ...stats, sample_count: 0 },
        },
        {
          ...data.summary,
          ts: 220,
          request_body_bytes: {
            avg: 0,
            p50: 0,
            p90: 0,
            p95: 0,
            p99: 0,
            max: 0,
            sample_count: 1,
          },
        },
        { ...data.summary, ts: 280, request_body_bytes: stats },
      ],
    },
  })
  renderDiagnostics()
  await screen.findByText('Upstream request body size')
  const card = upstreamBodySizeCard()
  const expected = ['1.5 KiB', '1 KiB', '2 KiB', '3 KiB', '4 KiB', '8 KiB']
  expect(
    [...card.querySelectorAll('dt')].map((element) => element.textContent)
  ).toEqual(['AVG', 'P50', 'P90', 'P95', 'P99', 'MAX'])
  expect(
    [...card.querySelectorAll('dd')].map((element) => element.textContent)
  ).toEqual(expected)
  expect(card.querySelector('dl')).toHaveClass('grid-cols-3', 'sm:grid-cols-6')
  expect(
    within(card).getByRole('img', { name: 'Upstream request body size' })
  ).toBeVisible()
  await waitFor(() => {
    const options = chartSetOption.mock.calls
      .map(([option]) => option)
      .filter(
        (option) => option.aria?.description === 'Upstream request body size'
      )
    const option = options.at(-1)
    expect(
      option?.series.map((series: { name: string; data: number[][] }) => ({
        name: series.name,
        data: series.data,
      }))
    ).toEqual(
      ['AVG', 'P50', 'P90', 'P95', 'P99', 'MAX'].map((name) => ({
        name,
        data: [
          [100_000, null],
          [160_000, null],
          [220_000, 0],
          [
            280_000,
            stats[
              name.toLowerCase() as
                | 'avg'
                | 'p50'
                | 'p90'
                | 'p95'
                | 'p99'
                | 'max'
            ],
          ],
        ],
      }))
    )
    expect(option.tooltip.valueFormatter(1536)).toBe('1.5 KiB (1,536 B)')
  })
})

test('shows unknown upstream body size statistics and an empty chart for historical data without measurements', async () => {
  renderDiagnostics()
  await screen.findByText('Upstream request body size')
  const card = upstreamBodySizeCard()
  expect(
    [...card.querySelectorAll('dd')].map((element) => element.textContent)
  ).toEqual(Array(6).fill('—'))
  expect(within(card).getByText('No performance data available')).toBeVisible()
  expect(within(card).queryByRole('img')).not.toBeInTheDocument()
})

test('updates upstream body statistics and curves when the channel or time range changes', async () => {
  getChannelPerformanceAnalytics.mockImplementation(async (params) => {
    let size = 1024
    if (params.channel_id === 7) {
      size = params.end_timestamp - params.start_timestamp > 3600 ? 4096 : 2048
    }
    const stats = {
      avg: size,
      p50: size,
      p90: size,
      p95: size,
      p99: size,
      max: size,
      sample_count: 1,
    }
    const point = { ...data.summary, request_body_bytes: stats }
    return { success: true, data: { ...data, summary: point, series: [point] } }
  })
  const user = userEvent.setup()
  renderDiagnostics()
  await screen.findByText('Upstream request body size')
  const card = upstreamBodySizeCard()
  expect(within(card).getAllByText('1 KiB')).toHaveLength(6)
  fireEvent.change(screen.getByRole('textbox', { name: 'Channel ID' }), {
    target: { value: '7' },
  })
  await waitFor(() =>
    expect(within(upstreamBodySizeCard()).getAllByText('2 KiB')).toHaveLength(6)
  )
  await user.click(screen.getByRole('tab', { name: '3 Hours' }))
  await waitFor(() =>
    expect(within(upstreamBodySizeCard()).getAllByText('4 KiB')).toHaveLength(6)
  )
  expect(getChannelPerformanceAnalytics).toHaveBeenLastCalledWith(
    expect.objectContaining({ channel_id: 7 }),
    expect.any(AbortSignal)
  )
  await waitFor(() => {
    const option = chartSetOption.mock.calls
      .map(([value]) => value)
      .reverse()
      .find((value) => value.aria?.description === 'Upstream request body size')
    expect(option?.series[0].data).toEqual([[100_000, 4096]])
  })
})
