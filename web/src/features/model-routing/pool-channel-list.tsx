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
import {
  Fragment,
  useId,
  useLayoutEffect,
  useRef,
  useState,
  type ReactElement,
} from 'react'
import { useTranslation } from 'react-i18next'

import { AutoGroupOrderItem } from '@/components/auto-group-order-item'
import { ConfirmDialog } from '@/components/confirm-dialog'
import { StatusBadge } from '@/components/status-badge'
import { TruncatedText } from '@/components/truncated-text'
import { Badge } from '@/components/ui/badge'
import { Input } from '@/components/ui/input'
import { Switch } from '@/components/ui/switch'
import { TooltipProvider } from '@/components/ui/tooltip'
import { ChannelMonitoringStrip } from '@/features/channel-monitoring/monitoring-strip'
import { useChannelMonitoring } from '@/features/channel-monitoring/use-channel-monitoring'
import { toIntlLocale } from '@/i18n/languages'
import { formatNumber } from '@/lib/format'
import { cn } from '@/lib/utils'

import type { PoolCandidate, PoolChannel } from './api'
import type { EarlyChunkPool } from './early-chunk'
import { EarlyChunkModeSelect } from './early-chunk-settings'
import { detachPoolChannel } from './priority-groups'

export interface PoolChannelDraft extends Omit<
  PoolChannel,
  'priority' | 'weight'
> {
  priority: number | null
  weight: number | null
  retry_times_override?: number | null
}

function PriorityDropZone(props: {
  index: number
  active: boolean
}): ReactElement {
  const { t } = useTranslation()
  return (
    <li
      aria-hidden='true'
      data-priority-position={props.index}
      className={cn(
        'flex items-center justify-center',
        props.active
          ? 'text-muted-foreground h-6 rounded-md border border-dashed text-[10px]'
          : 'h-2'
      )}
    >
      {props.active && <span>+ {t('Priority')}</span>}
    </li>
  )
}

