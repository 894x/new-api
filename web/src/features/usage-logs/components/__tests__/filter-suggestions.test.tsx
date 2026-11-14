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
  createMemoryHistory,
  createRootRoute,
  createRoute,
  createRouter,
  RouterProvider,
} from '@tanstack/react-router'
import { getCoreRowModel, useReactTable } from '@tanstack/react-table'
import {
  cleanup,
  render,
  screen,
  waitFor,
  within,
} from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, beforeEach, expect, it, vi } from 'vitest'

import { api } from '@/lib/api'
import { Route as UsageLogsRoute } from '@/routes/_authenticated/usage-logs/$section'
import { useAuthStore } from '@/stores/auth-store'

import { fetchLogsByCategory } from '../../lib/utils'
import type { LogCategory } from '../../types'
import { CommonLogsFilterBar } from '../common-logs-filter-bar'
import { TaskLogsFilterBar } from '../task-logs-filter-bar'
import { UsageLogsProvider } from '../usage-logs-provider'

const commonLogs: Record<string, unknown>[] = [
  {
    model_name: 'recent-model',
    token_name: 'recent-token',
    username: 'recent-user',
    channel: 99,
    channel_name: 'Archived channel',
    request_id: 'request-1',
    upstream_request_id: 'upstream-1',
    other: '{"status_code":429}',
  },
]
let statusCodes: number[] | null = []
const pointerCaptureDescriptor = Object.getOwnPropertyDescriptor(
  HTMLElement.prototype,
  'setPointerCapture'
)

function FilterFixture(props: {
  logCategory: LogCategory
  data: Record<string, unknown>[]
}) {
  const table = useReactTable({
    data: props.data,
    columns: [],
    getCoreRowModel: getCoreRowModel(),
  })
  return (
    <UsageLogsProvider>
      {props.logCategory === 'common' ? (
        <CommonLogsFilterBar table={table} />
      ) : (
        <TaskLogsFilterBar table={table} logCategory={props.logCategory} />
      )}
    </UsageLogsProvider>
  )
}

async function renderFilter(
  logCategory: LogCategory = 'common',
  data: Record<string, unknown>[] = commonLogs,
  initialEntry = `/usage-logs/${logCategory}`
) {
  const root = createRootRoute()
  const auth = createRoute({ getParentRoute: () => root, id: '_authenticated' })
  const logs = createRoute({
    getParentRoute: () => auth,
    path: '/usage-logs/$section',
    component: () => <FilterFixture logCategory={logCategory} data={data} />,
    validateSearch: UsageLogsRoute.options.validateSearch,
  })
  const router = createRouter({
    routeTree: root.addChildren([auth.addChildren([logs])]),
    history: createMemoryHistory({
      initialEntries: [initialEntry],
    }),
  })
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  render(
    <QueryClientProvider client={client}>
      <RouterProvider router={router} />
    </QueryClientProvider>
  )
  if (window.matchMedia('(max-width: 640px)').matches) {
    await userEvent.click(
      await screen.findByRole('button', { name: /^Filter/ })
    )
  }
  await screen.findByRole('combobox', {
    name: logCategory === 'common' ? 'Model Name' : 'Task ID',
  })
  return router
}

beforeEach(() => {
  statusCodes = [200, 429, 503]
  useAuthStore.getState().auth.setUser({ id: 1, username: 'admin', role: 10 })
  vi.spyOn(api, 'get').mockImplementation(async (url) => {
    if (url === '/api/log/status_codes') {
      if (statusCodes === null) throw new Error('Status codes unavailable')
      return { data: { success: true, data: statusCodes } }
    }
    if (url === '/api/user/models') {
      return { data: { success: true, data: ['available-model'] } }
    }
    if (url === '/api/token/?p=1&size=100') {
      return {
        data: {
          success: true,
          data: { items: [{ name: 'available-token' }], total: 1 },
        },
      }
    }
    if (url === '/api/user/') {
      return {
        data: {
          success: true,
          data: { items: [{ username: 'available-user' }], total: 1 },
        },
      }
    }
    if (url === '/api/channel') {
      return {
        data: {
          success: true,
          data: {
            items: [{ id: 7, name: 'Primary channel' }],
            total: 1,
          },
        },
      }
    }
    if (url === '/api/group/') {
      return { data: { success: true, data: ['default'] } }
    }
    return { data: { success: true, data: { quota: 0, rpm: 0, tpm: 0 } } }
  })
})

