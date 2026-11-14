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
import i18next from 'i18next'
import { afterEach, beforeEach, expect, it, vi } from 'vitest'

import { api } from '@/lib/api'
import { useAuthStore } from '@/stores/auth-store'

import type { DashboardFilters, StatusCodeStats } from '../../../types'
import { StatusCodeDistribution } from '../status-code-distribution'

let client: QueryClient
const filters: DashboardFilters = {
  start_timestamp: new Date(100_000),
  end_timestamp: new Date(200_000),
  username: 'alice',
}
const distribution: StatusCodeStats = {
  total: 10000,
  recorded: 7000,
  unrecorded: 3000,
  items: [
    { status_code: 200, count: 4000 },
    { status_code: 429, count: 2000 },
    { status_code: 500, count: 1000 },
  ],
}

function Fixture(props: { filters?: DashboardFilters }) {
  return (
    <QueryClientProvider client={client}>
      <StatusCodeDistribution filters={props.filters ?? filters} />
    </QueryClientProvider>
  )
}

beforeEach(async () => {
  client = new QueryClient({
    defaultOptions: { queries: { retry: false, gcTime: 0 } },
  })
  useAuthStore.getState().auth.setUser({ id: 1, username: 'admin', role: 10 })
  await i18next.changeLanguage('en')
  vi.spyOn(api, 'get').mockResolvedValue({
    data: { success: true, data: distribution },
  })
})

afterEach(async () => {
  cleanup()
  client.clear()
  vi.restoreAllMocks()
  useAuthStore.getState().auth.setUser(null)
  await i18next.changeLanguage('en')
  for (const language of ['zhCN', 'zhTW', 'fr', 'ru', 'ja', 'vi']) {
    i18next.removeResourceBundle(language, 'translation')
  }
})

it('shows exact status counts and percentages while keeping missing statuses separate', async () => {
  render(<Fixture />)
  const table = await screen.findByRole('table', {
    name: 'HTTP status code distribution',
  })
  const recorded = within(table).getByRole('row', { name: /HTTP 200/ })
  expect(recorded).toHaveTextContent('4,000')
  expect(recorded).toHaveTextContent('40%')
  const missing = within(table).getByRole('row', { name: /Not recorded/ })
  expect(missing).toHaveTextContent('3,000')
  expect(missing).toHaveTextContent('30%')
  expect(api.get).toHaveBeenCalledWith(
    '/api/log/status_code_stats',
    expect.objectContaining({
      params: { start_timestamp: 100, end_timestamp: 200, username: 'alice' },
    })
  )
  expect(
    screen.getByRole('region', {
      name: 'HTTP status code distribution — Bar Chart',
    })
  ).toBeVisible()
})

it('switches between pie and bar charts without changing the reported counts', async () => {
  render(<Fixture />)
  await screen.findByRole('table')
  const user = userEvent.setup()
  const pie = screen.getByRole('button', { name: 'Pie Chart' })
  const bar = screen.getByRole('button', { name: 'Bar Chart' })
  await user.click(pie)
  expect(pie).toHaveAttribute('aria-pressed', 'true')
  expect(
    screen.getByRole('region', {
      name: 'HTTP status code distribution — Pie Chart',
    })
  ).toBeVisible()
  bar.focus()
  await user.keyboard(' ')
  expect(bar).toHaveAttribute('aria-pressed', 'true')
  expect(pie).toHaveAttribute('aria-pressed', 'false')
  expect(
    within(screen.getByRole('table')).getByRole('row', { name: /HTTP 429/ })
  ).toHaveTextContent('2,000')
})

it('replaces the old distribution when the dashboard time range and user change', async () => {
  const view = render(<Fixture />)
  await screen.findByRole('table')
  let finish: ((value: unknown) => void) | undefined
  vi.mocked(api.get).mockImplementationOnce(
    () =>
      new Promise((resolve) => {
        finish = resolve
      })
  )
  view.rerender(
    <Fixture
      filters={{
        start_timestamp: new Date(300_000),
        end_timestamp: new Date(400_000),
        username: 'bob',
      }}
    />
  )
  expect(screen.queryByRole('table')).not.toBeInTheDocument()
  await waitFor(() => expect(finish).toBeDefined())
  await act(async () =>
    finish?.({
      data: {
        success: true,
        data: {
          items: [{ status_code: 503, count: 2 }],
          total: 2,
          recorded: 2,
          unrecorded: 0,
        },
      },
    })
  )
  const table = await screen.findByRole('table')
  expect(within(table).getByText('HTTP 503')).toBeVisible()
  expect(within(table).queryByText('HTTP 200')).not.toBeInTheDocument()
  expect(api.get).toHaveBeenLastCalledWith(
    '/api/log/status_code_stats',
    expect.objectContaining({
      params: { start_timestamp: 300, end_timestamp: 400, username: 'bob' },
    })
  )
})

it('shows request failures with Retry instead of displaying an empty distribution', async () => {
  vi.mocked(api.get).mockResolvedValueOnce({
    data: { success: false, message: 'Database unavailable' },
  })
  render(<Fixture />)
  await screen.findByText('Failed to load status code distribution')
  expect(screen.queryByText('No data available')).not.toBeInTheDocument()
  await userEvent.click(screen.getByRole('button', { name: 'Retry' }))
  expect(await screen.findByRole('table')).toBeVisible()
})

it('shows an empty state when the selected period contains no HTTP logs', async () => {
  vi.mocked(api.get).mockResolvedValueOnce({
    data: {
      success: true,
      data: { items: [], total: 0, recorded: 0, unrecorded: 0 },
    },
  })
  render(<Fixture />)
  expect(await screen.findByText('No data available')).toBeVisible()
  expect(screen.queryByRole('table')).not.toBeInTheDocument()
  expect(screen.getByRole('button', { name: 'Refresh' })).toBeEnabled()
})

it('keeps an entirely unrecorded period visible without synthesizing HTTP 200', async () => {
  vi.mocked(api.get).mockResolvedValueOnce({
    data: {
      success: true,
      data: { items: [], total: 5, recorded: 0, unrecorded: 5 },
    },
  })
  render(<Fixture />)
  const table = await screen.findByRole('table')
  expect(
    within(table).getByRole('row', { name: /Not recorded/ })
  ).toHaveTextContent('100%')
  expect(within(table).queryByText('HTTP 200')).not.toBeInTheDocument()
})

it('does not request or render system-wide statistics for a regular user', async () => {
  useAuthStore.getState().auth.setUser({ id: 2, username: 'user', role: 1 })
  const view = render(<Fixture />)
  await act(async () => {})
  expect(view.container).toBeEmptyDOMElement()
  expect(api.get).not.toHaveBeenCalled()
})

it('updates number formatting when switching among the seven interface languages', async () => {
  render(<Fixture />)
  const table = await screen.findByRole('table')
  const row = within(table).getByRole('row', { name: /HTTP 200/ })
  expect(row).toHaveTextContent('4,000')
  for (const [language, count] of [
    ['zhCN', '4,000'],
    ['zhTW', '4,000'],
    ['fr', '4\u202f000'],
    ['ru', '4\u00a0000'],
    ['ja', '4,000'],
    ['vi', '4.000'],
  ]) {
    i18next.addResourceBundle(language, 'translation', { Count: 'Count' })
    await act(async () => {
      await i18next.changeLanguage(language)
    })
    expect(row.textContent).toContain(count)
  }
})
