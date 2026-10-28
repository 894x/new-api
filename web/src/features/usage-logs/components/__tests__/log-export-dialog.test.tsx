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
import { act, render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import type { ComponentProps } from 'react'
import { afterEach, beforeEach, expect, test, vi } from 'vitest'

import { api } from '@/lib/api'
import { useAuthStore } from '@/stores/auth-store'

import { CommonLogsFilterBar } from '../common-logs-filter-bar'
import { LogExportDialog } from '../log-export-dialog'
import { UsageLogsProvider, useUsageLogsContext } from '../usage-logs-provider'

const originalGetAnimations = Object.getOwnPropertyDescriptor(
  Element.prototype,
  'getAnimations'
)

beforeEach(() => {
  vi.spyOn(window, 'scrollTo').mockImplementation(() => undefined)
  vi.stubGlobal(
    'URL',
    class extends URL {
      static override createObjectURL = vi
        .fn()
        .mockReturnValue('blob:usage-export')
      static override revokeObjectURL = vi.fn()
    }
  )
  Object.defineProperty(Element.prototype, 'getAnimations', {
    configurable: true,
    value: () => [],
  })
})

afterEach(() => {
  vi.restoreAllMocks()
  vi.unstubAllGlobals()
  useAuthStore.getState().auth.setUser(null)
  if (originalGetAnimations) {
    Object.defineProperty(
      Element.prototype,
      'getAnimations',
      originalGetAnimations
    )
  } else {
    Reflect.deleteProperty(Element.prototype, 'getAnimations')
  }
})

function ExportToolbarFixture() {
  const { setViewScope } = useUsageLogsContext()
  const table = useReactTable({
    data: [],
    columns: [],
    getCoreRowModel: getCoreRowModel(),
  })
  return (
    <>
      <button type='button' onClick={() => setViewScope('self')}>
        View my logs
      </button>
      <CommonLogsFilterBar table={table} />
    </>
  )
}

test.each([
  { role: 1, self: false, title: 'Export my consumption data' },
  { role: 10, self: true, title: 'Export my consumption data' },
  { role: 10, self: false, title: 'Export usage logs' },
])(
  'exposes the correct toolbar export for role $role with self scope $self',
  async ({ role, self, title }) => {
    useAuthStore.getState().auth.setUser({ id: 1, username: 'viewer', role })
    vi.spyOn(api, 'get').mockImplementation(async (url) => {
      if (url === '/api/group/') return { data: { success: true, data: [] } }
      if (url === '/api/user/self/groups') {
        return { data: { success: true, data: {} } }
      }
      return { data: { success: true, data: { quota: 0, rpm: 0, tpm: 0 } } }
    })
    const root = createRootRoute()
    const auth = createRoute({
      getParentRoute: () => root,
      id: '_authenticated',
    })
    const logs = createRoute({
      getParentRoute: () => auth,
      path: '/usage-logs/$section',
      component: ExportToolbarFixture,
      validateSearch: (search: Record<string, unknown>) => search,
    })
    const router = createRouter({
      routeTree: root.addChildren([auth.addChildren([logs])]),
      history: createMemoryHistory({ initialEntries: ['/usage-logs/common'] }),
    })
    const client = new QueryClient({
      defaultOptions: { queries: { retry: false } },
    })
    render(
      <QueryClientProvider client={client}>
        <UsageLogsProvider>
          <RouterProvider router={router} />
        </UsageLogsProvider>
      </QueryClientProvider>
    )
    if (self) {
      await userEvent.click(
        await screen.findByRole('button', { name: 'View my logs' })
      )
    }
    await userEvent.click(await screen.findByRole('button', { name: 'Export' }))
    expect(screen.getByRole('dialog', { name: title })).toBeVisible()
  }
)

test('associates the administrator scope label and downloads the configured archive', async () => {
  vi.spyOn(URL, 'createObjectURL').mockReturnValue('blob:usage-export')
  const revoke = vi
    .spyOn(URL, 'revokeObjectURL')
    .mockImplementation(() => undefined)
  let downloadedFilename = ''
  vi.spyOn(HTMLAnchorElement.prototype, 'click').mockImplementation(
    function (this: HTMLAnchorElement) {
      downloadedFilename = this.download
    }
  )
  const exporter = vi
    .fn<NonNullable<ComponentProps<typeof LogExportDialog>['exporter']>>()
    .mockResolvedValue({
      blob: new Blob(['zip']),
      filename: 'usage-upstream.zip',
    })
  render(
    <LogExportDialog
      scope='all'
      params={{ request_id: 'client-request' }}
      exporter={exporter}
    />
  )
  await userEvent.click(screen.getByRole('button', { name: 'Export' }))
  const dialog = screen.getByRole('dialog')
  expect(
    within(dialog).getByRole('combobox', { name: 'Reconciliation view' })
  ).toBeVisible()
  expect(
    within(dialog).getByRole('checkbox', { name: 'Actual Charge (USD)' })
  ).toBeChecked()
  await userEvent.click(within(dialog).getByRole('button', { name: 'Export' }))
  await waitFor(() =>
    expect(exporter).toHaveBeenCalledWith(
      expect.objectContaining({
        view: 'upstream',
        request_id: 'client-request',
        fields: expect.arrayContaining(['actual_amount_usd']),
      })
    )
  )
  expect(downloadedFilename).toBe('usage-upstream.zip')
  expect(revoke).toHaveBeenCalledWith('blob:usage-export')
  await waitFor(() =>
    expect(screen.queryByRole('dialog')).not.toBeInTheDocument()
  )
})

test('personal exports show only safe selectable columns and keep the self scope when selecting all or resetting', async () => {
  const exporter = vi
    .fn<NonNullable<ComponentProps<typeof LogExportDialog>['exporter']>>()
    .mockRejectedValue(new Error('try again'))
  render(
    <LogExportDialog
      scope='self'
      params={{
        username: 'other',
        channel: 9,
        upstream_request_id: 'private',
        group: 'internal',
        request_id: 'mine',
      }}
      exporter={exporter}
    />
  )
  await userEvent.click(screen.getByRole('button', { name: 'Export' }))
  const dialog = screen.getByRole('dialog', {
    name: 'Export my consumption data',
  })
  expect(within(dialog).queryByRole('combobox')).not.toBeInTheDocument()
  expect(within(dialog).getAllByRole('checkbox')).toHaveLength(10)
  for (const name of [
    'Time',
    'Request ID',
    'Consumption type',
    'Token Name',
    'Model Name',
    'Input Tokens',
    'Cached Input Tokens',
    'Output Tokens',
    'Charged quota',
    'Actual Charge (USD)',
  ]) {
    expect(within(dialog).getByRole('checkbox', { name })).toBeChecked()
  }
  const amount = within(dialog).getByRole('checkbox', {
    name: 'Actual Charge (USD)',
  })
  await userEvent.click(amount)
  expect(amount).not.toBeChecked()
  await userEvent.click(
    within(dialog).getByRole('button', { name: 'Select all' })
  )
  expect(amount).toBeChecked()
  await userEvent.click(amount)
  await userEvent.click(within(dialog).getByRole('button', { name: 'Reset' }))
  expect(amount).toBeChecked()
  await userEvent.click(within(dialog).getByRole('button', { name: 'Export' }))
  await waitFor(() =>
    expect(exporter).toHaveBeenCalledWith({
      view: 'self',
      fields: [
        'created_at',
        'request_id',
        'consumption_type',
        'token_name',
        'model_name',
        'input_tokens',
        'cached_input_tokens',
        'output_tokens',
        'charged_quota',
        'actual_amount_usd',
      ],
      request_id: 'mine',
    })
  )
  expect(dialog).toBeVisible()
  expect(within(dialog).getByRole('button', { name: 'Export' })).toBeEnabled()
})

test('personal exports disable submission with no selected columns and while a request is pending, then allow retry after failure', async () => {
  let rejectExport: (error: Error) => void = () => undefined
  const exportResult = new Promise<never>((_resolve, reject) => {
    rejectExport = reject
  })
  render(
    <LogExportDialog scope='self' params={{}} exporter={() => exportResult} />
  )
  await userEvent.click(screen.getByRole('button', { name: 'Export' }))
  const dialog = screen.getByRole('dialog')
  for (const checkbox of within(dialog).getAllByRole('checkbox')) {
    await userEvent.click(checkbox)
  }
  expect(within(dialog).getByRole('button', { name: 'Export' })).toBeDisabled()
  await userEvent.click(
    within(dialog).getByRole('checkbox', { name: 'Request ID' })
  )
  await userEvent.click(within(dialog).getByRole('button', { name: 'Export' }))
  expect(
    within(dialog).getByRole('button', { name: 'Exporting...' })
  ).toBeDisabled()
  expect(within(dialog).getByRole('button', { name: 'Cancel' })).toBeDisabled()
  await act(async () => {
    rejectExport(new Error('export failed'))
    await exportResult.catch(() => undefined)
  })
  expect(dialog).toBeVisible()
  expect(within(dialog).getByRole('button', { name: 'Export' })).toBeEnabled()
})
