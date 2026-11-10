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
import { render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, expect, test, vi } from 'vitest'

import type {
  RoutingPoolView,
  UserRoutingPoolConfig,
} from '@/features/model-routing/api'
import { api } from '@/lib/api'
import { useAuthStore, type AuthUser } from '@/stores/auth-store'

import { ChannelPoolPanel } from '../channel-pool-panel'
import type { OverviewPool } from '../pool-overview-data'

const clients: QueryClient[] = []
afterEach(() => {
  clients.forEach((client) => client.clear())
  clients.length = 0
  useAuthStore.getState().auth.setUser(null)
})

function fixture() {
  const base: RoutingPoolView = {
    model: 'kimi',
    routing_group: 'vip',
    mode: 'discount',
    revision: 'fresh-default',
    max_ttft_ms: 12000,
    max_tpot_ms: 75,
    tuning: { chat_prefix_enabled: false },
    channels: [
      { channel_id: 27, priority: 100, weight: 0, enabled: true },
      { channel_id: 49, priority: 90, weight: 10, enabled: true },
    ],
    candidates: [
      {
        channel_id: 27,
        channel_name: 'Moonshot',
        priority: 100,
        weight: 0,
        available: true,
      },
      {
        channel_id: 49,
        channel_name: 'Other',
        priority: 90,
        weight: 10,
        available: true,
      },
    ],
  }
  const user: UserRoutingPoolConfig = {
    user_id: 42,
    username: 'Customer',
    user_role: 1,
    model: 'kimi',
    models: ['kimi'],
    routing_group: 'vip',
    revision: 'fresh-user',
    source: 'user',
    configured: true,
    mode: 'priority',
    pool_channels: base.channels.map((row) => ({ ...row })),
    channels: base.candidates.map((row, index) => ({
      ...row,
      default_priority: row.priority,
      model_priority: null,
      inherited_priority: row.priority,
      inherited_weight: row.weight,
      effective_priority: row.priority,
      effective_weight: row.weight,
      priority_override: index === 0 ? 0 : null,
      weight_override: index === 0 ? 0 : null,
      retry_times_override: index === 0 ? 0 : null,
      enabled: true,
    })),
  }
  const common = {
    at: 0,
    loading: false,
    unavailable: false,
    partial: false,
    inherited: false,
    names: new Map([
      [27, 'Moonshot'],
      [49, 'Other'],
    ]),
  }
  const pools: OverviewPool[] = [
    {
      ...common,
      target: { group: 'vip', model: 'kimi' },
      baseConfig: { pool: base, candidates: base.candidates },
    },
    {
      ...common,
      target: { userId: 42, group: 'vip', model: 'kimi' },
      username: 'Customer',
      userConfig: user,
    },
  ]
  const get = vi.spyOn(api, 'get').mockImplementation(async (url) => {
    if (url === '/api/channel/routing-pool') {
      return {
        data: {
          success: true,
          data: { pool: base, candidates: base.candidates },
        },
      }
    }
    if (url === '/api/user/42/channel-routing-overrides') {
      return { data: { success: true, data: user } }
    }
    throw new Error(`Unexpected request ${url}`)
  })
  const put = vi.spyOn(api, 'put').mockImplementation(async (_url, body) => {
    Object.assign(base, body, { revision: 'saved-default' })
    return { data: { success: true, data: base } }
  })
  const patch = vi
    .spyOn(api, 'patch')
    .mockImplementation(async () => ({ data: { success: true } }))
  const post = vi.spyOn(api, 'post').mockImplementation(async (url) => {
    if (url === '/api/channel/disable-impact') {
      return {
        data: {
          success: true,
          data: {
            channels: [{ id: 27, name: 'Moonshot' }],
            customers: [],
            customer_count: 0,
            routing_pools: [],
          },
        },
      }
    }
    if (url === '/api/channel/status/batch') {
      return { data: { success: true, data: 1 } }
    }
    throw new Error(`Unexpected request ${url}`)
  })
  return { pools, base, user, get, put, patch, post }
}

function mount(
  pools: OverviewPool[],
  dirty = false,
  actor: AuthUser = { id: 1, username: 'root', role: 100 }
) {
  useAuthStore.getState().auth.setUser(actor)
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  })
  clients.push(client)
  const onSelect = vi.fn()
  render(
    <QueryClientProvider client={client}>
      <ChannelPoolPanel
        channelId={27}
        name='Moonshot'
        pools={pools}
        isDirty={() => dirty}
        onSelect={onSelect}
        onClose={vi.fn()}
      />
    </QueryClientProvider>
  )
  return { onSelect }
}

