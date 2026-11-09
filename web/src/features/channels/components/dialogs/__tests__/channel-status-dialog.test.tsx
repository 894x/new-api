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

import type { ChannelStatusImpact } from '../../../api'
import { channelSchema } from '../../../types'
import { ChannelsProvider } from '../../channels-provider'
import { DataTableBulkActions } from '../../data-table-bulk-actions'
import { DataTableRowActions } from '../../data-table-row-actions'
import { DataTableTagRowActions } from '../../data-table-tag-row-actions'
import { ChannelStatusDialog } from '../channel-status-dialog'

const originalAuth = useAuthStore.getState().auth
let client: QueryClient
let impact: ChannelStatusImpact
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
    routing_pools: [
      {
        source: 'default_pool',
        user_id: 0,
        username: '',
        channel_id: 58,
        model: 'kimi-k3.1-pro',
        routing_group: 'kimi-k3.1 pro',
        priority: 1,
        enabled: true,
      },
      {
        source: 'user',
        user_id: 37,
        username: 'customer-b',
        channel_id: 58,
        model: 'kimi-k3.1-pro',
        routing_group: 'kimi-k3.1 pro',
        priority: 103,
        enabled: true,
      },
    ],
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
    if (
      url === '/api/channel/disable-impact' ||
      url === '/api/channel/enable-impact'
    ) {
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

test('row disable shows the default pool once and user overrides separately before confirmation', async () => {
  const user = userEvent.setup()
  render(<Harness mode='row' />)
  await user.click(screen.getByRole('button', { name: 'Disable' }))
  const dialog = await screen.findByRole('alertdialog', {
    name: 'Disable channels?',
  })
  expect(await within(dialog).findByText('customer-b (#37)')).toBeVisible()
  expect(within(dialog).getAllByText('Default channel pool')).toHaveLength(1)
  expect(
    within(dialog).queryByText('customer-a (#116)')
  ).not.toBeInTheDocument()
  expect(within(dialog).getByText('103')).toBeVisible()
  const routes = within(dialog).getByRole('region', {
    name: 'Affected routing pools',
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

test.each([
  {
    action: 'disable',
    status: 1,
    trigger: 'Disable',
    confirm: 'Confirm disable',
    combined: 'Disable channel and pool entries',
  },
  {
    action: 'enable',
    status: 2,
    trigger: 'Enable',
    confirm: 'Confirm enable',
    combined: 'Enable channel and pool entries',
  },
])(
  'pending $action check blocks submission and keyboard cancellation leaves channels unchanged',
  async (scenario) => {
    const user = userEvent.setup()
    let resolve!: (value: {
      data: { success: boolean; data: ChannelStatusImpact }
    }) => void
    vi.mocked(api.post).mockImplementationOnce(
      () =>
        new Promise((done) => {
          resolve = done
        })
    )
    render(<Harness mode='row' status={scenario.status} />)
    await user.click(screen.getByRole('button', { name: scenario.trigger }))
    expect(
      await screen.findByRole('button', { name: scenario.confirm })
    ).toBeDisabled()
    expect(screen.getByText('Checking routing pools...')).toBeVisible()
    expect(
      screen.getByRole('button', { name: scenario.combined })
    ).toBeDisabled()
    await user.keyboard('{Escape}')
    await waitFor(() =>
      expect(screen.queryByRole('alertdialog')).not.toBeInTheDocument()
    )
    await act(async () => resolve({ data: { success: true, data: impact } }))
    expect(api.post).toHaveBeenCalledTimes(1)
  }
)

test.each(['enable', 'disable'] as const)(
  'failed %s preview blocks confirmation until retry succeeds',
  async (action) => {
    const user = userEvent.setup()
    const confirm = action === 'enable' ? 'Confirm enable' : 'Confirm disable'
    const combined =
      action === 'enable'
        ? 'Enable channel and pool entries'
        : 'Disable channel and pool entries'
    vi.mocked(api.post).mockResolvedValueOnce({
      data: { success: false, message: 'Routing unavailable' },
    })
    render(
      <Harness>
        <ChannelStatusDialog
          action={action}
          target={{ ids: [58] }}
          onOpenChange={vi.fn()}
        />
      </Harness>
    )
    expect(
      await screen.findByText('Failed to check routing pools')
    ).toBeVisible()
    expect(screen.getByRole('button', { name: confirm })).toBeDisabled()
    expect(screen.getByRole('button', { name: combined })).toBeDisabled()
    await user.click(screen.getByRole('button', { name: 'Retry' }))
    expect(await screen.findByText('customer-b (#37)')).toBeVisible()
    expect(screen.getByRole('button', { name: confirm })).toBeEnabled()
  }
)

test('batch disable checks the selected channels together and submits the reviewed IDs', async () => {
  const user = userEvent.setup()
  impact.channels.push({ id: 47, name: 'Vendor B' })
  render(<Harness mode='batch' />)
  await user.click(
    screen.getByRole('button', { name: 'Disable selected channels' })
  )
  await screen.findByText('customer-b (#37)')
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
  await screen.findByText('customer-b (#37)')
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
  impact.routing_pools = []
  impact.customer_count = 0
  const view = render(
    <Harness>
      <ChannelStatusDialog target={{ ids: [58] }} onOpenChange={vi.fn()} />
    </Harness>
  )
  expect(
    await screen.findByText('No matching routing pool entries')
  ).toBeVisible()
  expect(screen.getByRole('button', { name: 'Confirm disable' })).toBeEnabled()
  view.unmount()
  client.clear()
  impact.channels = []
  render(
    <Harness>
      <ChannelStatusDialog target={{ ids: [47] }} onOpenChange={vi.fn()} />
    </Harness>
  )
  expect(
    await screen.findByText('No enabled channels in this selection')
  ).toBeVisible()
  expect(screen.getByRole('button', { name: 'Confirm disable' })).toBeDisabled()
})

test.each([
  { action: 'disable', buttonName: 'Confirm disable' },
  { action: 'disable', buttonName: 'Disable channel and pool entries' },
  { action: 'enable', buttonName: 'Confirm enable' },
  { action: 'enable', buttonName: 'Enable channel and pool entries' },
] as const)(
  'a failed $buttonName keeps the confirmation dialog open for retry',
  async (scenario) => {
    const user = userEvent.setup()
    const onOpenChange = vi.fn()
    vi.mocked(api.post).mockImplementation(async (url) => {
      if (
        url === '/api/channel/disable-impact' ||
        url === '/api/channel/enable-impact'
      ) {
        return { data: { success: true, data: impact } }
      }
      return { data: { success: false, message: 'Disable rejected' } }
    })
    render(
      <Harness>
        <ChannelStatusDialog
          action={scenario.action}
          target={{ ids: [58] }}
          onOpenChange={onOpenChange}
        />
      </Harness>
    )
    await screen.findByText('customer-b (#37)')
    await user.click(screen.getByRole('button', { name: scenario.buttonName }))
    await waitFor(() =>
      expect(
        screen.getByRole('button', { name: scenario.buttonName })
      ).toBeEnabled()
    )
    expect(onOpenChange).not.toHaveBeenCalled()
    expect(screen.getByRole('alertdialog')).toBeVisible()
  }
)

test('enabling a disabled row opens a pool review and only enables after confirmation', async () => {
  const user = userEvent.setup()
  render(<Harness mode='row' status={2} />)
  await user.click(screen.getByRole('button', { name: 'Enable' }))
  const dialog = await screen.findByRole('alertdialog', {
    name: 'Enable channels?',
  })
  expect(
    within(dialog).getByRole('button', {
      name: 'Enable channel and pool entries',
    })
  ).toBeVisible()
  expect(api.post).not.toHaveBeenCalledWith(
    '/api/channel/status/batch',
    expect.anything(),
    expect.anything()
  )
  await user.click(
    within(dialog).getByRole('button', { name: 'Confirm enable' })
  )
  await waitFor(() =>
    expect(api.post).toHaveBeenCalledWith(
      '/api/channel/status/batch',
      { ids: [58], status: 1 },
      expect.anything()
    )
  )
  expect(screen.queryByRole('alertdialog')).not.toBeInTheDocument()
})

test.each(['enable', 'disable'] as const)(
  'combined %s submits the reviewed IDs and blocks both actions while saving',
  async (action) => {
    const confirm = action === 'enable' ? 'Confirm enable' : 'Confirm disable'
    const combined =
      action === 'enable'
        ? 'Enable channel and pool entries'
        : 'Disable channel and pool entries'
    const user = userEvent.setup()
    let resolve!: (value: { data: { success: boolean; data: number } }) => void
    vi.mocked(api.post).mockImplementation(async (url) => {
      if (
        url === '/api/channel/disable-impact' ||
        url === '/api/channel/enable-impact'
      ) {
        return { data: { success: true, data: impact } }
      }
      return new Promise((done) => {
        resolve = done
      })
    })
    const onOpenChange = vi.fn()
    render(
      <Harness>
        <ChannelStatusDialog
          action={action}
          target={{ tag: 'vendors' }}
          onOpenChange={onOpenChange}
        />
      </Harness>
    )
    await screen.findByText('customer-b (#37)')
    await user.click(screen.getByRole('button', { name: combined }))
    expect(api.post).toHaveBeenLastCalledWith(
      '/api/channel/status/batch',
      {
        ids: [58],
        status: action === 'enable' ? 1 : 2,
        update_routing_pools: true,
      },
      expect.anything()
    )
    expect(screen.getByRole('button', { name: confirm })).toBeDisabled()
    expect(screen.getByRole('button', { name: combined })).toBeDisabled()
    expect(screen.getByRole('button', { name: 'Cancel' })).toBeDisabled()
    await user.keyboard('{Escape}')
    expect(onOpenChange).not.toHaveBeenCalled()
    await act(async () => resolve({ data: { success: true, data: 1 } }))
    await waitFor(() => expect(onOpenChange).toHaveBeenCalledWith(false))
  }
)

test.each(['enable', 'disable'] as const)(
  '%s requires pool editing permission for the combined action',
  async (action) => {
    const confirm = action === 'enable' ? 'Confirm enable' : 'Confirm disable'
    const combined =
      action === 'enable'
        ? 'Enable channel and pool entries'
        : 'Disable channel and pool entries'
    useAuthStore.setState({
      auth: {
        ...originalAuth,
        user: {
          id: 2,
          username: 'operator',
          role: ROLE.ADMIN,
          permissions: {
            admin_permissions: {
              channel: { operate: true },
              user_group_config: { read: true },
            },
          },
        },
      },
    })
    render(
      <Harness>
        <ChannelStatusDialog
          action={action}
          target={{ ids: [58] }}
          onOpenChange={vi.fn()}
        />
      </Harness>
    )
    await screen.findByText('customer-b (#37)')
    expect(
      screen.queryByRole('button', { name: combined })
    ).not.toBeInTheDocument()
    expect(screen.getByRole('button', { name: confirm })).toBeEnabled()
  }
)

test('pool rows retain disabled state and inherited priority with long names in the scrollable table', async () => {
  impact.routing_pools[1].priority = null
  impact.routing_pools[1].enabled = false
  impact.routing_pools[1].model = 'long-model-'.repeat(30)
  render(
    <Harness>
      <ChannelStatusDialog target={{ ids: [58] }} onOpenChange={vi.fn()} />
    </Harness>
  )
  await screen.findByText('customer-b (#37)')
  const row = screen.getByRole('row', { name: /customer-b/ })
  expect(within(row).getByText('Inherit')).toBeVisible()
  expect(within(row).getByText('Disabled')).toBeVisible()
  expect(
    within(row).getByText(impact.routing_pools[1].model)
  ).toBeInTheDocument()
  expect(
    screen.getByRole('region', { name: 'Affected routing pools' })
  ).toHaveClass('overflow-x-auto')
})

test.each(['batch', 'tag'] as const)(
  '%s enable reviews pool entries and applies the combined action to the previewed IDs',
  async (mode) => {
    const user = userEvent.setup()
    impact.channels = [
      { id: 58, name: 'Vendor A' },
      { id: 47, name: 'Vendor B' },
    ]
    impact.routing_pools[0].enabled = false
    render(<Harness mode={mode} status={2} />)
    if (mode === 'batch') {
      await user.click(
        screen.getByRole('button', { name: 'Enable selected channels' })
      )
    } else {
      await user.click(screen.getByRole('button', { name: 'Open menu' }))
      await user.click(
        await screen.findByRole('menuitem', { name: 'Enable All' })
      )
    }
    const dialog = await screen.findByRole('alertdialog', {
      name: 'Enable channels?',
    })
    await within(dialog).findByText('customer-b (#37)')
    expect(within(dialog).getByText('Disabled')).toBeVisible()
    expect(api.post).toHaveBeenCalledWith(
      '/api/channel/enable-impact',
      mode === 'batch' ? { ids: [58, 47] } : { tag: 'vendors' },
      expect.anything()
    )
    expect(api.post).not.toHaveBeenCalledWith(
      '/api/channel/status/batch',
      expect.anything(),
      expect.anything()
    )
    await user.click(
      within(dialog).getByRole('button', {
        name: 'Enable channel and pool entries',
      })
    )
    await waitFor(() =>
      expect(api.post).toHaveBeenCalledWith(
        '/api/channel/status/batch',
        { ids: [58, 47], status: 1, update_routing_pools: true },
        expect.anything()
      )
    )
  }
)

test('enable with no selected channels blocks both actions', async () => {
  impact.channels = []
  impact.routing_pools = []
  render(
    <Harness>
      <ChannelStatusDialog
        action='enable'
        target={{ ids: [58] }}
        onOpenChange={vi.fn()}
      />
    </Harness>
  )
  expect(await screen.findByText('No channels in this selection')).toBeVisible()
  expect(screen.getByRole('button', { name: 'Confirm enable' })).toBeDisabled()
  expect(
    screen.getByRole('button', { name: 'Enable channel and pool entries' })
  ).toBeDisabled()
})
