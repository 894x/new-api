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
  cleanup,
  render,
  screen,
  waitFor,
  within,
} from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, beforeEach, expect, test, vi } from 'vitest'

import { api } from '@/lib/api'
import dayjs from '@/lib/dayjs'
import { useAuthStore } from '@/stores/auth-store'

import { GrossProfitAnalysis } from '..'
import type { GrossProfitReport } from '../types'

let client: QueryClient
let report: GrossProfitReport

beforeEach(() => {
  client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  useAuthStore.getState().auth.setUser({ id: 1, username: 'root', role: 100 })
  report = {
    timezone: 'Asia/Shanghai',
    aggregation: { backfill_complete: true, pending_events: 0, updated_at: 0 },
    summary: {
      known_requests: 2,
      known_revenue_usd: 8,
      cost_usd: 3,
      profit_usd: 5,
      profit_margin: 0.625,
      revenue_coverage: 0.8,
      unknown_requests: 1,
      unknown_revenue_usd: 2,
    },
    channels: [
      {
        channel_id: 1,
        channel_name: 'Known channel',
        known_requests: 2,
        known_revenue_usd: 8,
        cost_usd: 3,
        profit_usd: 5,
        profit_margin: 0.625,
        revenue_coverage: 1,
        unknown_requests: 0,
        unknown_revenue_usd: 0,
      },
    ],
    unknown: [
      {
        channel_id: 2,
        channel_name: 'Unconfigured channel',
        reason: 'discount_unconfigured',
        requests: 1,
        revenue_usd: 2,
      },
    ],
    series: [],
  }
  vi.spyOn(api, 'get').mockImplementation(async () => ({
    data: { success: true, data: report },
  }))
})

afterEach(() => {
  cleanup()
  client.clear()
  useAuthStore.setState(useAuthStore.getInitialState(), true)
})

function renderAnalysis() {
  return render(
    <QueryClientProvider client={client}>
      <GrossProfitAnalysis />
    </QueryClientProvider>
  )
}

test('root sees known-cost profit separately from unconfigured calls', async () => {
  renderAnalysis()
  await screen.findByText('Known channel')
  const tables = screen.getAllByRole('table')
  expect(within(tables[0]).getByText('Known channel')).toBeVisible()
  expect(within(tables[0]).queryByText('Unconfigured channel')).toBeNull()
  expect(
    within(tables[1]).getByText('Channel discount not configured')
  ).toBeVisible()
  expect(within(tables[1]).getByText('Unknown')).toBeVisible()
  expect(screen.getByText('80%')).toBeVisible()
})

test('ordinary administrators neither fetch nor render financial data', async () => {
  useAuthStore.getState().auth.setUser({ id: 2, username: 'admin', role: 10 })
  const view = renderAnalysis()
  await act(async () => {})
  expect(api.get).not.toHaveBeenCalled()
  expect(view.container).toBeEmptyDOMElement()
})

test('changing to thirty days requests and displays the selected period', async () => {
  renderAnalysis()
  await screen.findByText('Known channel')
  await userEvent.click(screen.getByRole('button', { name: 'Last 30 days' }))
  await waitFor(() => expect(api.get).toHaveBeenCalledTimes(2))
  const config = vi.mocked(api.get).mock.calls[1]?.[1]
  expect(
    dayjs(config?.params.end_date).diff(dayjs(config?.params.start_date), 'day')
  ).toBe(30)
  expect(screen.getByText('Known channel')).toBeVisible()
})

test('failed query shows retry and a successful retry restores results', async () => {
  vi.mocked(api.get).mockRejectedValueOnce(new Error('Report unavailable'))
  renderAnalysis()
  await screen.findByText('Report unavailable')
  await userEvent.click(screen.getByRole('button', { name: 'Retry' }))
  expect(await screen.findByText('Known channel')).toBeVisible()
})

test('clearing a required date disables apply until the range is restored', async () => {
  renderAnalysis()
  await screen.findByText('Known channel')
  await userEvent.click(screen.getAllByRole('button', { name: 'Clear' })[0])
  expect(screen.getByRole('button', { name: 'Apply' })).toBeDisabled()
  expect(screen.getByRole('alert')).toBeVisible()
  await userEvent.click(screen.getByRole('button', { name: 'Last 7 days' }))
  await waitFor(() =>
    expect(screen.getByRole('button', { name: 'Apply' })).toBeEnabled()
  )
})

test('empty period keeps unknown margins unset and shows both empty states', async () => {
  report = {
    timezone: 'Asia/Shanghai',
    aggregation: { backfill_complete: true, pending_events: 0, updated_at: 0 },
    summary: {
      known_requests: 0,
      known_revenue_usd: 0,
      cost_usd: 0,
      profit_usd: 0,
      profit_margin: null,
      revenue_coverage: null,
      unknown_requests: 0,
      unknown_revenue_usd: 0,
    },
    channels: [],
    unknown: [],
    series: [],
  }
  renderAnalysis()
  expect(
    await screen.findByText('No calls with a known cost in this period')
  ).toBeVisible()
  expect(screen.getByText('No unknown-cost calls in this period')).toBeVisible()
  expect(screen.queryByText('NaN')).toBeNull()
})

test('ninety-day range uses daily summaries without a thirty-one-day limit', async () => {
  renderAnalysis()
  await screen.findByText('Known channel')
  await userEvent.click(screen.getByRole('button', { name: 'Last 90 days' }))
  await waitFor(() => expect(api.get).toHaveBeenCalledTimes(2))
  const config = vi.mocked(api.get).mock.calls[1]?.[1]
  expect(
    dayjs(config?.params.end_date).diff(dayjs(config?.params.start_date), 'day')
  ).toBe(90)
  expect(screen.queryByRole('alert')).toBeNull()
  expect(document.querySelector('input[type=time]')).toBeNull()
})
test('pending aggregation is visible and goes away after refreshed totals are complete', async () => {
  report.aggregation = {
    backfill_complete: false,
    pending_events: 3,
    updated_at: 0,
  }
  renderAnalysis()
  await screen.findByText(
    'Historical aggregation is in progress. Totals may be incomplete.'
  )
  expect(screen.getByText('3 calls are pending aggregation.')).toBeVisible()
  report = {
    ...report,
    aggregation: { backfill_complete: true, pending_events: 0, updated_at: 1 },
  }
  await userEvent.click(screen.getByRole('button', { name: 'Apply' }))
  await waitFor(() =>
    expect(
      screen.queryByText(
        'Historical aggregation is in progress. Totals may be incomplete.'
      )
    ).toBeNull()
  )
})
