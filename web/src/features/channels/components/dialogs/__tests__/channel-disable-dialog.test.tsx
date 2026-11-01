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
  getCoreRowModel,
  useReactTable,
  type RowSelectionState,
} from '@tanstack/react-table'
import {
  act,
  cleanup,
  render,
  screen,
  waitFor,
  within,
} from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { useState, type ReactNode } from 'react'
import { afterEach, beforeEach, expect, test, vi } from 'vitest'

import { TooltipProvider } from '@/components/ui/tooltip'
import { api } from '@/lib/api'
import { ROLE } from '@/lib/roles'
import { useAuthStore } from '@/stores/auth-store'

import type { ChannelDisableImpact } from '../../../api'
import { channelSchema } from '../../../types'
import { ChannelsProvider } from '../../channels-provider'
import { DataTableBulkActions } from '../../data-table-bulk-actions'
import { DataTableRowActions } from '../../data-table-row-actions'
import { DataTableTagRowActions } from '../../data-table-tag-row-actions'
import { ChannelDisableDialog } from '../channel-disable-dialog'

const originalAuth = useAuthStore.getState().auth
let client: QueryClient
let impact: ChannelDisableImpact
const channel = channelSchema.parse({
  id: 58,
  name: 'Vendor A',
  key: '',
  type: 1,
  status: 1,
  created_time: 0,
  test_time: 0,
  response_time: 0,
  balance_updated_time: 0,
  models: 'kimi-k3.1-pro',
  group: 'kimi-k3.1 pro',
  tag: 'vendors',
})

function Harness(props: {
  children?: ReactNode
  mode?: 'row' | 'batch' | 'tag'
  status?: number
}) {
  const [rowSelection, setRowSelection] = useState<RowSelectionState>({
    '0': true,
    '1': true,
  })
  const table = useReactTable({
    data: [
      { ...channel, status: props.status ?? 1 },
      { ...channel, id: 47, name: 'Vendor B' },
    ],
    columns: [],
    getCoreRowModel: getCoreRowModel(),
    state: { rowSelection },
    onRowSelectionChange: setRowSelection,
  })
  return (
    <QueryClientProvider client={client}>
      <TooltipProvider>
        <ChannelsProvider>
          {props.mode === 'row' && (
            <DataTableRowActions row={table.getRowModel().rows[0]} />
          )}
          {props.mode === 'batch' && <DataTableBulkActions table={table} />}
          {props.mode === 'tag' && (
            <DataTableTagRowActions row={table.getRowModel().rows[0]} />
          )}
          {props.children}
        </ChannelsProvider>
      </TooltipProvider>
    </QueryClientProvider>
  )
}

beforeEach(() => {
  client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  })
  useAuthStore.setState({
    auth: {
      ...originalAuth,
      user: { id: 1, username: 'root', role: ROLE.SUPER_ADMIN },
    },
  })
  impact = {
    channels: [{ id: 58, name: 'Vendor A' }],
    customer_count: 2,
    customers: [
      {
        user_id: 116,
        username: 'customer-a',
        display_name: '',
        channel_id: 58,
        model: 'kimi-k3.1-pro',
        key_group: 'kimi-k3.1 pro',
        effective_priority: 1,
        primary: false,
        alternative_channels: 2,
      },
      {
        user_id: 37,
        username: 'customer-b',
        display_name: '',
        channel_id: 58,
        model: 'kimi-k3.1-pro',
        key_group: 'kimi-k3.1 pro',
        effective_priority: 103,
        primary: true,
        alternative_channels: 0,
      },
    ],
  }
  vi.spyOn(api, 'post').mockImplementation(async (url) => {
    if (url === '/api/channel/disable-impact') {
      return { data: { success: true, data: impact } }
    }
    return { data: { success: true, data: 1 } }
  })
})

afterEach(() => {
  cleanup()
  client.clear()
  useAuthStore.setState({ auth: originalAuth })
})

test('row disable shows customers and effective priorities and only submits after confirmation', async () => {
  const user = userEvent.setup()
  render(<Harness mode='row' />)
  await user.click(screen.getByRole('button', { name: 'Disable' }))
  const dialog = await screen.findByRole('alertdialog', {
    name: 'Disable channels?',
  })
  expect(await within(dialog).findByText('customer-a (#116)')).toBeVisible()
  expect(within(dialog).getByText('Primary route')).toBeVisible()
  expect(within(dialog).getByText('Fallback')).toBeVisible()
  expect(within(dialog).getByText('103')).toBeVisible()
  expect(within(dialog).getByText('No remaining channels')).toBeVisible()
  const routes = within(dialog).getByRole('region', {
    name: 'Affected customer routes',
  })
  expect(routes).toHaveAttribute('tabindex', '0')
  expect(routes).toHaveClass('overflow-x-auto')
  expect(api.post).not.toHaveBeenCalledWith(
    '/api/channel/status/batch',
    expect.anything(),
    expect.anything()
  )
  await user.click(
    within(dialog).getByRole('button', { name: 'Confirm disable' })
  )
  await waitFor(() =>
    expect(api.post).toHaveBeenCalledWith(
      '/api/channel/status/batch',
      { ids: [58], status: 2 },
      expect.anything()
    )
  )
  await waitFor(() =>
    expect(screen.queryByRole('alertdialog')).not.toBeInTheDocument()
  )
})

