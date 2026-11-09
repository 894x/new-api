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
import { assert, expect, test } from 'vitest'

import { aggregateMonitorRoutes } from '../aggregate-route'
import type { MonitorNode, MonitorRoute } from '../api'
import { effectiveMonitorRoute } from '../route-data'

function route(channelId: number, count: number, policy = ''): MonitorRoute {
  return {
    group: 'standard',
    model: 'kimi-k3',
    policy,
    mode: 'smart',
    dynamic: true,
    last_seen: 10000,
    window_seconds: 60,
    selection_count: count,
    truncated: false,
    channels: [
      {
        channel_id: channelId,
        active: true,
        priority: 1,
        weight: 100,
        effective_cost_discount: 0.5,
        target_share: 1,
        selection_count: count,
        selected_share: 1,
        probe: false,
        degraded: false,
        unverified: false,
        ttft_ms: 800,
        tpot_ms: 20,
        sample_count: 3,
      },
    ],
    history: [
      {
        at: 9000,
        selection_count: count,
        channels: [
          { channel_id: channelId, selection_count: count, selected_share: 1 },
        ],
      },
    ],
    events: [
      {
        at: 9000,
        kind: 'initialized',
        channel_ids: [],
        allocations: [{ channel_id: channelId, share: 1, probe: false }],
      },
    ],
    requests: [{ id: '1', at: 9900, channel_id: channelId }],
  }
}

test('combines channel counts and history by counts, preserves request identity across nodes and does not invent a common target or quality average', () => {
  const nodes: MonitorNode[] = [
    {
      id: 'a',
      name: 'A',
      local: true,
      updated_at: 10000,
      routes: [route(49, 6)],
    },
    {
      id: 'b',
      name: 'B',
      local: false,
      updated_at: 10000,
      routes: [route(55, 4)],
    },
  ]
  const combined = aggregateMonitorRoutes(nodes, 10000, (node) =>
    effectiveMonitorRoute(
      node,
      'standard',
      'kimi-k3',
      undefined,
      undefined,
      'smart'
    )
  )
  assert(combined)
  expect(combined.selection_count).toBe(10)
  expect(
    combined.channels.map((channel) => [
      channel.channel_id,
      channel.selection_count,
      channel.selected_share,
    ])
  ).toEqual([
    [49, 6, 0.6],
    [55, 4, 0.4],
  ])
  expect(combined.history[0]).toEqual({
    at: 9000,
    selection_count: 10,
    channels: [
      { channel_id: 49, selection_count: 6, selected_share: 0.6 },
      { channel_id: 55, selection_count: 4, selected_share: 0.4 },
    ],
  })
  expect(combined.requests?.map((request) => request.id)).toEqual([
    'a:1',
    'b:1',
  ])
  expect(combined.events.map((event) => event.node_name)).toEqual(['A', 'B'])
  expect(
    combined.channels.every(
      (channel) => channel.ttft_ms === null && channel.tpot_ms === null
    )
  ).toBe(true)
  expect(nodes[0].routes[0].channels[0].target_share).toBe(1)
})

test('merges matching user policies only and omits stale nodes without changing individual observations', () => {
  const nodes: MonitorNode[] = [
    {
      id: 'a',
      name: 'A',
      local: true,
      updated_at: 20000,
      routes: [route(49, 100), route(49, 3, 'user:7'), route(49, 80, 'user:8')],
    },
    {
      id: 'b',
      name: 'B',
      local: false,
      updated_at: 20000,
      routes: [route(49, 2, 'user:7')],
    },
    {
      id: 'stale',
      name: 'Stale',
      local: false,
      updated_at: 4000,
      routes: [route(49, 1000, 'user:7')],
    },
  ]
  const combined = aggregateMonitorRoutes(nodes, 20000, (node) =>
    effectiveMonitorRoute(node, 'standard', 'kimi-k3', 7, 'user', 'smart')
  )
  assert(combined)
  expect(combined.selection_count).toBe(5)
  expect(combined.channels).toHaveLength(1)
  expect(combined.channels[0].selection_count).toBe(5)
  expect(combined.channels[0].selected_share).toBe(1)
  expect(combined.requests).toHaveLength(2)
  expect(
    effectiveMonitorRoute(nodes[0], 'standard', 'kimi-k3', 7, 'user', 'smart')
      ?.selection_count
  ).toBe(3)
  expect(
    aggregateMonitorRoutes(nodes, 20000, (node) =>
      effectiveMonitorRoute(node, 'missing', 'kimi-k3')
    )
  ).toBeUndefined()
})

test('weights TTFT and TPOT with their own valid sample counts rather than averaging node means', () => {
  const first = route(49, 6)
  Object.assign(first.channels[0], {
    ttft_ms: 800,
    ttft_sample_count: 1,
    tpot_ms: 20,
    tpot_sample_count: 2,
  })
  const second = route(49, 4)
  Object.assign(second.channels[0], {
    ttft_ms: 1400,
    ttft_sample_count: 2,
    tpot_ms: 60,
    tpot_sample_count: 1,
  })
  const nodes: MonitorNode[] = [
    { id: 'a', name: 'A', local: true, updated_at: 10000, routes: [first] },
    { id: 'b', name: 'B', local: false, updated_at: 10000, routes: [second] },
  ]
  const combined = aggregateMonitorRoutes(
    nodes,
    10000,
    (node) => node.routes[0]
  )
  assert(combined)
  expect(combined.channels[0].ttft_ms).toBe(1200)
  expect(combined.channels[0].tpot_ms).toBeCloseTo(100 / 3)
  expect(combined.channels[0].ttft_sample_count).toBe(3)
  expect(combined.channels[0].tpot_sample_count).toBe(3)
})

test('priority completion metrics sum outcomes and weight latency by completed attempts across nodes', () => {
  const first = route(49, 2)
  const second = route(49, 1)
  first.mode = 'priority'
  second.mode = 'priority'
  Object.assign(first.channels[0], {
    completed_count: 2,
    success_count: 1,
    failure_count: 1,
    latency_ms: 100,
  })
  Object.assign(second.channels[0], {
    completed_count: 1,
    success_count: 1,
    failure_count: 0,
    latency_ms: 400,
  })
  const nodes: MonitorNode[] = [
    { id: 'a', name: 'A', local: true, updated_at: 10000, routes: [first] },
    { id: 'b', name: 'B', local: false, updated_at: 10000, routes: [second] },
  ]
  const combined = aggregateMonitorRoutes(nodes, 10000, (node) =>
    effectiveMonitorRoute(
      node,
      'standard',
      'kimi-k3',
      undefined,
      undefined,
      'priority'
    )
  )
  assert(combined)
  expect(combined.channels[0].completed_count).toBe(3)
  expect(combined.channels[0].success_count).toBe(2)
  expect(combined.channels[0].failure_count).toBe(1)
  expect(combined.channels[0].latency_ms).toBe(200)
})