test('channel membership lists enabled, disabled and inherited monitored pools and opens the selected pool', async () => {
  const data = fixture()
  for (const channel of data.user.pool_channels ?? []) {
    if (channel.channel_id === 27) channel.enabled = false
  }
  data.user.configured = false
  data.user.source = 'default_pool'
  const { onSelect } = mount(data.pools)
  const panel = screen.getByRole('region', { name: 'Channel routing pools' })
  expect(within(panel).getByText('2 related pools · 1 enabled')).toBeVisible()
  expect(within(panel).getByText('Inherited')).toBeVisible()
  expect(within(panel).getByText('Disabled')).toBeVisible()
  await userEvent.click(
    within(panel).getByRole('button', {
      name: 'View Customer · vip · kimi pool',
    })
  )
  expect(onSelect).toHaveBeenCalledWith({
    userId: 42,
    group: 'vip',
    model: 'kimi',
  })
})

test('current-pool disabling uses fresh revisions and preserves other channels, strategy limits and explicit user override zeros', async () => {
  const data = fixture()
  mount(data.pools)
  await userEvent.click(
    screen.getByRole('button', { name: 'Disable in all related pools' })
  )
  const confirm = screen.getByRole('alertdialog')
  expect(confirm).toHaveTextContent(
    'Default-pool changes also affect users who inherit that pool.'
  )
  expect(data.put).not.toHaveBeenCalled()
  await userEvent.click(
    within(confirm).getByRole('button', { name: 'Disable in reviewed pools' })
  )
  await waitFor(() => expect(data.patch).toHaveBeenCalledTimes(1))
  expect(data.put).toHaveBeenCalledWith(
    '/api/channel/routing-pool',
    expect.objectContaining({
      revision: 'fresh-default',
      mode: 'discount',
      max_ttft_ms: 12000,
      max_tpot_ms: 75,
      tuning: { chat_prefix_enabled: false },
      channels: [
        { channel_id: 27, priority: 100, weight: 0, enabled: false },
        { channel_id: 49, priority: 90, weight: 10, enabled: true },
      ],
    })
  )
  expect(data.patch).toHaveBeenCalledWith(
    '/api/user/42/channel-routing-overrides',
    {
      model: 'kimi',
      routing_group: 'vip',
      revision: 'fresh-user',
      overrides: [
        {
          channel_id: 27,
          priority_override: 0,
          weight_override: 0,
          retry_times_override: 0,
          enabled: false,
        },
        {
          channel_id: 49,
          priority_override: null,
          weight_override: null,
          retry_times_override: null,
          enabled: true,
        },
      ],
    }
  )
  expect(data.post).not.toHaveBeenCalled()
})

test('all-pool scope loads every page, includes unmonitored pools and disables only the reviewed memberships', async () => {
  const data = fixture()
  const outside = {
    ...data.base,
    model: 'unmonitored',
    revision: 'outside-revision',
  }
  data.get.mockImplementation(async (url, options) => {
    if (url === '/api/channel/routing-pools') {
      return {
        data: {
          success: true,
          data: {
            items: options?.params?.page === 1 ? [data.base] : [outside],
            total: 2,
            page: options?.params?.page,
            page_size: 1,
          },
        },
      }
    }
    if (url === '/api/channel/routing-pool') {
      return {
        data: {
          success: true,
          data: {
            pool:
              options?.params?.model === 'unmonitored' ? outside : data.base,
            candidates: [],
          },
        },
      }
    }
    throw new Error(`Unexpected request ${url}`)
  })
  mount(data.pools)
  await userEvent.selectOptions(
    screen.getByRole('combobox', { name: 'Disable scope' }),
    'all'
  )
  await screen.findByRole('button', {
    name: 'View Default configuration · vip · unmonitored pool',
  })
  expect(data.get).toHaveBeenCalledWith(
    '/api/channel/routing-pools',
    expect.objectContaining({
      params: expect.objectContaining({
        scope: 'all',
        channel_id: 27,
        page: 2,
      }),
    })
  )
  await userEvent.click(
    screen.getByRole('button', { name: 'Disable in all related pools' })
  )
  await userEvent.click(
    within(screen.getByRole('alertdialog')).getByRole('button', {
      name: 'Disable in reviewed pools',
    })
  )
  await waitFor(() => expect(data.put).toHaveBeenCalledTimes(2))
  expect(data.put).toHaveBeenCalledWith(
    '/api/channel/routing-pool',
    expect.objectContaining({
      model: 'unmonitored',
      revision: 'outside-revision',
    })
  )
  expect(data.patch).not.toHaveBeenCalled()
  expect(data.post).not.toHaveBeenCalled()
})

