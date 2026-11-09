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
  fireEvent,
  render,
  screen,
  waitFor,
  within,
} from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, expect, test, vi } from 'vitest'

import { routingPoolKeys } from '@/features/model-routing/api'
import { api } from '@/lib/api'

import RoutingMonitorWorkspace from '..'
import { monitorKey, type MonitorRoute, type MonitorSnapshot } from '../api'

const clients: QueryClient[] = []
const headerHosts: HTMLElement[] = []
afterEach(() => {
  clients.forEach((client) => client.clear())
  clients.length = 0
  headerHosts.forEach((host) => host.remove())
  headerHosts.length = 0
  vi.useRealTimers()
})

function routeFixture(policy = '', count = 123): MonitorRoute {
  return {
    group: 'standard',
    model: 'kimi-k3',
    policy,
    mode: 'discount',
    dynamic: true,
    last_seen: Date.now(),
    window_seconds: 60,
    selection_count: count,
    truncated: false,
    channels: [
      {
        channel_id: 49,
        active: true,
        priority: 1,
        weight: 100,
        effective_cost_discount: 0.5,
        target_share: 0.985,
        selection_count: count - 1,
        selected_share: 0.98,
        probe: false,
        degraded: false,
        unverified: false,
        ttft_ms: 820,
        tpot_ms: 42,
        sample_count: 3,
      },
      {
        channel_id: 55,
        active: true,
        priority: 0,
        weight: 100,
        effective_cost_discount: 0.6,
        target_share: 0.015,
        selection_count: 1,
        selected_share: 0.02,
        probe: true,
        degraded: false,
        unverified: false,
        ttft_ms: 610,
        tpot_ms: 31,
        sample_count: 3,
      },
    ],
    history: [
      {
        at: Date.now() - 1000,
        selection_count: 1,
        channels: [{ channel_id: 49, selection_count: 1, selected_share: 1 }],
      },
    ],
    events: [
      {
        at: Date.now(),
        kind: 'initialized',
        channel_ids: [],
        allocations: [{ channel_id: 49, share: 0.985, probe: false }],
      },
    ],
  }
}

function snapshotFixture(): MonitorSnapshot {
  return {
    generated_at: Date.now(),
    legacy_enabled: false,
    scope: 'base',
    coverage: 'local',
    partial: false,
    nodes: [
      {
        id: 'node-1',
        name: 'gateway-01',
        local: true,
        updated_at: Date.now(),
        routes: [routeFixture(), routeFixture('user:7', 7)],
      },
    ],
  }
}

function mockApi(
  options: {
    snapshot?: MonitorSnapshot
    source?: 'user' | 'default_pool' | 'channel_defaults'
    fail?: boolean
    mode?: 'priority' | 'discount'
    noPool?: boolean
    effectiveMode?: 'discount'
    modelName?: string
    pending?: boolean
  } = {}
) {
  return vi.spyOn(api, 'get').mockImplementation(async (url) => {
    if (url.endsWith('/routing-monitor')) {
      if (options.pending) return new Promise(() => {})
      if (options.fail) throw new Error('unavailable')
      return {
        data: { success: true, data: options.snapshot ?? snapshotFixture() },
      }
    }
    if (url.includes('/user/')) {
      return {
        data: {
          success: true,
          data: {
            models: ['kimi-k3'],
            model_groups: { 'kimi-k3': ['standard'] },
            key_groups: { standard: 'Standard' },
            channels: [
              { channel_id: 49, channel_name: 'Channel A' },
              { channel_id: 55, channel_name: 'Channel B' },
            ],
            source: options.source ?? 'user',
            mode: options.mode ?? 'discount',
            configured: true,
            user_id: 7,
            username: 'customer',
          },
        },
      }
    }
    if (url.endsWith('/catalog')) {
      return {
        data: {
          success: true,
          data: {
            groups: { standard: 'Standard' },
            models: [options.modelName ?? 'kimi-k3'],
          },
        },
      }
    }
    return {
      data: {
        success: true,
        data: {
          pool: options.noPool ? null : { mode: options.mode ?? 'discount' },
          effective_mode: options.effectiveMode ?? '',
          candidates: [
            { channel_id: 49, channel_name: 'Channel A' },
            { channel_id: 55, channel_name: 'Channel B' },
          ],
        },
      },
    }
  })
}

