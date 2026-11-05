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
import { useQuery } from '@tanstack/react-query'
import { useState } from 'react'

import {
  getRoutingCatalog,
  getRoutingPool,
  getUserRoutingPool,
  routingPoolKeys,
} from '@/features/model-routing/api'

import { getRoutingMonitor, monitorKey } from './api'
import { effectiveMonitorRoute } from './route-data'

export function useMonitorData(
  userId: number | undefined,
  paused: boolean,
  fixedIdentity?: { group: string; model: string }
) {
  const [selectedGroup, setGroup] = useState('')
  const [selectedModel, setModel] = useState('')
  const [selectedNode, setSelectedNode] = useState<{
    id: string
    name: string
  }>()
  const monitor = useQuery({
    queryKey: monitorKey(userId),
    queryFn: ({ signal }) => getRoutingMonitor(userId, signal),
    enabled: !paused,
    refetchInterval: paused ? false : 3000,
    refetchOnWindowFocus: !paused,
    meta: { errorToast: false },
  })
  const catalog = useQuery({
    queryKey: ['routing-monitor-catalog', userId ?? 'base'],
    queryFn: async () => {
      if (userId) {
        const data = await getUserRoutingPool(userId)
        return {
          groups: data.key_groups ?? {},
          models: data.models,
          modelGroups: data.model_groups,
        }
      }
      const data = await getRoutingCatalog()
      return { ...data, modelGroups: undefined }
    },
    staleTime: 30000,
    enabled: !paused && !fixedIdentity,
    refetchOnWindowFocus: !paused,
    meta: { errorToast: false },
  })
  const nodes = monitor.data?.nodes ?? []
  const node = selectedNode
    ? nodes.find((item) => item.id === selectedNode.id)
    : (nodes.find((item) => item.local) ?? nodes[0])
  const nodeUnavailable = !!selectedNode && !node
  const setNode = (id: string) => {
    const option = nodes.find((item) => item.id === id)
    setSelectedNode({ id, name: option?.name ?? id })
  }
  const preferredPolicy = userId ? `user:${userId}` : ''
  const activeRoutes =
    node?.routes.filter((route) => route.selection_count > 0) ?? []
  const preferredRoute =
    activeRoutes.find((route) => route.policy === preferredPolicy) ??
    activeRoutes.find((route) => route.policy === '')
  let groups = [
    ...new Set([
      ...Object.keys(catalog.data?.groups ?? {}),
      ...nodes.flatMap((item) => item.routes.map((route) => route.group)),
    ]),
  ].sort()
  let group = groups.includes(selectedGroup)
    ? selectedGroup
    : (preferredRoute?.group ?? groups[0] ?? '')
  if (fixedIdentity) {
    group = fixedIdentity.group
    groups = group ? [group] : []
  }
  const groupCatalog = useQuery({
    queryKey: routingPoolKeys.catalog(group),
    queryFn: () => getRoutingCatalog(group),
    enabled: !fixedIdentity && !userId && !!group && !paused,
    staleTime: 30000,
    refetchOnWindowFocus: !paused,
    meta: { errorToast: false },
  })
  let models = catalog.data?.models ?? []
  if (!userId) models = groupCatalog.data?.models ?? models
  if (userId && catalog.data?.modelGroups) {
    const modelGroups = catalog.data.modelGroups
    models = models.filter((model) => modelGroups[model]?.includes(group))
  }
  models = [
    ...new Set([
      ...models,
      ...nodes.flatMap((item) =>
        item.routes
          .filter((route) => route.group === group)
          .map((route) => route.model)
      ),
    ]),
  ].sort()
  let model = models.includes(selectedModel)
    ? selectedModel
    : (models.find(
        (candidate) =>
          candidate === preferredRoute?.model && preferredRoute.group === group
      ) ??
      models[0] ??
      '')
  if (fixedIdentity) {
    model = fixedIdentity.model
    models = model ? [model] : []
  }
  const detail = useQuery({
    queryKey: ['routing-monitor-pool', userId ?? 'base', group, model],
    queryFn: async () => {
      if (userId) {
        const data = await getUserRoutingPool(userId, group, model)
        return {
          source: data.strategy_user_override ? ('user' as const) : data.source,
          mode:
            data.source === 'channel_defaults' && !data.strategy_user_override
              ? undefined
              : data.mode,
          names: new Map(
            data.channels.map((channel) => [
              channel.channel_id,
              channel.channel_name,
            ])
          ),
        }
      }
      const data = await getRoutingPool(group, model)
      return {
        source: data.pool
          ? ('default_pool' as const)
          : ('channel_defaults' as const),
        mode: data.effective_mode || data.pool?.mode,
        names: new Map(
          data.candidates.map((channel) => [
            channel.channel_id,
            channel.channel_name,
          ])
        ),
      }
    },
    enabled: !!group && !!model && !paused,
    staleTime: 10000,
    refetchInterval: paused ? false : 15000,
    refetchOnWindowFocus: !paused,
    meta: { errorToast: false },
  })
  const effectiveMode =
    detail.data?.mode ?? (monitor.data?.legacy_enabled ? 'smart' : 'priority')
  const route = effectiveMonitorRoute(
    node,
    group,
    model,
    userId,
    detail.data?.source,
    effectiveMode
  )
  return {
    monitor,
    fixed: !!fixedIdentity,
    catalog,
    groupCatalog,
    detail,
    nodes,
    node,
    selectedNode,
    nodeUnavailable,
    groups,
    group,
    models,
    model,
    route,
    setGroup,
    setModel,
    setNode,
  }
}
