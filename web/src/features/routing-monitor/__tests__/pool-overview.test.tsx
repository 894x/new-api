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
  fireEvent,
  render,
  screen,
  waitFor,
  within,
} from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { useState, type ReactNode } from 'react'
import { afterEach, expect, test, vi } from 'vitest'

import type {
  RoutingPoolView,
  UserRoutingPoolConfig,
} from '@/features/model-routing/api'
import { MonitorPoolEditor } from '@/features/model-routing/monitor-pool-editor'
import { PoolCard } from '@/features/model-routing/pool-card'
import { PoolMonitorDisplay } from '@/features/model-routing/pool-monitor-display'
import { UserPoolCard } from '@/features/model-routing/user-pool-card'
import { api } from '@/lib/api'
import { useAuthStore, type AuthUser } from '@/stores/auth-store'

import type { MonitorRoute, MonitorSnapshot } from '../api'
import { PoolOverview } from '../pool-overview'
import { overviewFlows, type OverviewPool } from '../pool-overview-data'

const clients: QueryClient[] = []
afterEach(() => {
  clients.forEach((client) => client.clear())
  clients.length = 0
  useAuthStore.getState().auth.setUser(null)
})

function poolFixture(model = 'kimi-a'): RoutingPoolView {
  return {
    routing_group: 'standard',
    model,
    mode: 'priority',
    revision: 'original',
    channels: [{ channel_id: 49, priority: 10, weight: 10, enabled: true }],
    candidates: [
      {
        channel_id: 49,
        channel_name: 'Shared upstream',
        priority: 10,
        weight: 10,
        available: true,
      },
    ],
  }
}

function routeFixture(model = 'kimi-a', count = 25): MonitorRoute {
  return {
    group: 'standard',
    model,
    mode: 'priority',
    policy: '',
    dynamic: false,
    last_seen: Date.now(),
    window_seconds: 60,
    selection_count: count,
    truncated: false,
    history: [],
    events: [],
    requests: [],
    channels: [
      {
        channel_id: 49,
        status: 1,
        active: true,
        priority: 10,
        weight: 10,
        effective_cost_discount: null,
        target_share: 1,
        selected_share: 1,
        selection_count: count,
        probe: false,
        degraded: false,
        unverified: false,
        ttft_ms: null,
        tpot_ms: null,
        sample_count: 0,
        ttft_sample_count: 0,
        tpot_sample_count: 0,
      },
    ],
  }
}

function snapshotFixture(): MonitorSnapshot {
  const now = Date.now()
  return {
    generated_at: now,
    legacy_enabled: false,
    coverage: 'cluster',
    scope: 'base',
    partial: false,
    nodes: [
      {
        id: 'gateway',
        name: 'Gateway',
        local: true,
        updated_at: now,
        routes: [routeFixture(), routeFixture('kimi-b', 15)],
      },
    ],
  }
}

function userFixture(): UserRoutingPoolConfig {
  return {
    user_id: 42,
    username: 'customer',
    user_role: 1,
    model: 'kimi-a',
    models: ['kimi-a'],
    routing_group: 'standard',
    key_groups: { standard: 'Standard' },
    model_groups: { 'kimi-a': ['standard'] },
    revision: 'user-revision',
    configured: true,
    source: 'user',
    mode: 'priority',
    pool_channels: poolFixture().channels,
    channels: [
      {
        channel_id: 49,
        channel_name: 'Shared upstream',
        default_priority: 10,
        model_priority: null,
        inherited_priority: 10,
        inherited_weight: 10,
        priority_override: null,
        weight_override: null,
        effective_priority: 10,
        effective_weight: 10,
        enabled: true,
        available: true,
      },
    ],
  }
}

function mockApi() {
  const pools = new Map([
    ['kimi-a', poolFixture()],
    ['kimi-b', poolFixture('kimi-b')],
  ])
  const user = userFixture()
  const get = vi.spyOn(api, 'get').mockImplementation(async (url, options) => {
    let data: unknown
    if (url.endsWith('/routing-monitor')) data = snapshotFixture()
    else if (url === '/api/channel/monitoring') data = []
    else if (url.endsWith('/channel-routing-overrides')) data = user
    else if (url.endsWith('/catalog')) {
      data = { groups: { standard: 'Standard' }, models: ['kimi-a', 'kimi-b'] }
    } else if (url === '/api/channel/routing-pool') {
      const pool = pools.get(options?.params?.model)
      data = {
        pool,
        candidates: pool?.candidates ?? [],
        effective_mode: pool?.mode,
      }
    } else throw new Error(`Unexpected API: ${url}`)
    return { data: { success: true, data } }
  })
  const put = vi.spyOn(api, 'put').mockImplementation(async (_url, body) => {
    const saved = { ...(body as RoutingPoolView), revision: 'saved' }
    pools.set(saved.model, saved)
    return { data: { success: true, data: saved } }
  })
  const patch = vi
    .spyOn(api, 'patch')
    .mockResolvedValue({ data: { success: true } })
  return { get, put, patch }
}

