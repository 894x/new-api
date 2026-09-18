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
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { beforeEach, expect, test, vi } from 'vitest'

import { PerformanceAnalytics } from '../index'
import type {
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

const { getPerformanceAnalytics, getPerformanceAnalyticsOptions } = vi.hoisted(
  () => ({
    getPerformanceAnalytics: vi.fn(),
    getPerformanceAnalyticsOptions: vi.fn(),
  })
)

vi.mock('../api', () => ({
  getPerformanceAnalytics,
  getPerformanceAnalyticsOptions,
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

function response<T>(data: T): PerformanceAnalyticsResponse<T> {
  return { success: true, data }
}

function renderAnalytics() {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  return render(
    <QueryClientProvider client={queryClient}>
      <PerformanceAnalytics isAdmin />
    </QueryClientProvider>
  )
}

beforeEach(() => {
  getPerformanceAnalytics.mockResolvedValue(response(emptyAnalytics))
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

  await user.click(await screen.findByRole('tab', { name: '1 Hour' }))

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
