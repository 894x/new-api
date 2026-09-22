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
import { render, screen, waitFor, within } from '@testing-library/react'
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

vi.mock('@visactor/react-vchart', () => ({ VChart: () => null }))

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
  users: [
    { id: 1, username: 'admin' },
    { id: 2, username: 'alice' },
  ],
  tokens: [],
}

const emptyAnalytics: PerformanceAnalyticsData = {
  model_name: 'gpt-test',
  effective_start_timestamp: 100,
  effective_end_timestamp: 200,
  summary: {
    request_count: 0,
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
  transport_groups: [],
  effective_start_timestamp: 100,
  effective_end_timestamp: 200,
  summary: {
    ts: 100,
    request_count: 0,
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

function renderAnalytics() {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false, gcTime: 0 } },
  })
  return render(
    <QueryClientProvider client={queryClient}>
      <PerformanceAnalytics isAdmin />
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

test('keeps the first selected user while scoped options are loading', async () => {
  const user = userEvent.setup()
  renderAnalytics()

  const userSelect = await screen.findByRole('combobox', { name: /User/ })
  await user.click(userSelect)
  await user.click(await screen.findByRole('option', { name: 'alice' }))

  await waitFor(() => expect(userSelect).toHaveTextContent('alice'))
  expect(getPerformanceAnalyticsOptions).toHaveBeenCalledWith(true, 2)
  await waitFor(() =>
    expect(getPerformanceAnalytics).toHaveBeenCalledWith(
      expect.objectContaining({ user_id: 2 }),
      true
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
      true
    )
  )
})
