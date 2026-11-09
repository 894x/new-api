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
import { useId, useState } from 'react'
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
    fill: 'bg-amber-400/90',
  },
  very_slow: {
    label: 'Very slow response',
    tone: 'text-orange-600 dark:text-orange-400',
    fill: 'bg-orange-500/90',
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
  excluded: {
    label: 'Excluded by monitoring policy',
    tone: 'text-muted-foreground',
    fill: 'bg-slate-400/50',
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
    label: 'Monitoring data is stale',
    tone: 'text-muted-foreground',
    fill: 'bg-muted-foreground/25',
  },
  pending: {
    label: 'Waiting for first observation',
    tone: 'text-muted-foreground',
    fill: 'bg-muted-foreground/25',
  },
  empty: {
    label: 'No monitoring data',
    tone: 'text-muted-foreground',
    fill: 'bg-muted-foreground/25',
  },
}

const bucketStates = [
  'healthy',
  'slow',
  'very_slow',
  'unavailable',
  'configuration',
  'unsupported',
  'excluded',
] as const

const tooltipLabels: Record<string, string> = {
  healthy: 'Healthy',
  slow: 'Slow',
  very_slow: 'Very slow',
  unavailable: 'Failed',
  configuration: 'Configuration',
  unsupported: 'Unsupported',
  excluded: 'Excluded',
}

