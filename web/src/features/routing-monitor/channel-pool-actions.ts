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
import {
  getRoutingPool,
  getUserRoutingPool,
  listRoutingPools,
  saveRoutingPool,
  type RoutingPoolView,
} from '@/features/model-routing/api'
import { patchUserChannelRouting } from '@/features/users/api'
import {
  ADMIN_PERMISSION_ACTIONS,
  ADMIN_PERMISSION_RESOURCES,
  canAccessUserGroupConfig,
  hasPermission,
} from '@/lib/admin-permissions'
import { ROLE } from '@/lib/roles'
import { useAuthStore, type AuthUser } from '@/stores/auth-store'

import type { MonitorTarget } from './monitor-panel'
import { monitorTargetKey } from './pool-overview-data'

export function canEditDefaultPool(actor: AuthUser | null) {
  return (
    (actor?.role ?? 0) >= ROLE.ADMIN &&
    hasPermission(
      actor,
      ADMIN_PERMISSION_RESOURCES.CHANNEL,
      ADMIN_PERMISSION_ACTIONS.READ
    ) &&
    hasPermission(
      actor,
      ADMIN_PERMISSION_RESOURCES.CHANNEL,
      ADMIN_PERMISSION_ACTIONS.WRITE
    )
  )
}

export async function listChannelRoutingPools(channelId: number) {
  const entries = new Map<string, RoutingPoolView>()
  let page = 1
  let total = 1
  while (entries.size < total) {
    const data = await listRoutingPools({
      scope: 'all',
      channel_id: channelId,
      model: '',
      routing_group: '',
      mode: '',
      page,
    })
    const previousSize = entries.size
    for (const pool of data.items) {
      entries.set(
        monitorTargetKey({
          userId: pool.user_id,
          group: pool.routing_group,
          model: pool.model,
        }),
        pool
      )
    }
    total = data.total
    if (entries.size < total && entries.size === previousSize) {
      throw new Error('Failed to load all routing pools')
    }
    page++
  }
  return [...entries.values()]
}

export type ChannelPoolUpdate = {
  target: MonitorTarget
  changed: boolean
  error?: unknown
}

export async function disableChannelInPools(
  channelId: number,
  targets: MonitorTarget[],
  isDirty: (target: MonitorTarget) => boolean
): Promise<ChannelPoolUpdate[]> {
  if (targets.some(isDirty)) {
    throw new Error('Save or reset related pool drafts first.')
  }
  const ordered = [
    ...new Map(
      targets.map((target) => [monitorTargetKey(target), target])
    ).values(),
  ].sort((a, b) => Number(!!a.userId) - Number(!!b.userId))
  const results: ChannelPoolUpdate[] = []
  for (const target of ordered) {
    try {
      if (isDirty(target)) {
        throw new Error('Save or reset related pool drafts first.')
      }
      const actor = useAuthStore.getState().auth.user
      if (target.userId) {
        const config = await getUserRoutingPool(
          target.userId,
          target.group,
          target.model
        )
        if (!canAccessUserGroupConfig(actor, config.user_role, 'write')) {
          throw new Error('No permission to perform this action')
        }
        const channels = config.pool_channels ?? []
        if (
          !channels.some(
            (entry) => entry.channel_id === channelId && entry.enabled
          )
        ) {
          results.push({ target, changed: false })
          continue
        }
        const inherited = !config.configured || config.source !== 'user'
        const rows = new Map(
          config.channels.map((row) => [row.channel_id, row])
        )
        await patchUserChannelRouting(target.userId, {
          model: target.model,
          routing_group: target.group,
          revision: config.revision,
          overrides: channels.map((entry) => {
            const row = rows.get(entry.channel_id)
            if (!row) throw new Error('Failed to load all routing pools')
            return {
              channel_id: entry.channel_id,
              priority_override: inherited
                ? entry.priority
                : row.priority_override,
              weight_override: inherited
                ? entry.weight
                : (row.weight_override ?? null),
              enabled: entry.channel_id === channelId ? false : entry.enabled,
              ...(row.retry_times_override !== undefined
                ? { retry_times_override: row.retry_times_override }
                : {}),
            }
          }),
        })
      } else {
        if (!canEditDefaultPool(actor)) {
          throw new Error('No permission to perform this action')
        }
        const data = await getRoutingPool(target.group, target.model)
        const pool = data.pool ?? {
          model: target.model,
          routing_group: target.group,
          mode: data.effective_mode || 'priority',
          revision: '',
          channels: data.candidates
            .filter((row) => row.available)
            .map((row) => ({
              channel_id: row.channel_id,
              priority: row.priority,
              weight: row.weight,
              enabled: true,
            })),
        }
        if (
          !pool.channels.some(
            (entry) => entry.channel_id === channelId && entry.enabled
          )
        ) {
          results.push({ target, changed: false })
          continue
        }
        await saveRoutingPool({
          ...pool,
          channels: pool.channels.map((entry) =>
            entry.channel_id === channelId
              ? { ...entry, enabled: false }
              : entry
          ),
        })
      }
      results.push({ target, changed: true })
    } catch (error) {
      results.push({ target, changed: false, error })
    }
  }
  return results
}
