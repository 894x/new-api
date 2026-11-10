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
import { useQueries } from '@tanstack/react-query'

import {
  getRoutingPool,
  getUserRoutingPool,
  routingPoolKeys,
  userRoutingPoolKeys,
} from '@/features/model-routing/api'

import { aggregateMonitorRoutes } from './aggregate-route'
import { getRoutingMonitor, monitorKey } from './api'
import type { MonitorTarget } from './monitor-panel'
import {
  monitorTargetKey,
  overviewMode,
  type OverviewPool,
} from './pool-overview-data'
import { effectiveMonitorRoute } from './route-data'

export function usePoolOverview(
  targets: MonitorTarget[],
  paused: boolean,
  nodeId: string
) {
  const scopes = [...new Set(targets.map((target) => target.userId))]
  const snapshots = useQueries({
    queries: scopes.map((userId) => ({
      queryKey: monitorKey(userId),
      queryFn: ({ signal }: { signal: AbortSignal }) =>
        getRoutingMonitor(userId, signal),
      enabled: !paused,
      refetchInterval: paused ? (false as const) : 3000,
      refetchOnWindowFocus: !paused,
      retry: false,
      meta: { errorToast: false },
    })),
  })
  const defaults = targets.filter((target) => !target.userId)
  const users = targets.filter(
    (target): target is MonitorTarget & { userId: number } => !!target.userId
  )
  const defaultConfigs = useQueries({
    queries: defaults.map((target) => ({
      queryKey: routingPoolKeys.detail(target.group, target.model),
      queryFn: () => getRoutingPool(target.group, target.model),
      enabled: !paused,
      staleTime: 10000,
      refetchInterval: paused ? (false as const) : 15000,
      retry: false,
      meta: { errorToast: false },
    })),
  })
  const userConfigs = useQueries({
    queries: users.map((target) => ({
      queryKey: userRoutingPoolKeys.detail(
        target.userId,
        target.group,
        target.model
      ),
      queryFn: () =>
        getUserRoutingPool(target.userId, target.group, target.model),
      enabled: !paused,
      staleTime: 10000,
      refetchInterval: paused ? (false as const) : 15000,
      retry: false,
      meta: { errorToast: false },
    })),
  })
  const snapshotsByScope = new Map(
    scopes.map((scope, index) => [scope, snapshots[index]])
  )
  const defaultsByKey = new Map(
    defaults.map((target, index) => [
      monitorTargetKey(target),
      defaultConfigs[index],
    ])
  )
  const usersByKey = new Map(
    users.map((target, index) => [monitorTargetKey(target), userConfigs[index]])
  )
  const nodes = new Map<string, string>()
  const pools: OverviewPool[] = targets.map((target) => {
    const key = monitorTargetKey(target)
    const snapshotQuery = snapshotsByScope.get(target.userId)
    const snapshot = snapshotQuery?.data
    const defaultQuery = defaultsByKey.get(key)
    const userQuery = usersByKey.get(key)
    const base = defaultQuery?.data
    const user = userQuery?.data
    const configQuery = userQuery ?? defaultQuery
    const source = user?.strategy_user_override ? 'user' : user?.source
    let configuredMode = base?.effective_mode || base?.pool?.mode
    if (
      user &&
      (user.source !== 'channel_defaults' || user.strategy_user_override)
    ) {
      configuredMode = user.mode
    }
    const mode = overviewMode(configuredMode, snapshot?.legacy_enabled)
    const at = snapshot?.generated_at ?? 0
    for (const node of snapshot?.nodes ?? []) nodes.set(node.id, node.name)
    const selectedNodes = (snapshot?.nodes ?? []).filter(
      (node) => nodeId === '__all' || node.id === nodeId
    )
    const unavailable =
      !snapshotQuery ||
      !configQuery ||
      snapshotQuery.isError ||
      configQuery.isError ||
      (!paused && !!snapshot && Date.now() - at > 20000) ||
      (!!snapshot &&
        nodeId !== '__all' &&
        (selectedNodes.length === 0 ||
          selectedNodes.some((node) => at - node.updated_at > 15000)))
    let route =
      unavailable || (!base && !user)
        ? undefined
        : aggregateMonitorRoutes(selectedNodes, at, (node) =>
            effectiveMonitorRoute(
              node,
              target.group,
              target.model,
              target.userId,
              source,
              mode
            )
          )
    if (route && nodeId !== '__all') {
      // A single node retains its own target allocation, unlike cluster counts.
      route = effectiveMonitorRoute(
        selectedNodes[0],
        target.group,
        target.model,
        target.userId,
        source,
        mode
      )
    }
    return {
      target,
      username: user?.username,
      route,
      names: new Map(
        (user?.channels ?? base?.candidates ?? []).map((channel) => [
          channel.channel_id,
          channel.channel_name,
        ])
      ),
      at,
      loading: !!snapshotQuery?.isPending || !!configQuery?.isPending,
      unavailable,
      partial:
        !!snapshot?.partial ||
        snapshot?.coverage === 'local' ||
        selectedNodes.some((node) => at - node.updated_at > 15000),
      inherited: !!target.userId && !!source && source !== 'user',
      baseConfig: base,
      userConfig: user,
      configError: !!configQuery?.isError,
    }
  })
  return {
    pools,
    nodes,
    refresh: () => {
      for (const query of [...snapshots, ...defaultConfigs, ...userConfigs]) {
        void query.refetch()
      }
    },
  }
}