function mount(
  userId?: number,
  prepare?: (client: QueryClient) => void,
  props?: {
    fixedGroup?: string
    fixedModel?: string
    embedded?: boolean
    panel?: boolean
    headerContainer?: HTMLElement | null
  }
) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false, gcTime: Infinity } },
  })
  clients.push(client)
  prepare?.(client)
  const view = render(
    <QueryClientProvider client={client}>
      <RoutingMonitorWorkspace
        key={userId ?? 'base'}
        userId={userId}
        {...props}
      />
    </QueryClientProvider>
  )
  return { ...view, client }
}

test('shows target shares separately from actual selection shares and channel metrics', async () => {
  mockApi()
  mount()
  await screen.findByRole('list', { name: 'Candidate channels' })
  await userEvent.click(screen.getByRole('combobox', { name: 'Monitor node' }))
  await userEvent.click(
    await screen.findByRole('option', { name: 'gateway-01 · Current node' })
  )
  fireEvent.click(screen.getByRole('tab', { name: 'Channel details' }))
  const table = screen.getByRole('table')
  const row = within(table).getByRole('row', { name: /Channel A/ })
  expect(within(row).getByText('98.5%')).toBeVisible()
  expect(within(row).getByText('98%')).toBeVisible()
  expect(within(row).getByText('820 ms')).toBeVisible()
  expect(
    screen.getByText(
      'Selections include retries and local candidate decisions; they are not upstream request counts.'
    )
  ).toBeVisible()
})

test('a card monitor keeps its fixed group and model instead of choosing another active pool', async () => {
  const get = mockApi()
  mount(undefined, undefined, {
    fixedGroup: 'vip',
    fixedModel: 'fixed-model',
    embedded: true,
  })
  await screen.findByText('No routing observations')
  expect(get).toHaveBeenCalledWith(
    '/api/channel/routing-pool',
    expect.objectContaining({
      params: { routing_group: 'vip', model: 'fixed-model' },
    })
  )
  expect(get.mock.calls.some(([url]) => url.endsWith('/catalog'))).toBe(false)
  expect(
    screen.queryByRole('list', { name: 'Candidate channels' })
  ).not.toBeInTheDocument()
})

test('an embedded card monitor places the overview beside the live pane without duplicate pool selectors or fullscreen controls', async () => {
  mockApi()
  mount(undefined, undefined, {
    fixedGroup: 'standard',
    fixedModel: 'kimi-k3',
    embedded: true,
  })
  await screen.findByRole('list', { name: 'Candidate channels' })
  const overview = screen.getByRole('region', { name: 'Routing overview' })
  const live = screen.getByRole('region', { name: 'Live routing flow' })
  expect(overview).toHaveAttribute('data-routing-monitor-overview')
  expect(live).toHaveAttribute('data-routing-monitor-live')
  expect(within(overview).getByText('Primary destination')).toBeVisible()
  expect(
    within(live).getByRole('combobox', { name: 'Monitor node' })
  ).toBeVisible()
  expect(
    screen.queryByRole('combobox', { name: 'Monitor model' })
  ).not.toBeInTheDocument()
  expect(
    screen.queryByRole('combobox', { name: 'Monitor group' })
  ).not.toBeInTheDocument()
  expect(
    screen.queryByRole('button', { name: 'Fullscreen' })
  ).not.toBeInTheDocument()
})

test('the embedded overview remains in place while observations load', async () => {
  mockApi({ pending: true })
  mount(undefined, undefined, {
    fixedGroup: 'standard',
    fixedModel: 'kimi-k3',
    embedded: true,
  })
  expect(screen.getByRole('region', { name: 'Routing overview' })).toBeVisible()
  expect(
    await screen.findByText('Loading routing observations...')
  ).toBeVisible()
  expect(
    screen.getByRole('region', { name: 'Live routing flow' })
  ).toBeVisible()
})