test('pending impact check blocks disabling and keyboard cancellation leaves channel enabled', async () => {
  const user = userEvent.setup()
  let resolve!: (value: {
    data: { success: boolean; data: ChannelDisableImpact }
  }) => void
  vi.mocked(api.post).mockImplementationOnce(
    () =>
      new Promise((done) => {
        resolve = done
      })
  )
  render(<Harness mode='row' />)
  await user.click(screen.getByRole('button', { name: 'Disable' }))
  expect(
    await screen.findByRole('button', { name: 'Confirm disable' })
  ).toBeDisabled()
  expect(screen.getByText('Checking affected customers...')).toBeVisible()
  await user.keyboard('{Escape}')
  await waitFor(() =>
    expect(screen.queryByRole('alertdialog')).not.toBeInTheDocument()
  )
  await act(async () => resolve({ data: { success: true, data: impact } }))
  expect(api.post).toHaveBeenCalledTimes(1)
})

test('failed impact check blocks confirmation and retry loads the affected routes', async () => {
  const user = userEvent.setup()
  vi.mocked(api.post).mockResolvedValueOnce({
    data: { success: false, message: 'Routing unavailable' },
  })
  render(
    <Harness>
      <ChannelDisableDialog target={{ ids: [58] }} onOpenChange={vi.fn()} />
    </Harness>
  )
  expect(
    await screen.findByText('Failed to check affected customers')
  ).toBeVisible()
  expect(screen.getByRole('button', { name: 'Confirm disable' })).toBeDisabled()
  await user.click(screen.getByRole('button', { name: 'Retry' }))
  expect(await screen.findByText('customer-a (#116)')).toBeVisible()
  expect(screen.getByRole('button', { name: 'Confirm disable' })).toBeEnabled()
})

test('batch disable checks the selected channels together and submits the reviewed IDs', async () => {
  const user = userEvent.setup()
  impact.channels.push({ id: 47, name: 'Vendor B' })
  render(<Harness mode='batch' />)
  await user.click(
    screen.getByRole('button', { name: 'Disable selected channels' })
  )
  await screen.findByText('customer-a (#116)')
  expect(api.post).toHaveBeenCalledWith(
    '/api/channel/disable-impact',
    { ids: [58, 47] },
    expect.anything()
  )
  await user.click(screen.getByRole('button', { name: 'Confirm disable' }))
  await waitFor(() =>
    expect(api.post).toHaveBeenCalledWith(
      '/api/channel/status/batch',
      { ids: [58, 47], status: 2 },
      expect.anything()
    )
  )
})

test('tag disable checks all enabled tag members and freezes that reviewed selection', async () => {
  const user = userEvent.setup()
  render(<Harness mode='tag' />)
  await user.click(screen.getByRole('button', { name: 'Open menu' }))
  await user.click(await screen.findByRole('menuitem', { name: 'Disable All' }))
  await screen.findByText('customer-a (#116)')
  expect(api.post).toHaveBeenCalledWith(
    '/api/channel/disable-impact',
    { tag: 'vendors' },
    expect.anything()
  )
  await user.click(screen.getByRole('button', { name: 'Confirm disable' }))
  await waitFor(() =>
    expect(api.post).toHaveBeenCalledWith(
      '/api/channel/status/batch',
      { ids: [58], status: 2 },
      expect.anything()
    )
  )
})

test('an empty customer list still requires confirmation while an empty channel selection cannot be submitted', async () => {
  impact.customers = []
  impact.customer_count = 0
  const view = render(
    <Harness>
      <ChannelDisableDialog target={{ ids: [58] }} onOpenChange={vi.fn()} />
    </Harness>
  )
  expect(
    await screen.findByText('No affected customers with valid API keys')
  ).toBeVisible()
  expect(screen.getByRole('button', { name: 'Confirm disable' })).toBeEnabled()
  view.unmount()
  client.clear()
  impact.channels = []
  render(
    <Harness>
      <ChannelDisableDialog target={{ ids: [47] }} onOpenChange={vi.fn()} />
    </Harness>
  )
  expect(
    await screen.findByText('No enabled channels in this selection')
  ).toBeVisible()
  expect(screen.getByRole('button', { name: 'Confirm disable' })).toBeDisabled()
})

test('a failed disable keeps the confirmation dialog open for retry', async () => {
  const user = userEvent.setup()
  const onOpenChange = vi.fn()
  vi.mocked(api.post).mockImplementation(async (url) => {
    if (url === '/api/channel/disable-impact') {
      return { data: { success: true, data: impact } }
    }
    return { data: { success: false, message: 'Disable rejected' } }
  })
  render(
    <Harness>
      <ChannelDisableDialog
        target={{ ids: [58] }}
        onOpenChange={onOpenChange}
      />
    </Harness>
  )
  await screen.findByText('customer-a (#116)')
  await user.click(screen.getByRole('button', { name: 'Confirm disable' }))
  await waitFor(() =>
    expect(
      screen.getByRole('button', { name: 'Confirm disable' })
    ).toBeEnabled()
  )
  expect(onOpenChange).not.toHaveBeenCalled()
  expect(screen.getByRole('alertdialog')).toBeVisible()
})

test('enabling a disabled row preserves the existing direct enable action', async () => {
  const user = userEvent.setup()
  render(<Harness mode='row' status={2} />)
  await user.click(screen.getByRole('button', { name: 'Enable' }))
  await waitFor(() =>
    expect(api.post).toHaveBeenCalledWith(
      '/api/channel/58/status',
      { status: 1 },
      expect.anything()
    )
  )
  expect(screen.queryByRole('alertdialog')).not.toBeInTheDocument()
})