function mount(
  children: ReactNode,
  actor: AuthUser = { id: 1, username: 'root', role: 100 }
) {
  useAuthStore.getState().auth.setUser(actor)
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false, gcTime: Infinity } },
  })
  clients.push(client)
  return render(
    <QueryClientProvider client={client}>{children}</QueryClientProvider>
  )
}

function Workspace() {
  const [initialDirty, setInitialDirty] = useState(false)
  const [extraDirty, setExtraDirty] = useState(false)
  return (
    <>
      <output aria-label='Additional pool drafts'>{String(extraDirty)}</output>
      <PoolMonitorDisplay
        open
        onOpenChange={() => {}}
        model='kimi-a'
        group='standard'
        initialDirty={initialDirty}
        onDirtyChange={setExtraDirty}
      >
        <PoolCard
          pool={poolFixture()}
          canWrite
          embedded
          onDirtyChange={setInitialDirty}
        />
      </PoolMonitorDisplay>
    </>
  )
}

function overviewPool(userId: number, count: number, at = 100): OverviewPool {
  return {
    target: { userId, group: 'standard', model: 'kimi-a' },
    names: new Map([[49, 'Shared upstream']]),
    route: routeFixture('kimi-a', count),
    at,
    loading: false,
    unavailable: false,
    partial: false,
    inherited: true,
  }
}

test('inherited users share one effective flow and use the latest snapshot without adding duplicate counts', () => {
  const flows = overviewFlows([overviewPool(42, 25), overviewPool(43, 30, 200)])
  expect(flows).toHaveLength(1)
  expect(flows[0].targets.map((pool) => pool.target.userId)).toEqual([42, 43])
  expect(flows[0].route.selection_count).toBe(30)
  expect(flows[0].route.channels[0].selection_count).toBe(30)
})

test('different user policies remain separate while failed snapshots contribute no observed traffic', () => {
  const user = overviewPool(42, 25)
  user.route = { ...routeFixture(), policy: 'user:42' }
  const failed = { ...overviewPool(44, 900), unavailable: true }
  const flows = overviewFlows([user, overviewPool(43, 30), failed])
  expect(flows).toHaveLength(2)
  expect(flows.map((flow) => flow.route.selection_count)).toEqual([25, 30])
})

test('two pools merge a shared channel, expand it per pool, and keep channel quality scoped to its pool', async () => {
  mockApi()
  const onSelect = vi.fn()
  mount(
    <PoolOverview
      targets={[
        { group: 'standard', model: 'kimi-a' },
        { group: 'standard', model: 'kimi-b' },
      ]}
      selectedTarget={{ group: 'standard', model: 'kimi-a' }}
      onSelect={onSelect}
      onAdd={vi.fn()}
      onRemove={vi.fn()}
      isDirty={() => false}
    />
  )
  const channel = await screen.findByRole('button', { name: 'View channel 49' })
  expect(channel).toHaveTextContent('40')
  await userEvent.click(channel)
  const quality = screen.getByLabelText('Pools using this channel')
  expect(within(quality).getByText(/kimi-a/)).toBeVisible()
  expect(within(quality).getByText(/kimi-b/)).toBeVisible()
  expect(quality).toHaveTextContent('TTFT —')
  await userEvent.click(
    screen.getByRole('button', { name: 'Channels by pool' })
  )
  expect(
    screen.getAllByRole('button', { name: 'View channel 49' })
  ).toHaveLength(2)
  fireEvent.change(
    screen.getByRole('textbox', { name: 'Search monitoring pools' }),
    { target: { value: 'no-matching-model' } }
  )
  expect(screen.getByText('No routing observations')).toBeVisible()
  expect(
    screen.queryByRole('button', { name: 'View channel 49' })
  ).not.toBeInTheDocument()
})