export function PoolChannelList(props: {
  model?: string
  channels: PoolChannelDraft[]
  candidates: PoolCandidate[]
  disabled: boolean
  validationAttempt?: number
  allowInheritance?: boolean
  removable?: boolean
  earlyChunk?: EarlyChunkPool | null
  onEarlyChunkChange?: (value: EarlyChunkPool) => void
  onChange: (channels: PoolChannelDraft[]) => void
}) {
  const { t, i18n } = useTranslation()
  const locale = toIntlLocale(i18n.resolvedLanguage || i18n.language)
  const [removingChannelId, setRemovingChannelId] = useState<number | null>(
    null
  )
  const [editingPriority, setEditingPriority] = useState<{
    channelId: number
    priority: number
  } | null>(null)
  const [draggingChannel, setDraggingChannel] = useState(false)
  const controlId = useId()
  const listRef = useRef<HTMLDivElement>(null)
  const pendingFocus = useRef<string | null>(null)
  useLayoutEffect(() => {
    if (editingPriority || !pendingFocus.current) return
    const trigger = document.querySelector<HTMLButtonElement>(
      `[id="${pendingFocus.current}"]`
    )
    trigger?.focus()
    pendingFocus.current = null
  }, [editingPriority])
  const monitoring = useChannelMonitoring(
    props.model,
    props.channels.map((channel) => channel.channel_id)
  )
  const observations = new Map(
    monitoring.data?.map((entry) => [entry.channel_id, entry])
  )
  let maxSampleCount = 1
  for (const summary of monitoring.data ?? []) {
    for (const bucket of summary.buckets) {
      maxSampleCount = Math.max(maxSampleCount, bucket.count)
    }
  }
  const candidates = new Map(
    props.candidates.map((candidate) => [candidate.channel_id, candidate])
  )
  const grouped = new Map<number, PoolChannelDraft[]>()
  for (const entry of props.channels) {
    let priority =
      entry.priority ?? candidates.get(entry.channel_id)?.priority ?? 0
    if (!Number.isFinite(priority)) priority = 0
    if (editingPriority?.channelId === entry.channel_id) {
      priority = editingPriority.priority
    }
    const entries = grouped.get(priority) ?? []
    entries.push(entry)
    grouped.set(priority, entries)
  }
  const groups = Array.from(grouped, ([priority, channels]) => ({
    id: channels
      .map((channel) => channel.channel_id)
      .sort((left, right) => left - right)
      .join(','),
    priority,
    channels,
  })).sort((left, right) => right.priority - left.priority)

  function reorder(ids: string[]): void {
    if (
      props.disabled ||
      ids.length !== groups.length ||
      new Set(ids).size !== groups.length ||
      ids.some((id) => !groups.some((group) => group.id === id)) ||
      ids.every((id, index) => id === groups[index].id)
    ) {
      return
    }
    props.onChange(
      ids.flatMap((id, index) => {
        const group = groups.find((group) => group.id === id)
        if (!group) return []
        const target = groups[index]?.priority ?? group.priority
        return group.channels.map((entry) =>
          group.priority === target ? entry : { ...entry, priority: target }
        )
      })
    )
  }

  function move(index: number, direction: 'up' | 'down'): void {
    const target = index + (direction === 'up' ? -1 : 1)
    if (target < 0 || target >= groups.length) return
    const ids = groups.map((group) => group.id)
    const current = ids[index]
    ids[index] = ids[target]
    ids[target] = current
    reorder(ids)
  }

  function update(id: number, patch: Partial<PoolChannelDraft>) {
    if (!props.disabled) {
      props.onChange(
        props.channels.map((entry) =>
          entry.channel_id === id ? { ...entry, ...patch } : entry
        )
      )
    }
  }

  function transferChannel(
    channelId: number,
    priority: number,
    beforeChannelId?: number | null
  ): void {
    const entry = props.channels.find(
      (channel) => channel.channel_id === channelId
    )
    if (props.disabled || !entry) return
    const original = entry.priority ?? candidates.get(channelId)?.priority ?? 0
    if (original === priority && beforeChannelId === undefined) return
    const moved = original === priority ? entry : { ...entry, priority }
    const channels = groups.flatMap((group) => {
      const remaining = group.channels.filter(
        (channel) => channel.channel_id !== channelId
      )
      if (group.priority !== priority) return remaining
      let index = remaining.findIndex(
        (channel) => channel.channel_id === beforeChannelId
      )
      if (index < 0) index = remaining.length
      return [...remaining.slice(0, index), moved, ...remaining.slice(index)]
    })
    if (channels.every((channel, index) => channel === props.channels[index])) {
      return
    }
    props.onChange(channels)
  }

  return (
    <div ref={listRef} className='@container/channel-list space-y-2'>
      <Reorder.Group
        axis='y'
        values={groups.map((group) => group.id)}
        onReorder={reorder}
        className='space-y-0'
      >
        {groups.map((group, index) => {
          const priorityLabel = `${t('Priority')} ${formatNumber(group.priority, locale)}`
          return (
            <Fragment key={group.id}>
              <PriorityDropZone index={index} active={draggingChannel} />
              <AutoGroupOrderItem
                group={group.id}
                label={priorityLabel}
                reorderLabel={priorityLabel}
                labelClassName='flex-1 text-xs'
                compact
                index={index}
                count={groups.length}
                disabled={props.disabled}
                onMove={move}
                footer={
                  <div
                    role='group'
                    aria-label={priorityLabel}
                    data-priority-group={group.priority}
                  >
                    <ul className='space-y-2'>
                      {group.channels.map((entry) => {
                        const candidate = candidates.get(entry.channel_id)
                        const name =
                          candidate?.channel_name ?? `#${entry.channel_id}`
                        const discount = candidate?.effective_cost_discount
                        const observation = monitoring.isError
                          ? undefined
                          : observations.get(entry.channel_id)
                        let tone: 'danger' | 'warning' | undefined
                        if (!entry.enabled) {
                          tone = 'warning'
                        } else if (
                          candidate?.available === false ||
                          observation?.state === 'unavailable'
                        ) {
                          tone = 'danger'
                        } else if (
                          observation &&
                          ['slow', 'very_slow', 'configuration'].includes(
                            observation.state
                          )
                        ) {
                          tone = 'warning'
                        }
                        return (
                          <AutoGroupOrderItem
                            key={entry.channel_id}
                            group={String(entry.channel_id)}
                            compact
                            reorderable={false}
                            className={cn(
                              tone === 'danger' &&
                                'border-destructive/35 bg-destructive/5',
                              tone === 'warning' &&
                                'border-warning/40 bg-warning/5'
                            )}
                            footer={
                              props.model && (
                                <ChannelMonitoringStrip
                                  showTimelineLabels={false}
                                  channelName={name}
                                  summary={observations.get(entry.channel_id)}
                                  loading={monitoring.isLoading}
                                  error={monitoring.isError}
                                  maxSampleCount={maxSampleCount}
                                />
                              )
                            }
                            labelClassName='flex-1 basis-0 text-xs'
                            removeClassName='text-destructive hover:bg-destructive/10 hover:text-destructive'
                            label={
                              <div
                                className='flex min-w-0 items-center gap-1'
                                title=''
                              >
                                <TooltipProvider>
                                  <TruncatedText
                                    text={name}
                                    maxWidth='max-w-full'
                                    className='min-w-0'
                                  />
                                </TooltipProvider>
                                {discount != null &&
                                  Number.isFinite(discount) &&
                                  discount >= 0 &&
                                  discount <= 1 && (
                                    <Badge
                                      variant='outline'
                                      className='h-4 px-1 text-[10px] tabular-nums'
                                      title={t('Upstream cost discount')}
                                      aria-label={`${t('Upstream cost discount')}: ${discount.toLocaleString(locale, { maximumFractionDigits: 20 })}×`}
                                    >
                                      {discount.toLocaleString(locale, {
                                        style: 'percent',
                                        maximumFractionDigits: 20,
                                      })}
                                    </Badge>
                                  )}
                                <span className='text-muted-foreground shrink-0 text-[10px]'>
                                  #{entry.channel_id}
                                </span>
                                {(!entry.enabled || !candidate?.available) && (
                                  <StatusBadge
                                    variant={tone ?? 'warning'}
                                    copyable={false}
                                    className='h-4 px-1 text-[10px]'
                                  >
                                    {entry.enabled
                                      ? t('Unavailable')
                                      : t('Disabled')}
                                  </StatusBadge>
                                )}
                              </div>
                            }
                            index={index}
                            count={groups.length}
                            disabled={props.disabled}
                            settingsOpenRequest={
                              (entry.priority !== null &&
                                (!Number.isInteger(entry.priority) ||
                                  entry.priority < 0 ||
                                  entry.priority > 2147483647)) ||
                              (entry.retry_times_override != null &&
                                (!Number.isInteger(
                                  entry.retry_times_override
                                ) ||
                                  entry.retry_times_override < 0 ||
                                  entry.retry_times_override > 99))
                                ? props.validationAttempt
                                : undefined
                            }
                            settingsTriggerId={`${controlId}-${entry.channel_id}`}
                            onSettingsOpenChange={(open, restoreFocus) => {
                              if (!open && restoreFocus) {
                                pendingFocus.current = `${controlId}-${entry.channel_id}`
                              }
                              setEditingPriority((current) => {
                                if (!open) {
                                  return current?.channelId === entry.channel_id
                                    ? null
                                    : current
                                }
                                return {
                                  channelId: entry.channel_id,
                                  priority: group.priority,
                                }
                              })
                            }}
                            settingsLabel={`${t('Settings')}: ${name}`}
                            settings={
                              <div className='space-y-3'>
                                <div className='space-y-1.5'>
                                  <label className='flex min-w-0 flex-col items-start gap-1.5 text-xs'>
                                    <span className='text-muted-foreground'>
                                      {props.allowInheritance
                                        ? t('User priority')
                                        : t('Base priority')}
                                    </span>
                                    <Input
                                      type='number'
                                      inputMode='numeric'
                                      min={0}
                                      max={2147483647}
                                      step={1}
                                      value={
                                        entry.priority !== null &&
                                        Number.isFinite(entry.priority)
                                          ? entry.priority
                                          : ''
                                      }
                                      placeholder={
                                        props.allowInheritance
                                          ? String(candidate?.priority ?? 0)
                                          : undefined
                                      }
                                      disabled={props.disabled}
                                      className='h-8 w-full [appearance:textfield] px-2 text-xs tabular-nums [&::-webkit-inner-spin-button]:appearance-none [&::-webkit-outer-spin-button]:appearance-none'
                                      aria-label={t(
                                        'Priority for {{channel}}',
                                        {
                                          channel: name,
                                        }
                                      )}
                                      onChange={(event) =>
                                        update(entry.channel_id, {
                                          priority:
                                            props.allowInheritance &&
                                            event.target.value === ''
                                              ? null
                                              : event.target.valueAsNumber,
                                        })
                                      }
                                    />
                                  </label>
                                </div>
                                {props.onEarlyChunkChange && (
                                  <div className='mb-2'>
                                    <EarlyChunkModeSelect
                                      label={t('Early chunk for {{channel}}', {
                                        channel: name,
                                      })}
                                      inherit='pool'
                                      value={
                                        props.earlyChunk?.channels?.[
                                          entry.channel_id
                                        ]
                                      }
                                      disabled={props.disabled}
                                      onChange={(mode) => {
                                        const channels = {
                                          ...props.earlyChunk?.channels,
                                        }
                                        if (mode) {
                                          channels[entry.channel_id] = mode
                                        } else {
                                          delete channels[entry.channel_id]
                                        }
                                        props.onEarlyChunkChange?.({
                                          ...props.earlyChunk,
                                          channels: Object.keys(channels).length
                                            ? channels
                                            : undefined,
                                        })
                                      }}
                                    />
                                  </div>
                                )}
                                {props.allowInheritance && (
                                  <label className='text-muted-foreground mb-1 flex min-w-0 flex-wrap items-center justify-between gap-2 text-xs'>
                                    <span>{t('Maximum retries')}</span>
                                    <Input
                                      type='number'
                                      inputMode='numeric'
                                      min={0}
                                      max={99}
                                      step={1}
                                      className='h-7 w-16 [appearance:textfield] px-1 text-center text-xs tabular-nums [&::-webkit-inner-spin-button]:appearance-none [&::-webkit-outer-spin-button]:appearance-none'
                                      value={
                                        entry.retry_times_override != null &&
                                        Number.isFinite(
                                          entry.retry_times_override
                                        )
                                          ? entry.retry_times_override
                                          : ''
                                      }
                                      placeholder={t('Inherit global')}
                                      aria-label={t('Retries for {{channel}}', {
                                        channel: name,
                                      })}
                                      aria-invalid={
                                        entry.retry_times_override != null &&
                                        (!Number.isInteger(
                                          entry.retry_times_override
                                        ) ||
                                          entry.retry_times_override < 0 ||
                                          entry.retry_times_override > 99)
                                      }
                                      disabled={props.disabled}
                                      onChange={(event) =>
                                        update(entry.channel_id, {
                                          retry_times_override:
                                            event.target.value === ''
                                              ? null
                                              : event.target.valueAsNumber,
                                        })
                                      }
                                    />
                                  </label>
                                )}
                              </div>
                            }
                            onDraggingChange={setDraggingChannel}
                            onDrop={(point) => {
                              if (props.disabled) return
                              const zones =
                                listRef.current?.querySelectorAll<HTMLElement>(
                                  '[data-priority-position]'
                                ) ?? []
                              for (const zone of zones) {
                                const rect = zone.getBoundingClientRect()
                                if (
                                  point.x >= rect.left &&
                                  point.x <= rect.right &&
                                  point.y >= rect.top &&
                                  point.y <= rect.bottom
                                ) {
                                  const channels = detachPoolChannel(
                                    groups,
                                    entry.channel_id,
                                    Number(zone.dataset.priorityPosition)
                                  )
                                  if (channels) props.onChange(channels)
                                  return
                                }
                              }
                              const targets =
                                listRef.current?.querySelectorAll<HTMLElement>(
                                  '[data-priority-group]'
                                ) ?? []
                              for (const target of targets) {
                                const rect = target
                                  .closest('li')
                                  ?.getBoundingClientRect()
                                if (
                                  !rect ||
                                  point.x < rect.left ||
                                  point.x > rect.right ||
                                  point.y < rect.top ||
                                  point.y > rect.bottom
                                ) {
                                  continue
                                }
                                const peers =
                                  target.querySelectorAll<HTMLElement>(
                                    ':scope > ul > [data-order-item]'
                                  )
                                let before: number | null = null
                                for (const peer of peers) {
                                  if (
                                    Number(peer.dataset.orderItem) ===
                                    entry.channel_id
                                  ) {
                                    continue
                                  }
                                  const bounds = peer.getBoundingClientRect()
                                  if (
                                    point.y <
                                    bounds.top + bounds.height / 2
                                  ) {
                                    before = Number(peer.dataset.orderItem)
                                    break
                                  }
                                }
                                transferChannel(
                                  entry.channel_id,
                                  Number(target.dataset.priorityGroup),
                                  before
                                )
                                return
                              }
                            }}
                            onMove={(_, direction) => {
                              const target =
                                groups[index + (direction === 'up' ? -1 : 1)]
                              if (target) {
                                transferChannel(
                                  entry.channel_id,
                                  target.priority
                                )
                              }
                            }}
                            onRemove={
                              props.removable === false
                                ? undefined
                                : () => setRemovingChannelId(entry.channel_id)
                            }
                          >
                            <label className='flex shrink-0 items-center gap-1 text-xs'>
                              <span className='text-muted-foreground'>
                                {t('Weight')}
                              </span>
                              <Input
                                type='number'
                                inputMode='numeric'
                                min={0}
                                max={2147483637}
                                step={1}
                                value={
                                  entry.weight !== null &&
                                  Number.isFinite(entry.weight)
                                    ? entry.weight
                                    : ''
                                }
                                placeholder={
                                  props.allowInheritance
                                    ? String(candidate?.weight ?? 0)
                                    : undefined
                                }
                                disabled={props.disabled}
                                className='h-7 w-14 [appearance:textfield] px-1 text-center text-xs tabular-nums [&::-webkit-inner-spin-button]:appearance-none [&::-webkit-outer-spin-button]:appearance-none'
                                aria-label={t('Weight for {{channel}}', {
                                  channel: name,
                                })}
                                onChange={(event) =>
                                  update(entry.channel_id, {
                                    weight:
                                      props.allowInheritance &&
                                      event.target.value === ''
                                        ? null
                                        : event.target.valueAsNumber,
                                  })
                                }
                              />
                            </label>
                            <div className='flex w-8 shrink-0 items-center justify-center'>
                              <span className='sr-only'>
                                {t('Enabled in pool')}
                              </span>
                              <Switch
                                checked={entry.enabled}
                                disabled={props.disabled}
                                aria-label={t(
                                  'Enable {{channel}} in this pool',
                                  {
                                    channel: name,
                                  }
                                )}
                                onCheckedChange={(enabled) =>
                                  update(entry.channel_id, { enabled })
                                }
                              />
                            </div>
                          </AutoGroupOrderItem>
                        )
                      })}
                    </ul>
                  </div>
                }
              >
                <Badge variant='secondary' className='shrink-0 text-[10px]'>
                  {formatNumber(group.channels.length, locale)} {t('Channels')}
                </Badge>
              </AutoGroupOrderItem>
            </Fragment>
          )
        })}
        <PriorityDropZone index={groups.length} active={draggingChannel} />
      </Reorder.Group>
      <ConfirmDialog
        open={removingChannelId !== null}
        onOpenChange={(open) => {
          if (!open) setRemovingChannelId(null)
        }}
        title={t('Remove channel from pool')}
        desc={t(
          'Remove channel "{{name}}" from this routing pool? Save the card to apply this change.',
          {
            name:
              candidates.get(removingChannelId ?? 0)?.channel_name ??
              `#${removingChannelId}`,
          }
        )}
        confirmText={t('Remove')}
        destructive
        disabled={
          props.disabled ||
          props.removable === false ||
          !props.channels.some(
            (channel) => channel.channel_id === removingChannelId
          )
        }
        handleConfirm={() => {
          if (
            props.disabled ||
            props.removable === false ||
            removingChannelId === null
          ) {
            return
          }
          props.onChange(
            props.channels.filter(
              (channel) => channel.channel_id !== removingChannelId
            )
          )
          setRemovingChannelId(null)
        }}
      />
    </div>
  )
}