test.each(['user', 'default_pool'] as const)(
  'a monitor panel keeps compact header controls and exposes %s pool details in its floating info panel',
  async (source) => {
    const host = document.createElement('div')
    document.body.append(host)
    headerHosts.push(host)
    mockApi({ source })
    mount(7, undefined, {
      fixedGroup: 'standard',
      fixedModel: 'kimi-k3',
      panel: true,
      headerContainer: host,
    })
    const content = screen.getByRole('region', { name: 'Live routing flow' })
    await within(content).findByRole('list', { name: 'Candidate channels' })
    expect(
      within(host).getByRole('combobox', { name: 'Monitor node' })
    ).toBeVisible()
    expect(within(host).getByRole('button', { name: 'Refresh' })).toBeVisible()
    expect(
      within(host).getByRole('button', { name: 'Refresh' })
    ).not.toHaveTextContent('Refresh')
    expect(within(host).getByLabelText('User #7')).toBeVisible()
    expect(within(host).queryByText(/Snapshot time/)).not.toBeInTheDocument()
    await userEvent.click(
      within(host).getByRole('button', { name: 'Routing observations' })
    )
    const details = screen.getByRole('dialog', { name: 'Routing observations' })
    expect(within(details).getByText(/Snapshot time/)).toBeVisible()
    expect(
      within(details).getByText(
        'Each dot represents one upstream attempt on its channel, including retries.'
      )
    ).toBeVisible()
    expect(
      within(content).queryByText(
        'Each dot represents one upstream attempt on its channel, including retries.'
      )
    ).not.toBeInTheDocument()
    expect(
      within(details).getByText(
        source === 'user' ? 'User override' : 'Inherited base pool'
      )
    ).toBeVisible()
    if (source === 'default_pool') {
      expect(
        within(details).getByText(
          'This user inherits the base pool. The displayed traffic is shared base-pool traffic, not traffic attributed only to this user.'
        )
      ).toBeVisible()
    }
    await userEvent.keyboard('{Escape}')
    expect(
      within(content).queryByRole('combobox', { name: 'Monitor node' })
    ).not.toBeInTheDocument()
    const tabs = within(content).getByRole('tablist', {
      name: 'Routing observation view',
    })
    expect(tabs.closest('[data-slot=card-header]')).toHaveClass(
      'absolute',
      'pointer-events-none'
    )
    expect(tabs).toHaveClass('pointer-events-auto')
    expect(within(tabs).getAllByRole('tab')).toHaveLength(4)
    const overview = within(content).getByRole('region', {
      name: 'Routing overview',
    })
    expect(overview).toHaveClass('shrink-0')
    expect(within(overview).getByText('Primary destination')).toBeVisible()
    await userEvent.click(
      within(tabs).getByRole('tab', { name: 'Traffic share history' })
    )
    expect(
      within(content).getByRole('region', { name: 'Traffic share history' })
    ).toBeVisible()
    expect(overview).toBeVisible()
    await userEvent.click(
      within(tabs).getByRole('tab', { name: 'Routing changes' })
    )
    expect(
      within(content).getByRole('list', { name: 'Routing changes' })
    ).toBeVisible()
    expect(
      within(content).queryByRole('list', { name: 'Candidate channels' })
    ).not.toBeInTheDocument()
    expect(
      within(tabs).getByRole('tab', { name: 'Dynamic flow' })
    ).not.toHaveTextContent('Dynamic flow')
    await userEvent.click(
      within(tabs).getByRole('tab', { name: 'Channel details' })
    )
    expect(within(content).getByRole('table')).toBeVisible()
    await userEvent.keyboard('{ArrowLeft}{Enter}')
    await waitFor(() =>
      expect(
        within(tabs).getByRole('tab', { name: 'Dynamic flow' })
      ).toHaveAttribute('aria-selected', 'true')
    )
  }
)

test('embedded controls and monitoring notices render in the global header instead of duplicating the body title', async () => {
  const host = document.createElement('div')
  document.body.append(host)
  headerHosts.push(host)
  mockApi({ source: 'default_pool' })
  mount(7, undefined, {
    fixedGroup: 'standard',
    fixedModel: 'kimi-k3',
    embedded: true,
    headerContainer: host,
  })
  await screen.findByRole('list', { name: 'Candidate channels' })
  const live = screen.getByRole('region', { name: 'Live routing flow' })
  expect(
    within(host).getByRole('combobox', { name: 'Monitor node' })
  ).toBeVisible()
  expect(
    within(host).getByRole('button', { name: 'Pause live updates' })
  ).toBeVisible()
  expect(within(host).getByText(/Snapshot time/)).toBeVisible()
  expect(
    within(host).getByText(
      'Current node only. Cluster snapshots are unavailable.'
    )
  ).toBeVisible()
  expect(
    within(host).getByText(
      'This user inherits the base pool. The displayed traffic is shared base-pool traffic, not traffic attributed only to this user.'
    )
  ).toBeVisible()
  expect(
    within(live).queryByRole('combobox', { name: 'Monitor node' })
  ).not.toBeInTheDocument()
  expect(
    within(live).queryByRole('heading', { name: 'Routing observatory' })
  ).not.toBeInTheDocument()
  expect(
    within(live).queryByText('Observe allocation changes and their causes')
  ).not.toBeInTheDocument()
  await userEvent.click(
    within(host).getByRole('button', { name: 'Pause live updates' })
  )
  expect(
    within(host).getByRole('button', { name: 'Resume live updates' })
  ).toHaveAttribute('aria-pressed', 'true')
})

