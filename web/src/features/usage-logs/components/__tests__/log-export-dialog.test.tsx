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
  act,
  render as renderUI,
  screen,
  waitFor,
  within,
} from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import type { ComponentProps, ReactNode } from 'react'
import { afterEach, beforeEach, expect, test, vi } from 'vitest'

import { api } from '@/lib/api'
import { useAuthStore } from '@/stores/auth-store'

import { CommonLogsFilterBar } from '../common-logs-filter-bar'
import { LogExportDialog } from '../log-export-dialog'
import { LogExportHistory } from '../log-export-history'
import { UsageLogsProvider, useUsageLogsContext } from '../usage-logs-provider'

const originalGetAnimations = Object.getOwnPropertyDescriptor(
  Element.prototype,
  'getAnimations'
)

function render(ui: ReactNode) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  return renderUI(
    <QueryClientProvider client={client}>{ui}</QueryClientProvider>
  )
}

beforeEach(() => {
  vi.spyOn(api, 'get').mockImplementation(async (url) => {
    if (url === '/api/log/export/config') {
      return {
        data: {
          success: true,
          data: {
            max_rows: 100000,
            retention_days: 7,
            max_file_bytes: 67108864,
          },
        },
      }
    }
    return { data: { success: true, data: [], total: 0 } }
  })
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
  Reflect.deleteProperty(window, 'showSaveFilePicker')
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

test('queues the configured administrator export and opens persistent history without downloading synchronously', async () => {
  const exporter = vi
    .fn<NonNullable<ComponentProps<typeof LogExportDialog>['exporter']>>()
    .mockResolvedValue({ task_id: 'export-1' } as Awaited<
      ReturnType<
        NonNullable<ComponentProps<typeof LogExportDialog>['exporter']>
      >
    >)
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
  await waitFor(() =>
    expect(
      within(dialog).getByRole('tab', { name: 'Export history' })
    ).toHaveAttribute('aria-selected', 'true')
  )
  expect(dialog).toBeVisible()
  expect(URL.createObjectURL).not.toHaveBeenCalled()
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
  await waitFor(() =>
    expect(within(dialog).getByRole('button', { name: 'Export' })).toBeEnabled()
  )
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
  await waitFor(() =>
    expect(within(dialog).getByRole('button', { name: 'Export' })).toBeEnabled()
  )
})

test('history retains statuses and downloads completed files through the authenticated API while expired files cannot be downloaded', async () => {
  let filename = ''
  vi.spyOn(HTMLAnchorElement.prototype, 'click').mockImplementation(
    function (this: HTMLAnchorElement) {
      filename = this.download
    }
  )
  const now = Math.floor(Date.now() / 1000)
  const base = {
    view: 'self',
    error: '',
    max_rows: 100000,
    row_count: 3,
    file_size: 100,
    filename: 'usage-self.zip',
    created_at: now,
    updated_at: now,
    expires_at: now + 86400,
    filters: { view: 'self', fields: [] },
  }
  const get = vi.spyOn(api, 'get').mockImplementation(async (url) => {
    if (url === '/api/log/self/exports/ready/file') {
      return {
        data: new Blob(['zip']),
        headers: { 'content-type': 'application/zip' },
      }
    }
    return {
      data: {
        success: true,
        total: 4,
        data: [
          { ...base, task_id: 'ready', status: 'succeeded' },
          {
            ...base,
            task_id: 'expired',
            status: 'succeeded',
            expires_at: now - 1,
          },
          { ...base, task_id: 'pending', status: 'pending' },
          {
            ...base,
            task_id: 'failed',
            status: 'failed',
            error: 'export exceeds 10 records; narrow the filters',
          },
        ],
      },
    }
  })
  const mounted = render(<LogExportHistory scope='self' />)
  expect(await screen.findByText('Queued')).toBeVisible()
  expect(screen.getByText('Expired')).toBeVisible()
  expect(screen.getByText('Failed')).toBeVisible()
  expect(screen.getAllByRole('button', { name: 'Download' })).toHaveLength(1)
  await userEvent.click(screen.getByRole('button', { name: 'Download' }))
  await waitFor(() => expect(filename).toBe('usage-self.zip'))
  expect(get).toHaveBeenCalledWith(
    '/api/log/self/exports/ready/file',
    expect.objectContaining({ responseType: 'blob' })
  )
  expect(URL.revokeObjectURL).toHaveBeenCalledWith('blob:usage-export')
  mounted.unmount()
})

test('history exposes a retry after a load failure and then shows the empty state', async () => {
  vi.spyOn(api, 'get')
    .mockRejectedValueOnce(new Error('offline'))
    .mockResolvedValue({ data: { success: true, data: [], total: 0 } })
  render(<LogExportHistory scope='self' />)
  expect(await screen.findByText('Unable to load export history')).toBeVisible()
  await userEvent.click(screen.getByRole('button', { name: 'Retry' }))
  expect(await screen.findByText('No export records yet')).toBeVisible()
})

test('a browser with file saving support streams the authenticated response to disk without creating a whole-file blob', async () => {
  const chunks: number[][] = []
  const destination = {
    createWritable: vi.fn(
      async () =>
        new WritableStream<Uint8Array>({
          write: (chunk) => {
            chunks.push([...chunk])
          },
        })
    ),
  }
  const picker = vi.fn(async () => destination)
  Object.defineProperty(window, 'showSaveFilePicker', {
    configurable: true,
    value: picker,
  })
  const now = Math.floor(Date.now() / 1000)
  const record = {
    task_id: 'streamed',
    view: 'self',
    status: 'succeeded',
    filename: 'large-export.zip',
    file_size: 400000000,
    row_count: 10000000,
    expires_at: now + 86400,
    created_at: now,
    error: '',
    filters: {},
  }
  const get = vi.spyOn(api, 'get').mockImplementation(async (url) => {
    if (url === '/api/log/self/exports/streamed/file') {
      return {
        data: new ReadableStream<Uint8Array>({
          start: (controller) => {
            controller.enqueue(new Uint8Array([80, 75]))
            controller.enqueue(new Uint8Array([3, 4]))
            controller.close()
          },
        }),
        headers: { 'content-type': 'application/zip' },
      }
    }
    return { data: { success: true, data: [record], total: 1 } }
  })
  render(<LogExportHistory scope='self' />)
  await userEvent.click(await screen.findByRole('button', { name: 'Download' }))
  await waitFor(() =>
    expect(chunks).toEqual([
      [80, 75],
      [3, 4],
    ])
  )
  expect(picker).toHaveBeenCalledWith({ suggestedName: 'large-export.zip' })
  expect(get).toHaveBeenCalledWith(
    '/api/log/self/exports/streamed/file',
    expect.objectContaining({ adapter: 'fetch', responseType: 'stream' })
  )
  expect(URL.createObjectURL).not.toHaveBeenCalled()
})

test('switching accounts hides the previous account export history while the new history loads', async () => {
  useAuthStore.getState().auth.setUser({ id: 41, username: 'first', role: 1 })
  let resolveNext: (value: unknown) => void = () => undefined
  const nextHistory = new Promise((resolve) => {
    resolveNext = resolve
  })
  vi.spyOn(api, 'get')
    .mockResolvedValueOnce({
      data: {
        success: true,
        total: 1,
        data: [
          {
            task_id: 'first-private-export',
            view: 'self',
            status: 'failed',
            error: 'First account export',
            created_at: 100,
            filters: {},
          },
        ],
      },
    })
    .mockImplementationOnce(async () => nextHistory)
  render(<LogExportHistory scope='self' />)
  expect(await screen.findByText('First account export')).toBeVisible()
  act(() =>
    useAuthStore
      .getState()
      .auth.setUser({ id: 42, username: 'second', role: 1 })
  )
  expect(screen.queryByText('First account export')).not.toBeInTheDocument()
  await act(async () =>
    resolveNext({ data: { success: true, data: [], total: 0 } })
  )
  expect(await screen.findByText('No export records yet')).toBeVisible()
})
