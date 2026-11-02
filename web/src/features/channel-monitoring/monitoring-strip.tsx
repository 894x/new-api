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
import dayjs from 'dayjs'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import { Dialog } from '@/components/dialog'
import { Button } from '@/components/ui/button'
import { Skeleton } from '@/components/ui/skeleton'
import {
  Tooltip,
  TooltipContent,
  TooltipProvider,
  TooltipTrigger,
} from '@/components/ui/tooltip'
import { toIntlLocale } from '@/i18n/languages'
import { formatNumber } from '@/lib/format'
import { cn } from '@/lib/utils'

import type { ChannelMonitorSummary } from './api'

const states: Record<string, { label: string; tone: string; fill: string }> = {
  healthy: {
    label: 'Connected',
    tone: 'text-emerald-600 dark:text-emerald-400',
    fill: 'bg-emerald-500/80',
  },
  slow: {
    label: 'Slow response',
    tone: 'text-amber-600 dark:text-amber-400',
    fill: 'bg-amber-500/80',
  },
  unavailable: {
    label: 'Connection failed',
    tone: 'text-rose-600 dark:text-rose-400',
    fill: 'bg-rose-500/80',
  },
  configuration: {
    label: 'Monitoring configuration error',
    tone: 'text-amber-600 dark:text-amber-400',
    fill: 'bg-amber-500/80',
  },
  unsupported: {
    label: 'Monitoring protocol unsupported',
    tone: 'text-muted-foreground',
    fill: 'bg-muted-foreground/25',
  },
  disabled: {
    label: 'Monitoring disabled',
    tone: 'text-muted-foreground',
    fill: 'bg-muted-foreground/25',
  },
  unregistered: {
    label: 'This model is not monitored',
    tone: 'text-muted-foreground',
    fill: 'bg-muted-foreground/25',
  },
  paused: {
    label: 'Monitoring paused',
    tone: 'text-muted-foreground',
    fill: 'bg-muted-foreground/25',
  },
  stale: {
    label: 'Probe data is stale',
    tone: 'text-muted-foreground',
    fill: 'bg-muted-foreground/25',
  },
  pending: {
    label: 'Waiting for first probe',
    tone: 'text-muted-foreground',
    fill: 'bg-muted-foreground/25',
  },
  empty: {
    label: 'No probe data',
    tone: 'text-muted-foreground',
    fill: 'bg-muted-foreground/25',
  },
}