test('compact history and event views show empty states while keeping the summary visible', async () => {
  const snapshot = snapshotFixture()
  for (const route of snapshot.nodes[0].routes) {
    route.history = []
    route.events = []
  }
  mockApi({ snapshot })
  mount(undefined, undefined, {
    fixedGroup: 'standard',
    fixedModel: 'kimi-k3',
    panel: true,
  })
  await screen.findByRole('list', { name: 'Candidate channels' })
  const tabs = screen.getByRole('tablist', { name: 'Routing observation view' })
  await userEvent.click(
    within(tabs).getByRole('tab', { name: 'Traffic share history' })
  )
  expect(
    within(
      screen.getByRole('region', { name: 'Traffic share history' })
    ).getByText('No Data')
  ).toBeVisible()
  await userEvent.click(
    within(tabs).getByRole('tab', { name: 'Routing changes' })
  )
  expect(
    screen.getByText('No allocation changes in this observation window')
  ).toBeVisible()
  expect(screen.getByRole('region', { name: 'Routing overview' })).toBeVisible()
})

test('the embedded history and event cards share a bounded footer while all recent events remain scrollable', async () => {
  const snapshot = snapshotFixture()
  const route = snapshot.nodes[0].routes[0]
  route.events = Array.from({ length: 20 }, (_, index) => ({
    ...route.events[0],
    at: route.last_seen - index * 1000,
  }))
  mockApi({ snapshot })
  mount(undefined, undefined, {
    fixedGroup: 'standard',
    fixedModel: 'kimi-k3',
    embedded: true,
  })
  const events = await screen.findByRole('region', { name: 'Routing changes' })
  const history = screen.getByRole('region', { name: 'Traffic share history' })
  expect(history.closest('[data-routing-monitor-history-row]')).toHaveClass(
    'xl:items-stretch',
    'xl:shrink-0'
  )
  expect(history).toHaveClass('xl:h-full', 'xl:min-h-0')
  expect(events).toHaveClass('xl:h-full', 'xl:min-h-0')
  expect(events.querySelector('[data-routing-event-scroll]')).toHaveClass(
    'xl:overflow-y-auto',
    'xl:min-h-0'
  )
  expect(within(events).getAllByRole('listitem')).toHaveLength(20)
  expect(
    screen
      .getByRole('region', { name: 'Live routing flow' })
      .querySelector('[data-routing-monitor-flow]')
  ).toHaveClass('xl:flex-1', 'xl:min-h-0')
})

test('user override selects only that user policy while inherited pools show shared base traffic', async () => {
  const get = mockApi({ source: 'user' })
  const view = mount(7)
  await screen.findByRole('list', { name: 'Candidate channels' })
  expect(screen.getAllByText('7').length).toBeGreaterThan(0)
  expect(screen.queryByText('123')).not.toBeInTheDocument()
  expect(get).toHaveBeenCalledWith(
    '/api/user/7/routing-monitor',
    expect.anything()
  )
  view.unmount()
  get.mockRestore()
  mockApi({ source: 'default_pool' })
  mount(7)
  await screen.findByText(
    'This user inherits the base pool. The displayed traffic is shared base-pool traffic, not traffic attributed only to this user.'
  )
  await screen.findByRole('list', { name: 'Candidate channels' })
  expect(screen.getAllByText('123').length).toBeGreaterThan(0)
})

