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
  createRouter,
  RouterProvider,
} from '@tanstack/react-router'
import {
  act,
  fireEvent,
  render,
  screen,
  waitFor,
  within,
} from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, beforeEach, expect, test, vi } from 'vitest'

import { api } from '@/lib/api'
import { useAuthStore } from '@/stores/auth-store'

import { userRoutingPoolKeys, type UserRoutingPoolConfig } from '../api'
import { RoutingScopeSelect } from '../scope-select'
import { UserRoutingWorkspace } from '../user-workspace'

const clients: QueryClient[] = []
beforeEach(() =>
  useAuthStore.getState().auth.setUser({ id: 1, username: 'root', role: 100 })
)
afterEach(() => {
  clients.forEach((client) => client.clear())
  useAuthStore.getState().auth.setUser(null)
})

function fixture(): UserRoutingPoolConfig {
  return {
    user_id: 42,
    username: 'acme',
    user_role: 1,
    model: 'kimi-k3',
    models: ['kimi-k3'],
    revision: 'original',
    configured: false,
    source: 'default_pool',
    mode: 'smart',
    routing_group: 'vip',
    channels: [
      {
        channel_id: 1,
        channel_name: 'Primary',
        default_priority: 10,
        model_priority: null,
        inherited_priority: 10,
        inherited_weight: 20,
        priority_override: null,
        weight_override: null,
        effective_priority: 100,
        effective_weight: 7,
        enabled: true,
        available: true,
      },
      {
        channel_id: 2,
        channel_name: 'Backup',
        default_priority: 5,
        model_priority: null,
        inherited_priority: 5,
        inherited_weight: 8,
        priority_override: null,
        weight_override: null,
        effective_priority: 0,
        effective_weight: 0,
        enabled: true,
        available: true,
      },
    ],
    pool_channels: [
      { channel_id: 1, priority: 100, weight: 7, enabled: true },
      { channel_id: 2, priority: 0, weight: 0, enabled: false },
    ],
  }
}

function mockPool(config = fixture()) {
  const state = { config }
  vi.spyOn(api, 'get').mockImplementation(async (url, options) => {
    if (url === '/api/channel/monitoring') {
      return {
        data: {
          success: true,
          data: [
            {
              channel_id: 1,
              state: 'disabled',
              buckets: [],
              cache: { hit_rate: 50, truncated: false },
            },
          ],
        },
      }
    }
    if (url === '/api/user/42/routing-monitor') {
      return {
        data: {
          success: true,
          data: {
            generated_at: Date.now(),
            legacy_enabled: false,
            coverage: 'local',
            scope: 'user',
            user_id: 42,
            partial: false,
            nodes: [
              {
                id: 'local',
                name: 'gateway-01',
                local: true,
                updated_at: Date.now(),
                routes: [],
              },
            ],
          },
        },
      }
    }
    expect(url).toBe('/api/user/42/channel-routing-overrides')
    expect(options?.params?.view).toBe('pool')
    const data = options?.params?.model
      ? state.config
      : {
          ...state.config,
          model: '',
          key_groups: { vip: 'VIP' },
          configured_models: state.config.configured ? ['kimi-k3'] : [],
          configured_key_groups: state.config.configured
            ? { 'kimi-k3': ['vip'] }
            : {},
          model_groups: { 'kimi-k3': ['vip'] },
        }
    return { data: { success: true, data } }
  })
  return state
}

test('user monitoring retains the same unsaved override and returns focus without changing the selected user pool', async () => {
  mockPool()
  const patch = vi
    .spyOn(api, 'patch')
    .mockResolvedValue({ data: { success: true } })
  const { card } = await mountWorkspace()
  await userEvent.click(
    await within(card).findByRole('button', { name: 'Create user override' })
  )
  const priority = within(card).getByRole('spinbutton', {
    name: 'Priority for Primary',
  })
  fireEvent.change(priority, { target: { value: '0' } })
  const trigger = within(card).getByRole('button', { name: 'Monitoring view' })
  await userEvent.click(trigger)
  const monitor = screen.getByRole('dialog', {
    name: 'kimi-k3 · vip · acme #42',
  })
  expect(
    within(monitor).getByRole('spinbutton', { name: 'Priority for Primary' })
  ).toBe(priority)
  expect(priority).toHaveValue(0)
  expect(
    await within(monitor).findByLabelText('Cache hit rate: 50%')
  ).toBeVisible()
  await within(monitor).findByText('No adaptive routing observations')
  expect(patch).not.toHaveBeenCalled()
  await userEvent.click(
    within(monitor).getByRole('button', { name: 'Back to card view' })
  )
  expect(trigger).toHaveFocus()
  expect(
    within(card).getByRole('spinbutton', { name: 'Priority for Primary' })
  ).toBe(priority)
  expect(priority).toHaveValue(0)
  await userEvent.click(within(card).getByRole('button', { name: 'Save' }))
  await waitFor(() =>
    expect(patch).toHaveBeenCalledWith(
      '/api/user/42/channel-routing-overrides',
      expect.objectContaining({
        model: 'kimi-k3',
        routing_group: 'vip',
        revision: 'original',
        reset: false,
        overrides: [
          {
            channel_id: 1,
            priority_override: 0,
            weight_override: 7,
            enabled: true,
          },
          {
            channel_id: 2,
            priority_override: 0,
            weight_override: 0,
            enabled: false,
          },
        ],
      })
    )
  )
})