test('global disabling reuses the channel impact dialog and leaves pool entries unchanged', async () => {
  const data = fixture()
  mount(data.pools)
  await userEvent.selectOptions(
    screen.getByRole('combobox', { name: 'Disable scope' }),
    'global'
  )
  await userEvent.click(
    screen.getByRole('button', { name: 'Disable channel globally' })
  )
  const confirm = screen.getByRole('alertdialog', { name: 'Disable channels?' })
  expect(confirm).not.toHaveTextContent(
    'Disabling pool entries preserves their priorities and weights.'
  )
  const apply = await within(confirm).findByRole('button', {
    name: 'Confirm disable',
  })
  await waitFor(() => expect(apply).toBeEnabled())
  expect(
    within(confirm).queryByRole('button', {
      name: 'Disable channel and pool entries',
    })
  ).not.toBeInTheDocument()
  await userEvent.click(apply)
  await waitFor(() =>
    expect(data.post).toHaveBeenCalledWith(
      '/api/channel/status/batch',
      { ids: [27], status: 2 },
      expect.anything()
    )
  )
  expect(data.put).not.toHaveBeenCalled()
  expect(data.patch).not.toHaveBeenCalled()
})

test('a failed user update retains a successful default update and reports the failure without retrying writes', async () => {
  const data = fixture()
  data.patch.mockResolvedValue({
    data: { success: false, message: 'Revision conflict' },
  })
  mount(data.pools)
  await userEvent.click(
    screen.getByRole('button', { name: 'Disable in all related pools' })
  )
  await userEvent.click(
    within(screen.getByRole('alertdialog')).getByRole('button', {
      name: 'Disable in reviewed pools',
    })
  )
  expect(await screen.findByRole('alert')).toHaveTextContent(
    'Revision conflict'
  )
  expect(data.base.channels[0].enabled).toBe(false)
  expect(data.put).toHaveBeenCalledTimes(1)
  expect(data.patch).toHaveBeenCalledTimes(1)
})

test.each([true, false])(
  'dirty drafts or missing permissions block batch writes (dirty=%s)',
  async (dirty) => {
    const data = fixture()
    mount(
      data.pools,
      dirty,
      dirty ? undefined : { id: 2, username: 'reader', role: 10 }
    )
    expect(
      screen.getByRole('button', { name: 'Disable in all related pools' })
    ).toBeDisabled()
    await userEvent.selectOptions(
      screen.getByRole('combobox', { name: 'Disable scope' }),
      'global'
    )
    expect(
      screen.getByRole('button', { name: 'Disable channel globally' })
    ).toBeDisabled()
    expect(data.put).not.toHaveBeenCalled()
    expect(data.patch).not.toHaveBeenCalled()
    expect(data.post).not.toHaveBeenCalled()
  }
)

test('unknown pool configuration blocks scoped disabling instead of treating missing data as an empty pool', () => {
  const data = fixture()
  data.pools.push({
    ...data.pools[0],
    target: { group: 'vip', model: 'unknown' },
    baseConfig: undefined,
    configError: true,
  })
  mount(data.pools)
  expect(
    screen.getByText(
      'Some pool configurations could not be loaded. Refresh before disabling.'
    )
  ).toBeVisible()
  expect(
    screen.getByRole('button', { name: 'Disable in all related pools' })
  ).toBeDisabled()
})

test('an inherited user pool gets a scoped override preserving effective priorities and weights', async () => {
  const data = fixture()
  data.user.configured = false
  data.user.source = 'default_pool'
  mount([data.pools[1]])
  await userEvent.click(
    screen.getByRole('button', { name: 'Disable in all related pools' })
  )
  await userEvent.click(
    within(screen.getByRole('alertdialog')).getByRole('button', {
      name: 'Disable in reviewed pools',
    })
  )
  await waitFor(() => expect(data.patch).toHaveBeenCalledTimes(1))
  expect(data.patch).toHaveBeenCalledWith(
    '/api/user/42/channel-routing-overrides',
    expect.objectContaining({
      overrides: [
        {
          channel_id: 27,
          priority_override: 100,
          weight_override: 0,
          retry_times_override: 0,
          enabled: false,
        },
        {
          channel_id: 49,
          priority_override: 90,
          weight_override: 10,
          retry_times_override: null,
          enabled: true,
        },
      ],
    })
  )
  expect(data.put).not.toHaveBeenCalled()
})