test('pausing freezes the snapshot and resuming loads the next actual count', async () => {
  const snapshot = snapshotFixture()
  const get = mockApi({ snapshot })
  mount()
  await screen.findByRole('list', { name: 'Candidate channels' })
  vi.useFakeTimers()
  fireEvent.click(screen.getByRole('button', { name: 'Pause live updates' }))
  expect(
    screen.getByRole('button', { name: 'Resume live updates' })
  ).toHaveAttribute('aria-pressed', 'true')
  const before = get.mock.calls.filter(([url]) =>
    url.endsWith('/routing-monitor')
  ).length
  snapshot.nodes[0].routes[0].selection_count = 321
  await act(async () => {
    await vi.advanceTimersByTimeAsync(9000)
  })
  expect(
    get.mock.calls.filter(([url]) => url.endsWith('/routing-monitor'))
  ).toHaveLength(before)
  expect(screen.getAllByText('123').length).toBeGreaterThan(0)
  fireEvent.click(screen.getByRole('button', { name: 'Resume live updates' }))
  await act(async () => {
    await vi.advanceTimersByTimeAsync(1)
  })
  vi.useRealTimers()
  await screen.findAllByText('321')
})

test('priority pools hide old adaptive observations', async () => {
  mockApi({ mode: 'priority' })
  mount()
  await screen.findByText('No routing observations')
  expect(
    screen.queryByRole('list', { name: 'Candidate channels' })
  ).not.toBeInTheDocument()
})

test('priority pools display observed traffic without adaptive target shares', async () => {
  const snapshot = snapshotFixture()
  const route = snapshot.nodes[0].routes[0]
  route.mode = 'priority'
  route.dynamic = false
  route.channels[1].probe = false
  mockApi({ mode: 'priority', snapshot })
  mount()
  await screen.findByRole('list', { name: 'Candidate channels' })
  expect(screen.getByText('Priority routing')).toBeVisible()
  expect(screen.queryByText('Warming up')).not.toBeInTheDocument()
  fireEvent.click(screen.getByRole('tab', { name: 'Channel details' }))
  expect(screen.getByRole('columnheader', { name: 'Priority' })).toBeVisible()
  expect(screen.getByRole('columnheader', { name: 'Weight' })).toBeVisible()
  expect(
    screen.queryByRole('columnheader', { name: 'Target share' })
  ).not.toBeInTheDocument()
  expect(screen.getByRole('row', { name: /Channel A/ })).toHaveTextContent(
    '99.19%'
  )
})

test.each([
  ['Retired candidate', { active: false }, 'warning', 'priority'],
  ['Manually disabled', { status: 2 }, 'warning', 'priority'],
  ['Automatically disabled', { status: 3 }, 'destructive', 'priority'],
  [
    'Ejected',
    { ejected_until: Number.MAX_SAFE_INTEGER },
    'destructive',
    'discount',
  ],
  ['Degraded', { degraded: true }, 'warning', 'discount'],
] as const)(
  'channels show %s with matching status and card emphasis',
  async (label, state, tone, mode) => {
    const snapshot = snapshotFixture()
    const route = snapshot.nodes[0].routes[0]
    route.mode = mode
    Object.assign(route.channels[0], state)
    mockApi({ mode, snapshot })
    mount()
    const badge = await screen.findByText(label)
    expect(badge).toHaveClass(`text-${tone}`)
    expect(badge.closest('[data-flow-channel]')).toHaveClass(`bg-${tone}/5`)
  }
)

test('monitoring failure stops the flow and provides a retry action', async () => {
  mockApi({ fail: true })
  mount()
  await screen.findByText('Unable to refresh routing observations')
  expect(screen.getByRole('button', { name: 'Retry' })).toBeVisible()
  expect(
    screen.queryByRole('list', { name: 'Candidate channels' })
  ).not.toBeInTheDocument()
})

test('a previous smart allocation is hidden after the pool changes to discount mode', async () => {
  const snapshot = snapshotFixture()
  snapshot.nodes[0].routes[0].mode = 'smart'
  mockApi({ snapshot })
  mount()
  await screen.findByText('No routing observations')
  expect(
    screen.queryByRole('list', { name: 'Candidate channels' })
  ).not.toBeInTheDocument()
})

test('user models remain available when the shared base catalog has incompatible cached data', async () => {
  const snapshot = snapshotFixture()
  snapshot.nodes[0].routes = []
  mockApi({ snapshot })
  mount(7, (client) => {
    client.setQueryData(routingPoolKeys.catalog('standard'), {
      groups: { standard: 'Standard' },
      models: ['base-only'],
    })
  })
  await waitFor(() =>
    expect(screen.getByRole('combobox', { name: 'Monitor model' })).toHaveValue(
      'kimi-k3'
    )
  )
  expect(await screen.findByText('No routing observations')).toBeVisible()
})