async function mountWorkspace() {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  })
  clients.push(client)
  const dirty = vi.fn()
  const route = createRootRoute({
    component: () => (
      <QueryClientProvider client={client}>
        <UserRoutingWorkspace userId={42} onDirtyChange={dirty} />
      </QueryClientProvider>
    ),
  })
  const router = createRouter({
    routeTree: route,
    history: createMemoryHistory({ initialEntries: ['/'] }),
  })
  render(<RouterProvider router={router} />)
  return {
    client,
    dirty,
    card: await screen.findByRole('region', { name: 'kimi-k3 · vip' }),
  }
}

test('shows the user identity and effective inherited priorities and weights without writing overrides', async () => {
  mockPool()
  const { card } = await mountWorkspace()
  expect(within(card).getByText('vip')).toBeVisible()
  expect(await screen.findByText('acme')).toBeVisible()
  expect(within(card).getByText('Default routing pool')).toBeVisible()
  expect(
    within(card).getByText('Strategy: Smart routing (inherited)')
  ).toBeVisible()
  await waitFor(() =>
    expect(
      within(card).getByRole('spinbutton', { name: 'Priority for Primary' })
    ).toHaveValue(100)
  )
  expect(
    within(card).getByRole('spinbutton', { name: 'Weight for Primary' })
  ).toHaveValue(7)
  expect(
    within(card).getByRole('spinbutton', { name: 'Weight for Primary' })
  ).toBeDisabled()
  expect(
    within(card).getByRole('switch', { name: 'Enable Backup in this pool' })
  ).not.toBeChecked()
  expect(
    within(card).queryByRole('button', { name: 'Edit strategy' })
  ).not.toBeInTheDocument()
  expect(
    within(card).queryByRole('button', { name: 'Remove 1' })
  ).not.toBeInTheDocument()
  const model = screen.getByRole('combobox', { name: 'Search models' })
  await userEvent.click(model)
  await userEvent.clear(model)
  await userEvent.type(model, 'kimi')
  await userEvent.click(await screen.findByRole('option', { name: 'kimi-k3' }))
  expect(model).toHaveValue('kimi-k3')
  await userEvent.click(model)
  await userEvent.click(
    await screen.findByRole('option', { name: 'All models' })
  )
  expect(model).toHaveValue('All models')
})

test('user pool cards fill each responsive column without a fixed maximum width or splitting across columns', async () => {
  mockPool()
  const { card } = await mountWorkspace()
  const list = screen.getByRole('list', { name: 'User routing pools' })
  expect(list).toHaveClass('columns-[24rem]', 'gap-4')
  expect(list).not.toHaveClass('grid')
  const item = within(list).getByRole('listitem')
  expect(item).toHaveClass(
    'inline-block',
    'w-full',
    'break-inside-avoid',
    'mb-4'
  )
  expect(item.className).not.toMatch(/max-w-/)
  expect(item).toContainElement(card)
})

