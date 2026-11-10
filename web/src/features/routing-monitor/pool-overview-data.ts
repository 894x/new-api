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
import type {
  getRoutingPool,
  RoutingMode,
  UserRoutingPoolConfig,
} from '@/features/model-routing/api'

import type { MonitorRoute } from './api'
import type { MonitorTarget } from './monitor-panel'

export type OverviewPool = {
  target: MonitorTarget
  username?: string
  names: Map<number, string>
  route?: MonitorRoute
  at: number
  loading: boolean
  unavailable: boolean
  partial: boolean
  inherited: boolean
  baseConfig?: Awaited<ReturnType<typeof getRoutingPool>>
  userConfig?: UserRoutingPoolConfig
  configError?: boolean
}

export type OverviewFlow = {
  key: string
  targets: [OverviewPool, ...OverviewPool[]]
  route: MonitorRoute
  names: Map<number, string>
  at: number
}

export function monitorTargetKey(target: MonitorTarget) {
  return JSON.stringify([target.userId ?? 0, target.group, target.model])
}

export function monitorFlowKey(route: MonitorRoute) {
  return JSON.stringify([route.policy, route.group, route.model, route.mode])
}

// Inherited user views contain the same base-pool counters. Keep one snapshot
// per effective route, while preserving every selected configuration for editing.
export function overviewFlows(pools: OverviewPool[]): OverviewFlow[] {
  const flows = new Map<string, OverviewFlow>()
  for (const pool of pools) {
    if (!pool.route || pool.unavailable) continue
    const key = monitorFlowKey(pool.route)
    const previous = flows.get(key)
    if (!previous) {
      flows.set(key, {
        key,
        targets: [pool],
        route: pool.route,
        names: new Map(pool.names),
        at: pool.at,
      })
      continue
    }
    previous.targets.push(pool)
    for (const [id, name] of pool.names) previous.names.set(id, name)
    if (pool.at > previous.at) {
      previous.route = pool.route
      previous.at = pool.at
    }
  }
  return [...flows.values()]
}

export function overviewMode(
  mode: RoutingMode | '' | null | undefined,
  legacyEnabled: boolean | undefined
): RoutingMode {
  return mode || (legacyEnabled ? 'smart' : 'priority')
}