test('inherited channel defaults follow the live legacy switch without reloading pool configuration', async () => {
  const snapshot = snapshotFixture()
  snapshot.legacy_enabled = true
  snapshot.nodes[0].routes[0].mode = 'smart'
  const options = {
    snapshot,
    source: 'channel_defaults' as const,
    mode: 'priority' as const,
  }
  const get = mockApi(options)
  const { client } = mount(7)
  await screen.findByRole('list', { name: 'Candidate channels' })
  expect(screen.getAllByText('123').length).toBeGreaterThan(0)
  expect(
    screen.getByText(
      'This user inherits the base pool. The displayed traffic is shared base-pool traffic, not traffic attributed only to this user.'
    )
  ).toBeVisible()
  const configReads = get.mock.calls.filter(([url]) =>
    url.includes('/channel-routing-overrides')
  ).length
  options.snapshot = { ...snapshot, legacy_enabled: false }
  await act(async () => {
    await client.refetchQueries({ queryKey: monitorKey(7), exact: true })
  })
  expect(await screen.findByText('No routing observations')).toBeVisible()
  expect(
    get.mock.calls.filter(([url]) => url.includes('/channel-routing-overrides'))
  ).toHaveLength(configReads)
})

test('removing an explicit discount pool selects legacy smart observations rather than old discount allocations', async () => {
  const snapshot = snapshotFixture()
  snapshot.legacy_enabled = true
  const smart = routeFixture('', 77)
  smart.mode = 'smart'
  snapshot.nodes[0].routes.push(smart)
  mockApi({ snapshot, noPool: true })
  mount()
  await screen.findByRole('list', { name: 'Candidate channels' })
  expect(screen.getAllByText('77').length).toBeGreaterThan(0)
  expect(screen.queryByText('123')).not.toBeInTheDocument()
})

test('an unavailable explicitly selected node never silently displays local-node traffic', async () => {
  const snapshot = snapshotFixture()
  snapshot.nodes.push({
    id: 'node-2',
    name: 'gateway-02',
    local: false,
    updated_at: Date.now(),
    routes: [routeFixture('', 456)],
  })
  const options = { snapshot }
  mockApi(options)
  const { client } = mount()
  await screen.findByRole('list', { name: 'Candidate channels' })
  await userEvent.click(screen.getByRole('combobox', { name: 'Monitor node' }))
  await userEvent.click(
    await screen.findByRole('option', { name: 'gateway-02' })
  )
  await screen.findAllByText('456')
  options.snapshot = { ...snapshot, nodes: [snapshot.nodes[0]] }
  await act(async () => {
    await client.refetchQueries({ queryKey: monitorKey(), exact: true })
  })
  expect(
    await screen.findByText(
      'The selected node is unavailable. Select another node to continue.'
    )
  ).toBeVisible()
  expect(screen.queryAllByText('123')).toHaveLength(0)
  expect(screen.queryAllByText('456')).toHaveLength(0)
  expect(
    screen.getByRole('combobox', { name: 'Monitor node' })
  ).toHaveTextContent('gateway-02 · Unavailable')
  await userEvent.click(screen.getByRole('combobox', { name: 'Monitor node' }))
  expect(
    screen.getByRole('option', { name: 'gateway-02 · Unavailable' })
  ).toBeVisible()
  await userEvent.click(
    screen.getByRole('option', { name: 'gateway-01 · Current node' })
  )
  await screen.findAllByText('123')
})

