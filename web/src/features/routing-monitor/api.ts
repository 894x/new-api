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
import { api } from '@/lib/api'
import { requireServerSuccess } from '@/lib/server-error-message'

export interface MonitorChannel {
  channel_id: number
  active: boolean
  priority: number
  weight: number
  effective_cost_discount: number | null
  target_share: number
  selection_count: number
  selected_share: number
  probe: boolean
  degraded: boolean
  unverified: boolean
  ejected_until?: number
  ttft_ms: number | null
  tpot_ms: number | null
  sample_count: number
  ttft_sample_count?: number
  tpot_sample_count?: number
}

export interface MonitorHistory {
  at: number
  selection_count: number
  channels: {
    channel_id: number
    selection_count: number
    selected_share: number
  }[]
}

export interface MonitorEvent {
  node_id?: string
  node_name?: string
  at: number
  kind:
    | 'initialized'
    | 'candidates_changed'
    | 'failure'
    | 'degraded'
    | 'recovery'
    | 'allocation_changed'
  channel_ids: number[]
  allocations: { channel_id: number; share: number; probe: boolean }[]
}

export interface MonitorRoute {
  aggregate?: boolean
  requests?: MonitorRequest[]
  requests_truncated?: boolean
  group: string
  model: string
  policy: string
  mode: 'smart' | 'discount'
  dynamic: boolean
  last_seen: number
  window_seconds: number
  selection_count: number
  channels: MonitorChannel[]
  history: MonitorHistory[]
  events: MonitorEvent[]
  truncated: boolean
}

export interface MonitorRequest {
  id: string
  at: number
  channel_id: number
}

export interface MonitorNode {
  id: string
  name: string
  updated_at: number
  local: boolean
  routes: MonitorRoute[]
}

export interface MonitorSnapshot {
  generated_at: number
  legacy_enabled: boolean
  coverage: 'cluster' | 'local'
  nodes: MonitorNode[]
  scope: 'base' | 'user'
  user_id?: number
  partial: boolean
}

export const monitorKey = (userId?: number) =>
  ['routing-monitor', userId ?? 'base'] as const

export async function getRoutingMonitor(
  userId?: number,
  signal?: AbortSignal
): Promise<MonitorSnapshot> {
  const url = userId
    ? `/api/user/${userId}/routing-monitor`
    : '/api/channel/routing-monitor'
  const response = await api.get<{
    success: boolean
    message?: string
    data: MonitorSnapshot
  }>(url, { signal, disableDuplicate: true })
  return requireServerSuccess(response.data).data
}