export function ChannelMonitoringStrip(props: {
  channelName: string
  summary?: ChannelMonitorSummary
  loading: boolean
  error: boolean
  maxSampleCount?: number
  showTimelineLabels?: boolean
}) {
  const { t, i18n } = useTranslation()
  const locale = toIntlLocale(i18n.resolvedLanguage || i18n.language)
  const volumeHintId = useId()
  const [slot, setSlot] = useState(95)
  const [open, setOpen] = useState(false)
  const cache = props.error ? undefined : props.summary?.cache
  const cacheRate = cache?.hit_rate
  const cacheValue =
    cacheRate == null ? '—' : `${formatNumber(cacheRate, locale)}%`
  let cacheHint = t('Cache statistics unavailable')
  if (cache) {
    cacheHint = t(
      'Last 24 hours for this channel and model: cache read tokens divided by input tokens. Only successful requests are included.'
    )
    if (cacheRate == null) cacheHint += `\n${t('No data')}`
    if (cache.truncated) cacheHint += `\n${t('Partial cache data')}`
  }
  const cacheMetric = (
    <span
      className='text-foreground tabular-nums'
      title={`${t('Cache hit rate')}\n${cacheHint}`}
      aria-label={`${t('Cache hit rate')}: ${cacheValue}`}
    >
      {cacheValue}
    </span>
  )
  if (props.loading && !props.summary) {
    return (
      <div className='flex min-w-0 items-center gap-3 text-[11px]'>
        <Skeleton
          className='h-8 min-w-0 flex-1'
          aria-label={t('Loading monitoring data')}
        />
        {cacheMetric}
      </div>
    )
  }
  if (props.error || !props.summary) {
    return (
      <p
        role='status'
        className='text-muted-foreground flex flex-wrap items-center gap-3 text-xs'
      >
        <span>{t('Monitoring unavailable')}</span>
        {cacheMetric}
      </p>
    )
  }
  const summary = props.summary
  let status = states[summary.state] ?? states.empty
  if (summary.state === 'disabled' || summary.state === 'unregistered') {
    return (
      <p className='text-muted-foreground flex flex-wrap items-center gap-3 text-[11px]'>
        <span>{t(status.label)}</span>
        {cacheMetric}
      </p>
    )
  }
  const bucket = summary.buckets[slot]
  const bucketStatus = states[bucket?.state ?? 'empty'] ?? states.empty
  const latest = summary.latest
  const maxSampleCount = Math.max(
    1,
    props.maxSampleCount ??
      summary.buckets.reduce((count, point) => Math.max(count, point.count), 0)
  )
  const currentBucket = summary.buckets.at(-1)
  const performance = currentBucket?.performance
  const hasTTFT = performance?.ttft_p50_ms != null
  const latency =
    performance?.ttft_p50_ms ?? performance?.duration_p50_ms ?? null
  const latencyLabel = t(
    hasTTFT ? 'P50 first response' : 'P50 request duration'
  )
  const latencyValue =
    latency === null ? '—' : `≈ ${formatNumber(latency, locale)} ms`
  const sampleCount = hasTTFT
    ? (performance?.ttft_count ?? 0)
    : (performance?.duration_count ?? 0)
  const intervalLabel = currentBucket
    ? `${dayjs(currentBucket.timestamp * 1000).format('HH:mm')} – ${dayjs((currentBucket.timestamp + 900) * 1000).format('HH:mm')}`
    : '—'
  const performanceHint = [
    t('Current 15-minute interval: {{interval}}', { interval: intervalLabel }),
    t('Successful samples: {{count}}', {
      count: formatNumber(sampleCount, locale),
    }),
    latency === null
      ? t('No complete performance data for this interval')
      : t(
          'P50 is estimated from successful probes and live traffic with less than 1.6% rounding error. Failed and excluded requests are omitted.'
        ),
  ].join('\n')
  if (
    latency !== null &&
    ['healthy', 'slow', 'very_slow'].includes(summary.state)
  ) {
    let latencyState = 'very_slow'
    if (latency < 5000) latencyState = 'healthy'
    else if (latency < 10000) latencyState = 'slow'
    status = states[latencyState]
  }
  const hasLatestPerformance =
    latest && ['healthy', 'slow', 'very_slow'].includes(latest.state)
  const successRateValue =
    summary.success_rate === null
      ? '—'
      : `${formatNumber(summary.success_rate, locale)}%`
  const bucketCounts =
    bucket?.state_counts ??
    (bucket && bucket.count > 0 ? { [bucket.state]: bucket.count } : {})
  return (
    <div
      className='min-w-0 space-y-1.5'
      aria-label={t('Monitoring for {{channel}}', {
        channel: props.channelName,
      })}
    >
      <div className='flex min-w-0 flex-wrap items-center gap-x-3 gap-y-1 text-[11px]'>
        <span
          role='img'
          aria-label={t(status.label)}
          title={t(status.label)}
          className={cn('inline-flex items-center', status.tone)}
        >
          <span className='size-1.5 rounded-full bg-current' />
        </span>
        {latest && !hasLatestPerformance && latest.status_code > 0 && (
          <span className='text-muted-foreground'>
            HTTP {latest.status_code}
          </span>
        )}
        <span
          className='text-foreground tabular-nums'
          title={`${latencyLabel}\n${performanceHint}`}
          aria-label={`${latencyLabel}: ${latencyValue}`}
        >
          P50 {latencyValue}
        </span>
        {hasLatestPerformance && latest.tokens_per_second !== null && (
          <span
            className='text-foreground tabular-nums'
            title={t('Latest generation speed')}
            aria-label={`${t('Latest generation speed')}: ${formatNumber(latest.tokens_per_second, locale)} t/s`}
          >
            {formatNumber(latest.tokens_per_second, locale)} t/s
          </span>
        )}
        {cacheMetric}
        <span
          className='text-foreground ml-auto tabular-nums'
          title={`24h ${t('Success rate')}`}
          aria-label={`24h ${t('Success rate')}: ${successRateValue}`}
        >
          {successRateValue}
        </span>
      </div>
      <TooltipProvider>
        <Tooltip>
          <TooltipTrigger
            render={
              <Button
                type='button'
                variant='ghost'
                className='h-4 w-full items-end gap-px rounded-none p-0 hover:bg-transparent'
                aria-describedby={
                  props.showTimelineLabels === false
                    ? `${volumeHintId}-tooltip`
                    : `${volumeHintId} ${volumeHintId}-tooltip`
                }
                aria-label={t('24-hour channel history for {{channel}}', {
                  channel: props.channelName,
                })}
                onPointerMove={(event) => {
                  const index = (
                    event.target as HTMLElement
                  ).closest<HTMLElement>('[data-bucket]')?.dataset.bucket
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
                {summary.buckets.map((point, index) => {
                  const counts =
                    point.count > 0
                      ? (point.state_counts ?? { [point.state]: point.count })
                      : {}
                  return (
                    <span
                      key={point.timestamp}
                      data-bucket={index}
                      style={{
                        height:
                          point.count > 0
                            ? 3 +
                              (9 * Math.log1p(point.count)) /
                                Math.log1p(
                                  Math.max(maxSampleCount, point.count)
                                )
                            : 2,
                      }}
                      className='bg-muted-foreground/25 flex min-w-0 flex-1 flex-col-reverse overflow-hidden rounded-[1px]'
                    >
                      {bucketStates
                        .filter((state) => (counts[state] ?? 0) > 0)
                        .map((state) => (
                          <span
                            key={state}
                            data-state={state}
                            className={cn(
                              'block w-full shrink-0',
                              states[state].fill
                            )}
                            style={{
                              height: `${(counts[state] * 100) / point.count}%`,
                            }}
                          />
                        ))}
                    </span>
                  )
                })}
              </Button>
            }
          />
          <TooltipContent
            id={`${volumeHintId}-tooltip`}
            role='tooltip'
            className='max-w-[calc(100vw-2rem)] flex-col items-start gap-1 text-xs'
          >
            {props.showTimelineLabels === false && (
              <p>{t('Bar height shows sample volume')}</p>
            )}
            {bucket && (
              <>
                <p className='whitespace-nowrap'>
                  {dayjs(bucket.timestamp * 1000).format('MM-DD HH:mm')} –{' '}
                  {dayjs((bucket.timestamp + 900) * 1000).format('HH:mm')}
                </p>
                <div className='flex flex-wrap gap-x-3 gap-y-1'>
                  {bucketStates
                    .filter((state) => (bucketCounts[state] ?? 0) > 0)
                    .map((state) => (
                      <span
                        key={state}
                        className='inline-flex items-center gap-1 whitespace-nowrap'
                      >
                        <span
                          className={cn(
                            'size-2 rounded-[1px]',
                            states[state].fill
                          )}
                        />
                        {t(tooltipLabels[state])}{' '}
                        {formatNumber(bucketCounts[state], locale)}
                      </span>
                    ))}
                  {bucket.count === 0 && <span>{t('No data')}</span>}
                </div>
              </>
            )}
          </TooltipContent>
        </Tooltip>
      </TooltipProvider>
      {props.showTimelineLabels !== false && (
        <div className='text-muted-foreground flex justify-between text-[11px]'>
          <span>{t('24 hours ago')}</span>
          <span id={volumeHintId}>{t('Bar height shows sample volume')}</span>
          <span>
            {summary.truncated ? t('Partial monitoring history') : t('Now')}
          </span>
        </div>
      )}
      <Dialog
        open={open}
        onOpenChange={setOpen}
        title={t('Monitoring details')}
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
            <dt className='text-muted-foreground'>{t('Sample count')}</dt>
            <dd>{formatNumber(bucket.count, locale)}</dd>
            {bucket.source_counts && (
              <>
                <dt className='text-muted-foreground'>{t('Active probes')}</dt>
                <dd>{formatNumber(bucket.source_counts.probe ?? 0, locale)}</dd>
                <dt className='text-muted-foreground'>{t('Live traffic')}</dt>
                <dd>
                  {formatNumber(bucket.source_counts.traffic ?? 0, locale)}
                </dd>
              </>
            )}
            <dt className='text-muted-foreground'>{t('Successful samples')}</dt>
            <dd>{formatNumber(bucket.success_count, locale)}</dd>
            {bucketStates
              .filter((state) => (bucketCounts[state] ?? 0) > 0)
              .map((state) => (
                <div key={state} className='col-span-2 grid grid-cols-subgrid'>
                  <dt className='text-muted-foreground inline-flex items-center gap-1.5'>
                    <span
                      className={cn('size-2 rounded-[1px]', states[state].fill)}
                    />
                    {t(states[state].label)}
                  </dt>
                  <dd>{formatNumber(bucketCounts[state], locale)}</dd>
                </div>
              ))}
            <dt className='text-muted-foreground'>{t('P50 first response')}</dt>
            <dd>
              {bucket.performance?.ttft_p50_ms == null
                ? '—'
                : `≈ ${formatNumber(bucket.performance.ttft_p50_ms, locale)} ms`}
            </dd>
            <dt className='text-muted-foreground'>
              {t('P50 request duration')}
            </dt>
            <dd>
              {bucket.performance?.duration_p50_ms == null
                ? '—'
                : `≈ ${formatNumber(bucket.performance.duration_p50_ms, locale)} ms`}
            </dd>
            <dt className='text-muted-foreground'>
              {t('Latest successful request duration')}
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