test('adding a pool opens the existing editor, keeps its draft when switching pools, and saves through the existing endpoint', async () => {
  const mocks = mockApi()
  mount(<Workspace />)
  await userEvent.click(
    screen.getByRole('button', { name: 'Multi-pool overview' })
  )
  await screen.findByRole('button', { name: 'View channel 49' })
  await userEvent.click(
    screen.getByRole('button', { name: 'Add monitoring pool' })
  )
  const picker = screen.getByRole('dialog', { name: 'Add monitoring pool' })
  await userEvent.click(
    within(picker).getByRole('combobox', { name: 'Select model and group' })
  )
  await userEvent.click(await screen.findByRole('button', { name: 'Standard' }))
  await userEvent.click(await screen.findByRole('option', { name: 'kimi-b' }))
  await userEvent.click(
    within(picker).getByRole('button', { name: 'Start monitoring' })
  )
  const weight = await screen.findByRole('spinbutton', {
    name: 'Weight for Shared upstream',
  })
  fireEvent.change(weight, { target: { value: '33' } })
  await waitFor(() =>
    expect(screen.getByLabelText('Additional pool drafts')).toHaveTextContent(
      'true'
    )
  )
  const chips = screen.getByLabelText('Monitoring pools')
  await userEvent.click(
    within(chips).getByRole('button', {
      name: 'Default configuration · standard · kimi-a',
    })
  )
  expect(
    screen.getByRole('spinbutton', { name: 'Weight for Shared upstream' })
  ).toHaveValue(10)
  await userEvent.click(
    within(chips).getByRole('button', {
      name: 'Default configuration · standard · kimi-b',
    })
  )
  expect(
    screen.getByRole('spinbutton', { name: 'Weight for Shared upstream' })
  ).toBe(weight)
  expect(weight).toHaveValue(33)
  expect(
    within(chips).getByRole('button', {
      name: 'Remove Default configuration · kimi-b from monitoring',
    })
  ).toBeDisabled()
  await userEvent.click(screen.getByRole('button', { name: 'Save' }))
  await waitFor(() =>
    expect(mocks.put).toHaveBeenCalledWith(
      '/api/channel/routing-pool',
      expect.objectContaining({
        model: 'kimi-b',
        routing_group: 'standard',
        channels: [expect.objectContaining({ weight: 33 })],
      })
    )
  )
  await waitFor(() =>
    expect(screen.getByLabelText('Additional pool drafts')).toHaveTextContent(
      'false'
    )
  )
})

test('a failed refresh removes stale flow counts instead of continuing to present them as live', async () => {
  const mocks = mockApi()
  mount(
    <PoolOverview
      targets={[{ group: 'standard', model: 'kimi-a' }]}
      selectedTarget={{ group: 'standard', model: 'kimi-a' }}
      onSelect={vi.fn()}
      onAdd={vi.fn()}
      onRemove={vi.fn()}
      isDirty={() => false}
    />
  )
  await screen.findByRole('button', { name: 'View channel 49' })
  mocks.get.mockRejectedValue(new Error('monitoring unavailable'))
  await userEvent.click(screen.getByRole('button', { name: 'Refresh' }))
  await screen.findByText(
    'Some monitoring data is unavailable. Counts may be incomplete.'
  )
  await waitFor(() =>
    expect(
      screen.queryByRole('button', { name: 'View channel 49' })
    ).not.toBeInTheDocument()
  )
})

test('an embedded user pool retains its existing edit and save operations without opening another monitor', async () => {
  const mocks = mockApi()
  mount(
    <UserPoolCard
      embedded
      userId={42}
      model='kimi-a'
      group='standard'
      groupLabel='Standard'
      onDirtyChange={vi.fn()}
    />
  )
  await userEvent.click(
    await screen.findByRole('button', { name: 'Edit user override' })
  )
  expect(screen.getByText('customer · #42')).toBeVisible()
  await userEvent.click(
    screen.getByRole('switch', { name: 'Enable Shared upstream in this pool' })
  )
  await userEvent.click(screen.getByRole('button', { name: 'Save' }))
  await waitFor(() =>
    expect(mocks.patch).toHaveBeenCalledWith(
      '/api/user/42/channel-routing-overrides',
      expect.objectContaining({ model: 'kimi-a', routing_group: 'standard' })
    )
  )
  expect(
    screen.queryByRole('button', { name: 'Monitoring view' })
  ).not.toBeInTheDocument()
})

test('read-only embedded pools keep editing disabled', async () => {
  mockApi()
  mount(
    <UserPoolCard
      embedded
      readOnly
      userId={42}
      model='kimi-a'
      group='standard'
      groupLabel='Standard'
      onDirtyChange={vi.fn()}
    />
  )
  expect(
    await screen.findByRole('button', { name: 'Edit user override' })
  ).toBeDisabled()
  expect(
    screen.getByRole('switch', { name: 'Enable Shared upstream in this pool' })
  ).toHaveAttribute('aria-disabled', 'true')
})

test.each([
  [true, false],
  [false, true],
])(
  'a default pool requires both read and write permission before enabling edits (read=%s, write=%s)',
  async (read, write) => {
    mockApi()
    mount(
      <MonitorPoolEditor
        target={{ group: 'standard', model: 'kimi-a' }}
        onDirtyChange={vi.fn()}
      />,
      {
        id: 2,
        username: 'admin',
        role: 10,
        permissions: { admin_permissions: { channel: { read, write } } },
      }
    )
    const toggle = await screen.findByRole('switch', {
      name: 'Enable Shared upstream in this pool',
    })
    expect(toggle).toHaveAttribute('aria-disabled', 'true')
    expect(
      screen.getByRole('spinbutton', { name: 'Weight for Shared upstream' })
    ).toBeDisabled()
  }
)