test('defaults to all-node totals and preserves them when the responding gateway changes', async () => {
  const snapshot = snapshotFixture()
  snapshot.nodes.push({
    id: 'node-2',
    name: 'gateway-02',
    local: false,
    updated_at: Date.now(),
    routes: [routeFixture('', 77)],
  })
  const options = { snapshot }
  mockApi(options)
  const { client } = mount(undefined, undefined, {
    fixedGroup: 'standard',
    fixedModel: 'kimi-k3',
  })
  await screen.findByRole('list', { name: 'Candidate channels' })
  expect(
    screen.getByRole('combobox', { name: 'Monitor node' })
  ).toHaveTextContent('All nodes (combined)')
  expect(
    within(screen.getByRole('region', { name: 'Routing overview' })).getByText(
      '200'
    )
  ).toBeVisible()
  options.snapshot = {
    ...snapshot,
    nodes: snapshot.nodes.map((node) => ({ ...node, local: !node.local })),
  }
  await act(async () => {
    await client.refetchQueries({ queryKey: monitorKey(), exact: true })
  })
  expect(screen.getByRole('list', { name: 'Candidate channels' })).toBeVisible()
  expect(screen.queryByText('No routing observations')).not.toBeInTheDocument()
  expect(
    screen.getByRole('combobox', { name: 'Monitor node' })
  ).toHaveTextContent('All nodes (combined)')
  expect(
    within(screen.getByRole('region', { name: 'Routing overview' })).getByText(
      '200'
    )
  ).toBeVisible()
  await userEvent.click(screen.getByRole('combobox', { name: 'Monitor node' }))
  await userEvent.click(
    await screen.findByRole('option', { name: 'gateway-02 · Current node' })
  )
  expect(
    within(screen.getByRole('region', { name: 'Routing overview' })).getByText(
      '77'
    )
  ).toBeVisible()
  await userEvent.click(screen.getByRole('combobox', { name: 'Monitor node' }))
  await userEvent.click(
    await screen.findByRole('option', { name: 'All nodes (combined)' })
  )
  expect(
    within(screen.getByRole('region', { name: 'Routing overview' })).getByText(
      '200'
    )
  ).toBeVisible()
})

test('a request modifier model uses the inherited effective mode without creating an exact pool', async () => {
  const modelName = 'kimi-k3@effort:high'
  const snapshot = snapshotFixture()
  snapshot.nodes[0].routes[0].model = modelName
  mockApi({ snapshot, modelName, noPool: true, effectiveMode: 'discount' })
  mount()
  await screen.findByRole('list', { name: 'Candidate channels' })
  expect(screen.getByRole('combobox', { name: 'Monitor model' })).toHaveValue(
    modelName
  )
  expect(screen.getAllByText('123').length).toBeGreaterThan(0)
})

test('routing changes preserve newest-first ordering from the monitoring snapshot', async () => {
  const snapshot = snapshotFixture()
  const route = snapshot.nodes[0].routes[0]
  route.events.unshift({
    at: Date.now() + 1000,
    kind: 'failure',
    channel_ids: [49],
    allocations: [],
  })
  mockApi({ snapshot })
  mount()
  const events = await screen.findByRole('list', { name: 'Routing changes' })
  const entries = within(events).getAllByRole('listitem')
  expect(
    within(entries[0]).getByText('Channel failure', { exact: true })
  ).toBeVisible()
  expect(entries[1]).toHaveTextContent('Routing allocation initialized')
})

test('retired candidates remain in history details without receiving a live flow', async () => {
  const snapshot = snapshotFixture()
  snapshot.nodes[0].routes[0].channels[1].active = false
  snapshot.nodes[0].routes[0].channels[1].target_share = 0
  mockApi({ snapshot })
  mount()
  const candidates = await screen.findByRole('list', {
    name: 'Candidate channels',
  })
  expect(within(candidates).getAllByRole('listitem')).toHaveLength(1)
  fireEvent.click(screen.getByRole('tab', { name: 'Channel details' }))
  expect(screen.getByText('Retired candidate')).toBeVisible()
})

test('changing nodes shows their own allocation and fullscreen preserves the selected view', async () => {
  const snapshot = snapshotFixture()
  snapshot.nodes.push({
    id: 'node-2',
    name: 'gateway-02',
    local: false,
    updated_at: Date.now(),
    routes: [routeFixture('', 456)],
  })
  mockApi({ snapshot })
  mount()
  await screen.findByRole('list', { name: 'Candidate channels' })
  await userEvent.click(screen.getByRole('combobox', { name: 'Monitor node' }))
  await userEvent.click(
    await screen.findByRole('option', { name: 'gateway-02' })
  )
  await screen.findAllByText('456')
  fireEvent.click(screen.getByRole('tab', { name: 'Channel details' }))
  fireEvent.click(screen.getByRole('button', { name: 'Fullscreen' }))
  expect(screen.getByRole('dialog', { name: 'Model routing' })).toBeVisible()
  expect(screen.getByRole('tab', { name: 'Channel details' })).toHaveAttribute(
    'aria-selected',
    'true'
  )
  fireEvent.keyDown(window, { key: 'Escape' })
  await waitFor(() =>
    expect(screen.queryByRole('dialog')).not.toBeInTheDocument()
  )
})