test('creates a user snapshot with explicit zero priority and weight while preserving excluded default channels', async () => {
  mockPool()
  const patch = vi
    .spyOn(api, 'patch')
    .mockResolvedValue({ data: { success: true } })
  const { card, dirty } = await mountWorkspace()
  expect(
    screen.queryByText(
      'Save or reset edited cards before changing filters or views.'
    )
  ).not.toBeInTheDocument()
  fireEvent.click(
    await within(card).findByRole('button', { name: 'Create user override' })
  )
  fireEvent.change(
    within(card).getByRole('spinbutton', { name: 'Priority for Primary' }),
    { target: { value: '0' } }
  )
  fireEvent.change(
    within(card).getByRole('spinbutton', { name: 'Weight for Primary' }),
    { target: { value: '0' } }
  )
  expect(screen.getByRole('combobox', { name: 'Search models' })).toBeDisabled()
  await waitFor(() => expect(dirty).toHaveBeenLastCalledWith(true))
  expect(
    screen.getByText(
      'Save or reset edited cards before changing filters or views.'
    )
  ).toBeVisible()
  fireEvent.click(within(card).getByRole('button', { name: 'Save' }))
  await waitFor(() =>
    expect(patch).toHaveBeenCalledWith(
      '/api/user/42/channel-routing-overrides',
      {
        model: 'kimi-k3',
        routing_group: 'vip',
        revision: 'original',
        reset: false,
        overrides: [
          {
            channel_id: 1,
            priority_override: 0,
            weight_override: 0,
            enabled: true,
          },
          {
            channel_id: 2,
            priority_override: 0,
            weight_override: 0,
            enabled: false,
          },
        ],
      }
    )
  )
})

test('clearing a user weight explicitly restores inheritance and keeps the priority override', async () => {
  const config = fixture()
  config.configured = true
  config.source = 'user'
  config.channels[0] = {
    ...config.channels[0],
    configured: true,
    priority_override: 50,
    weight_override: 0,
  }
  config.pool_channels = config.pool_channels?.map((row) =>
    row.channel_id === 1 ? { ...row, priority: 50, weight: 0 } : row
  )
  mockPool(config)
  const patch = vi
    .spyOn(api, 'patch')
    .mockResolvedValue({ data: { success: true } })
  const { card } = await mountWorkspace()
  fireEvent.click(
    await within(card).findByRole('button', { name: 'Edit user override' })
  )
  fireEvent.change(
    within(card).getByRole('spinbutton', { name: 'Weight for Primary' }),
    { target: { value: '' } }
  )
  expect(
    within(card).getByRole('spinbutton', { name: 'Weight for Primary' })
  ).toHaveAttribute('placeholder', '20')
  fireEvent.click(within(card).getByRole('button', { name: 'Save' }))
  await waitFor(() =>
    expect(patch).toHaveBeenCalledWith(
      '/api/user/42/channel-routing-overrides',
      expect.objectContaining({
        overrides: [
          {
            channel_id: 1,
            priority_override: 50,
            weight_override: null,
            enabled: true,
          },
          {
            channel_id: 2,
            priority_override: null,
            weight_override: null,
            enabled: false,
          },
        ],
      })
    )
  )
})

test('restoring inheritance requires confirmation and resets only this model and public group', async () => {
  mockPool({ ...fixture(), configured: true, source: 'user' })
  const patch = vi
    .spyOn(api, 'patch')
    .mockResolvedValue({ data: { success: true } })
  const { card } = await mountWorkspace()
  fireEvent.click(
    await within(card).findByRole('button', {
      name: 'Clear and use default pool',
    })
  )
  fireEvent.click(
    within(screen.getByRole('alertdialog')).getByRole('button', {
      name: 'Cancel',
    })
  )
  expect(patch).not.toHaveBeenCalled()
  fireEvent.click(
    within(card).getByRole('button', { name: 'Clear and use default pool' })
  )
  fireEvent.click(
    within(screen.getByRole('alertdialog')).getByRole('button', {
      name: 'Clear and use default pool',
    })
  )
  await waitFor(() =>
    expect(patch).toHaveBeenCalledWith(
      '/api/user/42/channel-routing-overrides',
      {
        model: 'kimi-k3',
        routing_group: 'vip',
        revision: 'original',
        reset: true,
        overrides: [],
      }
    )
  )
})

test('a rejected save preserves the draft and cancel unlocks filters without changing the server', async () => {
  mockPool()
  vi.spyOn(api, 'patch').mockRejectedValue({
    response: {
      status: 409,
      data: { success: false, message: 'configuration changed' },
    },
  })
  const { card } = await mountWorkspace()
  fireEvent.click(
    await within(card).findByRole('button', { name: 'Create user override' })
  )
  fireEvent.change(
    within(card).getByRole('spinbutton', { name: 'Weight for Primary' }),
    { target: { value: '123' } }
  )
  fireEvent.click(within(card).getByRole('button', { name: 'Save' }))
  expect(await within(card).findByRole('alert')).toHaveTextContent(
    'Changes were not saved'
  )
  expect(
    within(card).getByRole('spinbutton', { name: 'Weight for Primary' })
  ).toHaveValue(123)
  fireEvent.click(within(card).getByRole('button', { name: 'Cancel' }))
  expect(screen.getByRole('combobox', { name: 'Search models' })).toBeEnabled()
  expect(
    within(card).getByRole('spinbutton', { name: 'Weight for Primary' })
  ).toHaveValue(7)
})