afterEach(() => {
  cleanup()
  vi.restoreAllMocks()
  useAuthStore.getState().auth.setUser(null)
  if (pointerCaptureDescriptor) {
    Object.defineProperty(
      HTMLElement.prototype,
      'setPointerCapture',
      pointerCaptureDescriptor
    )
  } else {
    Reflect.deleteProperty(HTMLElement.prototype, 'setPointerCapture')
  }
})

it('recommends values for every common text filter and submits a selected channel ID', async () => {
  const router = await renderFilter()
  const user = userEvent.setup()

  await user.click(screen.getByRole('combobox', { name: 'Model Name' }))
  expect(
    await screen.findByRole('option', { name: 'available-model' })
  ).toBeVisible()
  expect(screen.getByRole('option', { name: 'recent-model' })).toBeVisible()
  await user.keyboard('{Escape}')

  await user.click(screen.getByRole('button', { name: 'Expand' }))
  for (const [field, option] of [
    ['Token Name', 'available-token'],
    ['Username', 'available-user'],
    ['Request ID', 'request-1'],
    ['Upstream Request ID', 'upstream-1'],
  ]) {
    await user.click(screen.getByRole('combobox', { name: field }))
    expect(await screen.findByRole('option', { name: option })).toBeVisible()
    await user.keyboard('{Escape}')
  }

  const channelInput = screen.getByRole('combobox', { name: 'Channel ID' })
  await user.click(channelInput)
  expect(
    await screen.findByRole('option', { name: 'Archived channel (#99)' })
  ).toBeVisible()
  await user.click(screen.getByRole('option', { name: 'Primary channel (#7)' }))
  expect(channelInput).toHaveValue('Primary channel (#7)')
  await user.click(screen.getByRole('button', { name: 'Search' }))
  await waitFor(() =>
    expect(router.state.location.search).toMatchObject({ channel: '7' })
  )
})

it('recommends recent task IDs while keeping custom task IDs editable', async () => {
  const router = await renderFilter('task', [{ task_id: 'task-1' }])
  const user = userEvent.setup()
  const input = screen.getByRole('combobox', { name: 'Task ID' })

  await user.click(input)
  await user.click(await screen.findByRole('option', { name: 'task-1' }))
  await user.clear(input)
  await user.type(input, 'custom-task')
  await user.click(screen.getByRole('button', { name: 'Search' }))

  await waitFor(() =>
    expect(router.state.location.search).toMatchObject({
      filter: 'custom-task',
    })
  )
})

it('offers recorded status codes beyond the current page and sends the selection to the log API', async () => {
  const router = await renderFilter(
    'common',
    commonLogs,
    '/usage-logs/common?page=3&model=recent-model'
  )
  const user = userEvent.setup()
  const statusInput = screen.getByRole('combobox', { name: 'Status Code' })
  await user.click(statusInput)
  expect(statusInput).toHaveAttribute('aria-expanded', 'true')
  await screen.findByRole('option', { name: 'HTTP 503' })
  await user.keyboard('{End}{Enter}')
  expect(statusInput).toHaveAttribute('aria-expanded', 'false')
  expect(statusInput).toHaveTextContent('HTTP 503')
  expect(router.state.location.search).not.toHaveProperty('statusCode')
  await user.click(screen.getByRole('button', { name: 'Search' }))
  await waitFor(() =>
    expect(router.state.location.search).toMatchObject({
      statusCode: '503',
      model: 'recent-model',
      page: 1,
    })
  )
  await fetchLogsByCategory({
    logCategory: 'common',
    isAdmin: true,
    page: 1,
    pageSize: 20,
    searchParams: router.state.location.search,
    columnFilters: [],
  })
  expect(api.get).toHaveBeenCalledWith(
    expect.stringMatching(/\/api\/log\?.*status_code=503/)
  )
})

