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
import { Reorder } from 'motion/react'
import { useTranslation } from 'react-i18next'

import { AutoGroupOrderItem } from '@/components/auto-group-order-item'
import { Badge } from '@/components/ui/badge'
import { Input } from '@/components/ui/input'
import { Switch } from '@/components/ui/switch'

import type { PoolCandidate, PoolChannel } from './api'

export function PoolChannelList(props: {
  channels: PoolChannel[]
  candidates: PoolCandidate[]
  disabled: boolean
  onChange: (channels: PoolChannel[]) => void
}) {
  const { t } = useTranslation()
  const candidates = new Map(
    props.candidates.map((candidate) => [candidate.channel_id, candidate])
  )
  const byID = new Map(
    props.channels.map((channel) => [String(channel.channel_id), channel])
  )

  function reorder(ids: string[]) {
    if (props.disabled) return
    const channels = ids.flatMap((id, index) => {
      const entry = byID.get(id)
      return entry ? [{ ...entry, priority: ids.length - index }] : []
    })
    props.onChange(channels)
  }

  function move(index: number, direction: 'up' | 'down') {
    const target = index + (direction === 'up' ? -1 : 1)
    if (target < 0 || target >= props.channels.length) return
    const ids = props.channels.map((entry) => String(entry.channel_id))
    const current = ids[index]
    ids[index] = ids[target]
    ids[target] = current
    reorder(ids)
  }

  function update(id: number, patch: Partial<PoolChannel>) {
    if (!props.disabled) {
      props.onChange(
        props.channels.map((entry) =>
          entry.channel_id === id ? { ...entry, ...patch } : entry
        )
      )
    }
  }

  return (
    <div className='space-y-2'>
      <div
        aria-hidden='true'
        className='text-muted-foreground hidden items-center gap-3 pr-28 pl-10 text-xs sm:flex'
      >
        <span className='min-w-0 flex-1'>{t('Channel Name')}</span>
        <span className='w-20 shrink-0'>{t('Base priority')}</span>
        <span className='w-16 shrink-0'>{t('Weight')}</span>
        <span className='w-12 shrink-0'>{t('Enabled in pool')}</span>
      </div>
      <Reorder.Group
        axis='y'
        values={props.channels.map((entry) => String(entry.channel_id))}
        onReorder={reorder}
        className='space-y-2'
      >
        {props.channels.map((entry, index) => {
          const candidate = candidates.get(entry.channel_id)
          const name = candidate?.channel_name ?? `#${entry.channel_id}`
          return (
            <AutoGroupOrderItem
              key={entry.channel_id}
              group={String(entry.channel_id)}
              labelClassName='sm:flex-1 sm:basis-0'
              label={
                <span className='flex min-w-0 items-center gap-2'>
                  <span className='truncate' title={name}>
                    {name}
                  </span>
                  <span className='text-muted-foreground shrink-0 text-xs'>
                    #{entry.channel_id}
                  </span>
                  {!candidate?.available && (
                    <Badge variant='outline'>{t('Unavailable')}</Badge>
                  )}
                </span>
              }
              index={index}
              count={props.channels.length}
              disabled={props.disabled}
              onMove={move}
              onRemove={() =>
                props.onChange(
                  props.channels.filter(
                    (channel) => channel.channel_id !== entry.channel_id
                  )
                )
              }
            >
              <div className='flex w-full flex-wrap items-center gap-3 sm:w-auto sm:shrink-0'>
                <label className='flex items-center gap-2 text-xs'>
                  <span className='sm:sr-only'>{t('Base priority')}</span>
                  <Input
                    type='number'
                    min={0}
                    max={2147483647}
                    step={1}
                    value={
                      Number.isFinite(entry.priority) ? entry.priority : ''
                    }
                    disabled={props.disabled}
                    className='h-8 w-20'
                    aria-label={t('Priority for {{channel}}', {
                      channel: name,
                    })}
                    onChange={(event) =>
                      update(entry.channel_id, {
                        priority: event.target.valueAsNumber,
                      })
                    }
                  />
                </label>
                <label className='flex items-center gap-2 text-xs'>
                  <span className='sm:sr-only'>{t('Weight')}</span>
                  <Input
                    type='number'
                    min={0}
                    max={2147483637}
                    step={1}
                    value={Number.isFinite(entry.weight) ? entry.weight : ''}
                    disabled={props.disabled}
                    className='h-8 w-16'
                    aria-label={t('Weight for {{channel}}', { channel: name })}
                    onChange={(event) =>
                      update(entry.channel_id, {
                        weight: event.target.valueAsNumber,
                      })
                    }
                  />
                </label>
                <div className='flex items-center gap-2 text-xs sm:w-12 sm:justify-center'>
                  <span className='sm:sr-only'>{t('Enabled in pool')}</span>
                  <Switch
                    checked={entry.enabled}
                    disabled={props.disabled}
                    aria-label={t('Enable {{channel}} in this pool', {
                      channel: name,
                    })}
                    onCheckedChange={(enabled) =>
                      update(entry.channel_id, { enabled })
                    }
                  />
                </div>
              </div>
            </AutoGroupOrderItem>
          )
        })}
      </Reorder.Group>
    </div>
  )
}