test('read-only operators can inspect inherited values but cannot create overrides', async () => {
  useAuthStore.getState().auth.setUser({
    id: 1,
    username: 'reader',
    role: 10,
    permissions: {
      admin_permissions: { user_group_config: { read: true, write: false } },
    },
  })
  mockPool()
  const { card } = await mountWorkspace()
  expect(
    await within(card).findByRole('button', { name: 'Create user override' })
  ).toBeDisabled()
})

test('background refresh preserves the original draft revision so concurrent edits are rejected', async () => {
  mockPool()
  const patch = vi.spyOn(api, 'patch').mockRejectedValue({
    response: {
      status: 409,
      data: { success: false, message: 'configuration changed' },
    },
  })
  const { card, client } = await mountWorkspace()
  fireEvent.click(
    await within(card).findByRole('button', { name: 'Create user override' })
  )
  fireEvent.change(
    within(card).getByRole('spinbutton', { name: 'Weight for Primary' }),
    { target: { value: '42' } }
  )
  act(() =>
    client.setQueryData(userRoutingPoolKeys.detail(42, 'vip', 'kimi-k3'), {
      ...fixture(),
      revision: 'changed-by-another-admin',
    })
  )
  fireEvent.click(within(card).getByRole('button', { name: 'Save' }))
  await waitFor(() =>
    expect(patch).toHaveBeenCalledWith(
      '/api/user/42/channel-routing-overrides',
      expect.objectContaining({ revision: 'original' })
    )
  )
  expect(await within(card).findByRole('alert')).toBeVisible()
  expect(
    within(card).getByRole('spinbutton', { name: 'Weight for Primary' })
  ).toHaveValue(42)
})

test('invalid fractional weights are rejected before saving', async () => {
  mockPool()
  const patch = vi
    .spyOn(api, 'patch')
    .mockResolvedValue({ data: { success: true } })
  const { card } = await mountWorkspace()
  fireEvent.click(
    await within(card).findByRole('button', { name: 'Create user override' })
  )
  fireEvent.change(
    within(card).getByRole('spinbutton', { name: 'Weight for Primary' }),
    { target: { value: '1.5' } }
  )
  fireEvent.click(within(card).getByRole('button', { name: 'Save' }))
  expect(await within(card).findByRole('alert')).toBeVisible()
  expect(patch).not.toHaveBeenCalled()
})

test('a failed background refresh keeps the unsaved user priority and weight draft visible', async () => {
  mockPool()
  const { card, client } = await mountWorkspace()
  fireEvent.click(
    await within(card).findByRole('button', { name: 'Create user override' })
  )
  fireEvent.change(
    within(card).getByRole('spinbutton', { name: 'Weight for Primary' }),
    { target: { value: '42' } }
  )
  vi.mocked(api.get).mockRejectedValue(new Error('temporary network failure'))
  await act(async () => {
    await client.refetchQueries({
      queryKey: userRoutingPoolKeys.detail(42, 'vip', 'kimi-k3'),
      exact: true,
    })
  })
  expect(
    await within(card).findByRole('button', { name: 'Retry' })
  ).toBeVisible()
  expect(
    within(card).getByRole('spinbutton', { name: 'Weight for Primary' })
  ).toHaveValue(42)
  expect(within(card).getByRole('button', { name: 'Save' })).toBeEnabled()
  expect(screen.getByRole('combobox', { name: 'Search models' })).toBeDisabled()
})

test('scope search selects an authorized user and disabled scope controls cannot discard a draft', async () => {
  const operator = userEvent.setup()
  vi.spyOn(api, 'get').mockResolvedValue({
    data: {
      success: true,
      data: {
        items: [
          { id: 42, username: 'acme', role: 1 },
          { id: 43, username: 'deleted', role: 1, DeletedAt: '2026-01-01' },
        ],
      },
    },
  })
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  clients.push(client)
  const changed = vi.fn()
  const view = render(
    <QueryClientProvider client={client}>
      <RoutingScopeSelect disabled={false} onChange={changed} />
    </QueryClientProvider>
  )
  await operator.click(
    screen.getByRole('combobox', { name: 'Select routing user' })
  )
  await operator.click(
    await screen.findByRole('option', { name: 'acme · #42' })
  )
  expect(changed).toHaveBeenCalledWith(42)
  view.rerender(
    <QueryClientProvider client={client}>
      <RoutingScopeSelect userId={42} disabled onChange={changed} />
    </QueryClientProvider>
  )
  expect(
    screen.getByRole('combobox', { name: 'Select routing user' })
  ).toBeDisabled()
  expect(
    screen.getByRole('button', { name: 'Default configuration' })
  ).toBeDisabled()
})