it('restores a status from the URL, clears it with All, and resets it with other filters', async () => {
  const router = await renderFilter(
    'common',
    [],
    '/usage-logs/common?statusCode=429'
  )
  const user = userEvent.setup()
  const input = screen.getByRole('combobox', { name: 'Status Code' })
  expect(input).toHaveTextContent('HTTP 429')
  await user.click(input)
  await user.click(await screen.findByRole('option', { name: /^All$/ }))
  await user.click(screen.getByRole('button', { name: 'Search' }))
  await waitFor(() =>
    expect(router.state.location.search).not.toHaveProperty('statusCode')
  )
  await router.history.push('/usage-logs/common?statusCode=503')
  await waitFor(() => expect(input).toHaveTextContent('HTTP 503'))
  await user.click(screen.getByRole('button', { name: 'Reset' }))
  await waitFor(() =>
    expect(router.state.location.search).not.toHaveProperty('statusCode')
  )
  expect(input).not.toHaveTextContent('HTTP 503')
})

it.each([{ codes: [] }, { codes: null }])(
  'keeps a linked status usable when recorded choices are empty or unavailable ($codes)',
  async ({ codes }) => {
    statusCodes = codes
    await renderFilter('common', [], '/usage-logs/common?statusCode=502')
    const input = screen.getByRole('combobox', { name: 'Status Code' })
    expect(input).toHaveTextContent('HTTP 502')
    await userEvent.click(input)
    expect(
      await screen.findByRole('option', { name: 'HTTP 502' })
    ).toBeVisible()
    expect(
      screen.queryByRole('option', { name: 'HTTP 429' })
    ).not.toBeInTheDocument()
    expect(screen.getByRole('option', { name: /^All$/ })).toBeVisible()
  }
)

it('hides the status control and omits its API parameter in the personal log view', async () => {
  useAuthStore.getState().auth.setUser({ id: 2, username: 'user', role: 1 })
  const router = await renderFilter(
    'common',
    [],
    '/usage-logs/common?statusCode=503'
  )
  expect(
    screen.queryByRole('combobox', { name: 'Status Code' })
  ).not.toBeInTheDocument()
  expect(api.get).not.toHaveBeenCalledWith('/api/log/status_codes')
  await fetchLogsByCategory({
    logCategory: 'common',
    isAdmin: false,
    page: 1,
    pageSize: 20,
    searchParams: router.state.location.search,
    columnFilters: [],
  })
  expect(api.get).toHaveBeenCalledWith(
    expect.stringMatching(/\/api\/log\/self\?/)
  )
  expect(api.get).not.toHaveBeenCalledWith(
    expect.stringContaining('status_code=')
  )
})

it('selects a status in the mobile drawer and applies it when Search is pressed', async () => {
  Object.defineProperty(HTMLElement.prototype, 'setPointerCapture', {
    configurable: true,
    value: vi.fn(),
  })
  const matchMedia = window.matchMedia
  vi.spyOn(window, 'matchMedia').mockImplementation((query) => ({
    ...matchMedia(query),
    matches: query === '(max-width: 640px)',
  }))
  const router = await renderFilter()
  const user = userEvent.setup()
  await user.click(screen.getByRole('combobox', { name: 'Status Code' }))
  await user.click(await screen.findByRole('option', { name: 'HTTP 503' }))
  expect(router.state.location.search).not.toHaveProperty('statusCode')
  const dialog = screen.getByRole('dialog')
  await user.click(within(dialog).getByRole('button', { name: 'Search' }))
  await waitFor(() =>
    expect(router.state.location.search).toMatchObject({
      statusCode: '503',
      page: 1,
    })
  )
  await waitFor(() =>
    expect(screen.queryByRole('dialog')).not.toBeInTheDocument()
  )
})