export function ChannelMonitoringStrip(props: {
  channelName: string
  summary?: ChannelMonitorSummary
  loading: boolean
  error: boolean
}) {
  const { t, i18n } = useTranslation()
  const locale = toIntlLocale(i18n.resolvedLanguage || i18n.language)
  const [slot, setSlot] = useState(95)
  const [open, setOpen] = useState(false)
  if (props.loading && !props.summary) {
    return (
      <Skeleton
        className='h-8 w-full'
        aria-label={t('Loading monitoring data')}
      />
    )
  }
  if (props.error || !props.summary) {
    return (
      <p role='status' className='text-muted-foreground text-xs'>
        {t('Monitoring unavailable')}
      </p>
    )
  }
  const summary = props.summary
  const status = states[summary.state] ?? states.empty
  if (summary.state === 'disabled' || summary.state === 'unregistered') {
    return (
      <p className='text-muted-foreground text-[11px]'>{t(status.label)}</p>
    )
  }
  const bucket = summary.buckets[slot]
  const bucketStatus = states[bucket?.state ?? 'empty'] ?? states.empty
  const latest = summary.latest
  const hasLatency =
    latest && (latest.state === 'healthy' || latest.state === 'slow')
  const latency = hasLatency ? (latest.ttft_ms ?? latest.latency_ms) : null
  return (
    <div
      className='min-w-0 space-y-1.5'
      aria-label={t('Monitoring for {{channel}}', {
        channel: props.channelName,
      })}
    >
      <div className='flex min-w-0 flex-wrap items-center gap-x-3 gap-y-1 text-[11px]'>
        <span className={cn('inline-flex items-center gap-1', status.tone)}>
          <span className='size-1.5 rounded-full bg-current' />
          {t(status.label)}
        </span>
        <span className='text-muted-foreground'>
          {t(
            hasLatency && latest.ttft_ms !== null
              ? 'First response'
              : 'Probe duration'
          )}{' '}
          <span className='text-foreground tabular-nums'>
            {latency === null ? '—' : `${formatNumber(latency, locale)} ms`}
          </span>
        </span>
        {hasLatency && latest.tokens_per_second !== null && (
          <span className='text-muted-foreground'>
            {t('Generation speed')}{' '}
            <span className='text-foreground tabular-nums'>
              {formatNumber(latest.tokens_per_second, locale)} t/s
            </span>
          </span>
        )}
        <span className='text-muted-foreground ml-auto'>
          24h {t('Probe success rate')}{' '}
          <span className='text-foreground tabular-nums'>
            {summary.success_rate === null
              ? '—'
              : `${formatNumber(summary.success_rate, locale)}%`}
          </span>
        </span>
      </div>
      <TooltipProvider>
        <Tooltip>
          <TooltipTrigger
            render={
              <Button
                type='button'
                variant='ghost'
                className='h-4 w-full gap-px rounded-none p-0 hover:bg-transparent'
                aria-label={t('24-hour probe history for {{channel}}', {
                  channel: props.channelName,
                })}
                onPointerMove={(event) => {
                  const index = (event.target as HTMLElement).dataset.bucket
                  if (index !== undefined) setSlot(Number(index))
                }}
                onKeyDown={(event) => {
                  if (event.key === 'ArrowLeft' || event.key === 'ArrowRight') {
                    event.preventDefault()
                    setSlot((current) =>
                      Math.max(
                        0,
                        Math.min(
                          summary.buckets.length - 1,
                          current + (event.key === 'ArrowLeft' ? -1 : 1)
                        )
                      )
                    )
                  }
                }}
                onClick={() => setOpen(true)}
              >
                {summary.buckets.map((point, index) => (
                  <span
                    key={point.timestamp}
                    data-bucket={index}
                    className={cn(
                      'block h-3 min-w-0 flex-1 rounded-[2px]',
                      (states[point.state] ?? states.empty).fill
                    )}
                  />
                ))}
              </Button>
            }
          />
          <TooltipContent className='space-y-1 text-xs'>
            {bucket && (
              <>
                <p>
                  {dayjs(bucket.timestamp * 1000).format('MM-DD HH:mm')} –{' '}
                  {dayjs((bucket.timestamp + 900) * 1000).format('HH:mm')}
                </p>
                <p>
                  {t(bucketStatus.label)} ·{' '}
                  {t('{{success}} of {{count}} probes succeeded', {
                    success: formatNumber(bucket.success_count, locale),
                    count: formatNumber(bucket.count, locale),
                  })}
                </p>
              </>
            )}
          </TooltipContent>
        </Tooltip>
      </TooltipProvider>
      <div className='text-muted-foreground flex justify-between text-[11px]'>
        <span>{t('24 hours ago')}</span>
        <span>
          {summary.truncated ? t('Partial monitoring history') : t('Now')}
        </span>
      </div>
      <Dialog
        open={open}
        onOpenChange={setOpen}
        title={t('Probe details')}
        description={
          bucket
            ? `${props.channelName} · ${dayjs(bucket.timestamp * 1000).format('MM-DD HH:mm')} – ${dayjs((bucket.timestamp + 900) * 1000).format('HH:mm')}`
            : props.channelName
        }
        contentClassName='sm:max-w-md'
      >
        {bucket && (
          <dl className='grid grid-cols-2 gap-x-4 gap-y-3 text-sm'>
            <dt className='text-muted-foreground'>{t('Status')}</dt>
            <dd>{t(bucketStatus.label)}</dd>
            <dt className='text-muted-foreground'>{t('Probe count')}</dt>
            <dd>{formatNumber(bucket.count, locale)}</dd>
            <dt className='text-muted-foreground'>{t('Successful probes')}</dt>
            <dd>{formatNumber(bucket.success_count, locale)}</dd>
            <dt className='text-muted-foreground'>
              {t('Latest successful duration')}
            </dt>
            <dd>
              {bucket.latency_ms === null
                ? '—'
                : `${formatNumber(bucket.latency_ms, locale)} ms`}
            </dd>
          </dl>
        )}
      </Dialog>
    </div>
  )
}