test('a user can add an enabled channel outside the default pool and save the complete membership', async () => {
  const config = fixture()
  config.channels.push({
    ...config.channels[0],
    channel_id: 3,
    channel_name: 'Outside default',
    inherited_priority: 12,
    inherited_weight: 30,
  })
  mockPool(config)
  const patch = vi
    .spyOn(api, 'patch')
    .mockResolvedValue({ data: { success: true } })
  const { card } = await mountWorkspace()
  await userEvent.click(
    await within(card).findByRole('button', { name: 'Create user override' })
  )
  await userEvent.click(
    within(card).getByRole('combobox', { name: 'Select channel' })
  )
  await userEvent.click(
    await screen.findByRole('option', { name: 'Outside default #3' })
  )
  await userEvent.click(within(card).getByRole('button', { name: 'Add' }))
  expect(
    within(card).getByRole('spinbutton', {
      name: 'Priority for Outside default',
    })
  ).toHaveValue(12)
  await userEvent.click(within(card).getByRole('button', { name: 'Save' }))
  await waitFor(() =>
    expect(patch).toHaveBeenCalledWith(
      '/api/user/42/channel-routing-overrides',
      expect.objectContaining({
        overrides: expect.arrayContaining([
          expect.objectContaining({ channel_id: 1 }),
          expect.objectContaining({ channel_id: 2 }),
          expect.objectContaining({
            channel_id: 3,
            priority_override: 12,
            weight_override: 30,
            enabled: true,
          }),
        ]),
      })
    )
  )
})

test('clearing a configured user pool confirms that the default pool will be used', async () => {
  mockPool({ ...fixture(), configured: true, source: 'user' })
  const patch = vi
    .spyOn(api, 'patch')
    .mockResolvedValue({ data: { success: true } })
  const { card } = await mountWorkspace()
  await userEvent.click(
    await within(card).findByRole('button', {
      name: 'Clear and use default pool',
    })
  )
  const dialog = screen.getByRole('alertdialog')
  expect(
    within(dialog).getByText(
      'Clear this user pool and use the default routing pool for this model and public group?'
    )
  ).toBeVisible()
  expect(patch).not.toHaveBeenCalled()
  await userEvent.click(
    within(dialog).getByRole('button', { name: 'Clear and use default pool' })
  )
  await waitFor(() =>
    expect(patch).toHaveBeenCalledWith(
      '/api/user/42/channel-routing-overrides',
      expect.objectContaining({
        routing_group: 'vip',
        revision: 'original',
        reset: true,
        overrides: [],
      })
    )
  )
})

test('a nonempty user pool with every member disabled warns that requests will fail', async () => {
  mockPool({
    ...fixture(),
    configured: true,
    source: 'user',
    pool_channels: [
      { channel_id: 1, priority: 10, weight: 20, enabled: false },
    ],
    channels: [{ ...fixture().channels[0], configured: true, enabled: false }],
  })
  const { card } = await mountWorkspace()
  expect(await within(card).findByRole('alert')).toHaveTextContent(
    'This user pool has no enabled channels. Requests for this model will fail.'
  )
})

test('removing the final user member explains default fallback and saves an empty pool', async () => {
  const config = fixture()
  mockPool({
    ...config,
    configured: true,
    source: 'user',
    channels: [{ ...config.channels[0], configured: true }],
    pool_channels: [{ channel_id: 1, priority: 100, weight: 7, enabled: true }],
  })
  const patch = vi
    .spyOn(api, 'patch')
    .mockResolvedValue({ data: { success: true } })
  const { card } = await mountWorkspace()
  await userEvent.click(
    await within(card).findByRole('button', { name: 'Edit user override' })
  )
  await userEvent.click(within(card).getByRole('button', { name: 'Remove 1' }))
  const dialog = screen.getByRole('alertdialog')
  await userEvent.click(within(dialog).getByRole('button', { name: 'Remove' }))
  expect(within(card).getByRole('status')).toHaveTextContent(
    'This user pool is empty. Saving will restore the default routing pool.'
  )
  expect(within(card).queryByRole('alert')).not.toBeInTheDocument()
  await userEvent.click(within(card).getByRole('button', { name: 'Save' }))
  await waitFor(() =>
    expect(patch).toHaveBeenCalledWith(
      '/api/user/42/channel-routing-overrides',
      expect.objectContaining({ routing_group: 'vip', overrides: [] })
    )
  )
})
