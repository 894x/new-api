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
  RoutingMode,
  UserRoutingPoolConfig,
} from '@/features/model-routing/api'

import type { MonitorChannel, MonitorNode, MonitorRoute } from './api'

export function effectiveMonitorRoute(
  node: MonitorNode | undefined,
  group: string,
  model: string,
  userId?: number,
  source?: UserRoutingPoolConfig['source'],
  mode?: RoutingMode
): MonitorRoute | undefined {
  if (userId && !source) return undefined
  const policy = userId && source === 'user' ? `user:${userId}` : ''
  return node?.routes.find(
    (route) =>
      route.group === group &&
      route.model === model &&
      route.policy === policy &&
      route.mode === (mode ?? 'smart')
  )
}

export function monitorChannelStatus(
  channel: MonitorChannel,
  now: number
): 'ejected' | 'degraded' | 'unverified' | 'probe' | 'healthy' {
  if ((channel.ejected_until ?? 0) > now) return 'ejected'
  if (channel.degraded) return 'degraded'
  if (channel.unverified) return 'unverified'
  if (channel.probe) return 'probe'
  return 'healthy'
}

export function weightedDiscount(channels: MonitorChannel[]): number | null {
  let total = 0
  let share = 0
  for (const channel of channels) {
    if (channel.target_share <= 0) continue
    if (channel.effective_cost_discount == null) return null
    total += channel.effective_cost_discount * channel.target_share
    share += channel.target_share
  }
  return share > 0 ? total / share : null
}

export function channelColor(channelId: number): string {
  const hash = Math.imul(channelId, 0x9e3779b1) >>> 0
  return `var(--chart-${(hash % 5) + 1})`
}
