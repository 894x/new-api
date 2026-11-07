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
import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import { AutoGroupOrderItem } from '@/components/auto-group-order-item'
import { ConfirmDialog } from '@/components/confirm-dialog'
import { TruncatedText } from '@/components/truncated-text'
import { Badge } from '@/components/ui/badge'
import { Input } from '@/components/ui/input'
import { Switch } from '@/components/ui/switch'
import { TooltipProvider } from '@/components/ui/tooltip'
import { ChannelMonitoringStrip } from '@/features/channel-monitoring/monitoring-strip'
import { useChannelMonitoring } from '@/features/channel-monitoring/use-channel-monitoring'
import { toIntlLocale } from '@/i18n/languages'
import { cn } from '@/lib/utils'

import type { PoolCandidate, PoolChannel } from './api'

export interface PoolChannelDraft extends Omit<
  PoolChannel,
  'priority' | 'weight'
> {
  priority: number | null
  weight: number | null
  retry_times_override?: number | null
}

export function PoolChannelList(props: {
  model?: string
  channels: PoolChannelDraft[]
  candidates: PoolCandidate[]
  disabled: boolean
  allowInheritance?: boolean
  removable?: boolean
  onChange: (channels: PoolChannelDraft[]) => void
}) {
  const { t, i18n } = useTranslation()
  const locale = toIntlLocale(i18n.resolvedLanguage || i18n.language)
  const [removingChannelId, setRemovingChannelId] = useState<number | null>(
    null
  )
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

  function update(id: number, patch: Partial<PoolChannelDraft>) {
    if (!props.disabled) {
      props.onChange(
        props.channels.map((entry) =>
          entry.channel_id === id ? { ...entry, ...patch } : entry
        )
      )
    }
  }

  return (
    <div className='@container/channel-list space-y-2'>
      <div
        aria-hidden='true'
        className={cn(
          'text-muted-foreground flex items-center gap-1.5 pl-9 text-xs',
          props.removable === false
            ? 'pr-14 @max-[24rem]/channel-list:pr-8'
            : 'pr-20 @max-[24rem]/channel-list:pr-14'
        )}
      >
        <span className='min-w-0 flex-1 truncate' title={t('Channel Name')}>
          {t('Channel Name')}
        </span>
        <span
          className='w-12 shrink-0 truncate'
          title={
            props.allowInheritance ? t('User priority') : t('Base priority')
          }
        >
          {t('Priority')}
        </span>
        <span className='w-12 shrink-0 truncate' title={t('Weight')}>
          {t('Weight')}
        </span>
        <span className='w-8 shrink-0 truncate' title={t('Enabled in pool')}>
          {t('Enable')}
        </span>
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
          const discount = candidate?.effective_cost_discount
          return (
            <AutoGroupOrderItem
              key={entry.channel_id}
              group={String(entry.channel_id)}
              compact
              footer={
                (props.allowInheritance || props.model) && (
                  <>
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
                            Number.isFinite(entry.retry_times_override)
                              ? entry.retry_times_override
                              : ''
                          }
                          placeholder={t('Inherit global')}
                          aria-label={t('Retries for {{channel}}', {
                            channel: name,
                          })}
                          aria-invalid={
                            entry.retry_times_override != null &&
                            (!Number.isInteger(entry.retry_times_override) ||
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
                    {props.model && (
                      <ChannelMonitoringStrip
                        channelName={name}
                        summary={observations.get(entry.channel_id)}
                        loading={monitoring.isLoading}
                        error={monitoring.isError}
                        maxSampleCount={maxSampleCount}
                      />
                    )}
                  </>
                )
              }
              labelClassName='flex-1 basis-0 text-xs'
              removeClassName='text-destructive hover:bg-destructive/10 hover:text-destructive'
              label={
                <div className='flex min-w-0 items-center gap-1' title=''>
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
                  {!candidate?.available && (
                    <Badge variant='outline'>{t('Unavailable')}</Badge>
                  )}
                </div>
              }
              index={index}
              count={props.channels.length}
              disabled={props.disabled}
              onMove={move}
              onRemove={
                props.removable === false
                  ? undefined
                  : () => setRemovingChannelId(entry.channel_id)
              }
            >
              <div className='flex shrink-0 items-center gap-1.5'>
                <label className='flex items-center gap-2 text-xs'>
                  <span className='sr-only'>
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
                      entry.priority !== null && Number.isFinite(entry.priority)
                        ? entry.priority
                        : ''
                    }
                    placeholder={
                      props.allowInheritance
                        ? String(candidate?.priority ?? 0)
                        : undefined
                    }
                    disabled={props.disabled}
                    className='h-7 w-12 [appearance:textfield] px-1 text-center text-xs tabular-nums [&::-webkit-inner-spin-button]:appearance-none [&::-webkit-outer-spin-button]:appearance-none'
                    aria-label={t('Priority for {{channel}}', {
                      channel: name,
                    })}
                    onChange={(event) =>
                      update(entry.channel_id, {
                        priority:
                          props.allowInheritance && event.target.value === ''
                            ? null
                            : event.target.valueAsNumber,
                      })
                    }
                  />
                </label>
                <label className='flex items-center gap-2 text-xs'>
                  <span className='sr-only'>{t('Weight')}</span>
                  <Input
                    type='number'
                    inputMode='numeric'
                    min={0}
                    max={2147483637}
                    step={1}
                    value={
                      entry.weight !== null && Number.isFinite(entry.weight)
                        ? entry.weight
                        : ''
                    }
                    placeholder={
                      props.allowInheritance
                        ? String(candidate?.weight ?? 0)
                        : undefined
                    }
                    disabled={props.disabled}
                    className='h-7 w-12 [appearance:textfield] px-1 text-center text-xs tabular-nums [&::-webkit-inner-spin-button]:appearance-none [&::-webkit-outer-spin-button]:appearance-none'
                    aria-label={t('Weight for {{channel}}', { channel: name })}
                    onChange={(event) =>
                      update(entry.channel_id, {
                        weight:
                          props.allowInheritance && event.target.value === ''
                            ? null
                            : event.target.valueAsNumber,
                      })
                    }
                  />
                </label>
                <div className='flex w-8 items-center justify-center gap-2 text-xs'>
                  <span className='sr-only'>{t('Enabled in pool')}</span>
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
