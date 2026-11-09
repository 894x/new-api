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
import type { PoolChannelDraft } from './pool-channel-list'

export type PoolPriorityGroup = {
  priority: number
  channels: PoolChannelDraft[]
}

// Make room for an independent group by shifting only the nearest occupied
// priority chain. Keep all existing ties and untouched inherited values.
export function detachPoolChannel(
  groups: PoolPriorityGroup[],
  channelId: number,
  position: number
): PoolChannelDraft[] | null {
  const sourceIndex = groups.findIndex((group) =>
    group.channels.some((channel) => channel.channel_id === channelId)
  )
  if (
    sourceIndex < 0 ||
    groups.some(
      (group) =>
        !Number.isInteger(group.priority) ||
        group.priority < 0 ||
        group.priority > 2147483647
    )
  ) {
    return null
  }
  const source = groups[sourceIndex]
  const channel = source.channels.find(
    (entry) => entry.channel_id === channelId
  )
  if (!channel) return null
  const remaining = groups.flatMap((group) => {
    const channels = group.channels.filter(
      (entry) => entry.channel_id !== channelId
    )
    return channels.length ? [{ ...group, channels }] : []
  })
  let index = position
  if (source.channels.length === 1 && sourceIndex < position) index--
  index = Math.max(0, Math.min(index, remaining.length))
  if (
    !remaining.length ||
    (source.channels.length === 1 && index === sourceIndex)
  ) {
    return null
  }

  const above = remaining[index - 1]?.priority
  const below = remaining[index]?.priority
  const shifted = new Map<number, number>()
  let priority: number | undefined
  if (above === undefined && below !== undefined && below < 2147483647) {
    priority = below + 1
  } else if (below === undefined && above !== undefined && above > 0) {
    priority = above - 1
  } else if (above !== undefined && below !== undefined && above - below > 1) {
    priority = above - 1
  }

  if (priority === undefined) {
    for (let cursor = index - 1; cursor >= 0; cursor--) {
      const value = remaining[cursor].priority
      const previous = remaining[cursor - 1]?.priority ?? 2147483648
      if (value < 2147483647 && previous - value > 1) {
        for (let moved = cursor; moved < index; moved++) {
          shifted.set(remaining[moved].priority, remaining[moved].priority + 1)
        }
        priority = above
        break
      }
    }
  }
  if (priority === undefined) {
    for (let cursor = index; cursor < remaining.length; cursor++) {
      const value = remaining[cursor].priority
      const next = remaining[cursor + 1]?.priority ?? -1
      if (value > 0 && value - next > 1) {
        for (let moved = index; moved <= cursor; moved++) {
          shifted.set(remaining[moved].priority, remaining[moved].priority - 1)
        }
        priority = below
        break
      }
    }
  }
  if (priority === undefined) return null
  const channels = remaining.map((group) =>
    group.channels.map((entry) => {
      const target = shifted.get(group.priority)
      return target === undefined ? entry : { ...entry, priority: target }
    })
  )
  channels.splice(index, 0, [{ ...channel, priority }])
  return channels.flat()
}
