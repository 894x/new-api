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
import { act, fireEvent, render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { beforeEach, expect, test, vi } from 'vitest'

import { ChannelPerformanceDiagnostics } from '../channel-diagnostics'
import type { ChannelAnalyticsData } from '../types'

vi.mock('react-i18next', () => ({
  useTranslation: () => ({
    t: (key: string) => key,
    i18n: { language: 'en', resolvedLanguage: 'en' },
  }),
}))
const { getChannelPerformanceAnalytics, getHTTPTransportMetrics } = vi.hoisted(
  () => ({
    getChannelPerformanceAnalytics: vi.fn(),
    getHTTPTransportMetrics: vi.fn(),
  })
)
vi.mock('../api', () => ({
  getChannelPerformanceAnalytics,
  getHTTPTransportMetrics,
}))
// jsdom has no canvas; exercise the real chart adapter separately in series.test.
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
