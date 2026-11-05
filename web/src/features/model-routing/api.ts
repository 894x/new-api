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
import type { UserChannelRoutingConfig } from '@/features/users/types'
import { api } from '@/lib/api'
import { requireServerSuccess } from '@/lib/server-error-message'

export type RoutingMode = 'priority' | 'smart' | 'discount'

export const ROUTING_MODE_LABELS = {
  priority: 'Priority routing',
  smart: 'Smart routing',
  discount: 'Discount-first routing',
} as const

export const DEFAULT_DISCOUNT_LIMITS = {
  max_ttft_ms: 15000,
  max_tpot_ms: 200,
} as const

export interface UserRoutingPoolConfig extends UserChannelRoutingConfig {
  user_id: number
  username: string
  user_role: number
  model_groups?: Record<string, string[]>
  source: 'user' | 'default_pool' | 'channel_defaults'
  mode: RoutingMode
  configured: boolean
  pool_channels?: PoolChannel[]
}

export const userRoutingPoolKeys = {
  all: (userId: number) => ['user-routing-pools', userId] as const,
  catalog: (userId: number) =>
    ['user-routing-pools', userId, 'catalog'] as const,
  detail: (userId: number, group: string, model: string) =>
    ['user-routing-pools', userId, group, model] as const,
}

export async function getUserRoutingPool(
  userId: number,
  group = '',
  model = ''
) {
  const response = await api.get<{
    success: boolean
    message?: string
    data: UserRoutingPoolConfig
  }>(`/api/user/${userId}/channel-routing-overrides`, {
    params: { model, ...(group ? { routing_group: group } : {}), view: 'pool' },
  })
  return requireServerSuccess(response.data).data
}

export interface PoolChannel {
  channel_id: number
  priority: number
  weight: number
  enabled: boolean
}

export interface PoolCandidate {
  channel_id: number
  channel_name: string
  priority: number
  weight: number
  available: boolean
  effective_cost_discount?: number | null
}

export interface RoutingPool {
  routing_group: string
  model: string
  mode: RoutingMode
  revision: string
  channels: PoolChannel[]
  max_ttft_ms?: number
  max_tpot_ms?: number
}

export interface RoutingPoolView extends RoutingPool {
  candidates: PoolCandidate[]
}

export interface PoolFilters {
  model: string
  routing_group: string
  mode: RoutingMode | ''
  page: number
}

export const routingPoolKeys = {
  all: ['model-routing-pools'] as const,
  list: (filters: PoolFilters) => [...routingPoolKeys.all, filters] as const,
  detail: (group: string, model: string) =>
    [...routingPoolKeys.all, 'detail', group, model] as const,
  catalog: (group: string) => ['model-routing-catalog', group] as const,
}

export async function listRoutingPools(filters: PoolFilters) {
  const response = await api.get<{
    success: boolean
    message?: string
    data: {
      items: RoutingPoolView[]
      total: number
      page: number
      page_size: number
    }
  }>('/api/channel/routing-pools', { params: filters })
  return requireServerSuccess(response.data).data
}

export async function getRoutingCatalog(group = '') {
  const response = await api.get<{
    success: boolean
    message?: string
    data: { groups: Record<string, string>; models: string[] }
  }>('/api/channel/routing-pools/catalog', { params: { routing_group: group } })
  return requireServerSuccess(response.data).data
}

export async function getRoutingPool(group: string, model: string) {
  const response = await api.get<{
    success: boolean
    message?: string
    data: { pool: RoutingPool | null; candidates: PoolCandidate[] }
  }>('/api/channel/routing-pool', { params: { routing_group: group, model } })
  return requireServerSuccess(response.data).data
}

export async function saveRoutingPool(pool: RoutingPool) {
  const response = await api.put<{
    success: boolean
    message?: string
    data: RoutingPool
  }>('/api/channel/routing-pool', pool)
  return requireServerSuccess(response.data).data
}

export async function deleteRoutingPool(pool: RoutingPool) {
  const response = await api.delete<{ success: boolean; message?: string }>(
    '/api/channel/routing-pool',
    {
      data: {
        routing_group: pool.routing_group,
        model: pool.model,
        revision: pool.revision,
      },
    }
  )
  requireServerSuccess(response.data)
}
