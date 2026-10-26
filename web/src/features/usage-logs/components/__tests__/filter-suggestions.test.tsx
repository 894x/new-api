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
import { cleanup, render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, beforeEach, expect, it, vi } from 'vitest'

import { api } from '@/lib/api'
import { useAuthStore } from '@/stores/auth-store'

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
  },
]

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
  data: Record<string, unknown>[] = commonLogs
) {
  const root = createRootRoute()
  const auth = createRoute({ getParentRoute: () => root, id: '_authenticated' })
  const logs = createRoute({
    getParentRoute: () => auth,
    path: '/usage-logs/$section',
    component: () => <FilterFixture logCategory={logCategory} data={data} />,
    validateSearch: (search: Record<string, unknown>) => search,
  })
  const router = createRouter({
    routeTree: root.addChildren([auth.addChildren([logs])]),
    history: createMemoryHistory({
      initialEntries: [`/usage-logs/${logCategory}`],
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
  await screen.findByRole('combobox', {
    name: logCategory === 'common' ? 'Model Name' : 'Task ID',
  })
  return router
}

beforeEach(() => {
  useAuthStore.getState().auth.setUser({ id: 1, username: 'admin', role: 10 })
  vi.spyOn(api, 'get').mockImplementation(async (url) => {
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
