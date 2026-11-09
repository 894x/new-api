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
import { useTranslation } from 'react-i18next'

import { Card, CardContent } from '@/components/ui/card'
import { Skeleton } from '@/components/ui/skeleton'
import { toIntlLocale } from '@/i18n/languages'
import { formatNumber } from '@/lib/format'
import { cn } from '@/lib/utils'

import type { MonitorRoute } from './api'
import { monitorChannelStatus, weightedDiscount } from './route-data'

export function RoutingOverview(props: {
  route?: MonitorRoute
  at: number
  names?: Map<number, string>
  loading: boolean
  embedded?: boolean
  compact?: boolean
}) {
  const { t, i18n } = useTranslation()
  const locale = toIntlLocale(i18n.resolvedLanguage || i18n.language)
  const priority = props.route?.mode === 'priority'
  const observed = priority || props.route?.aggregate
  const channels =
    props.route?.channels.filter(
      (channel) =>
        channel.active && channel.status !== 2 && channel.status !== 3
    ) ?? []
  const primary = channels.reduce<(typeof channels)[number] | undefined>(
    (current, channel) =>
      !current || channel.target_share > current.target_share
        ? channel
        : current,
    undefined
  )
  const discount = props.route ? weightedDiscount(props.route.channels) : null
  const healthy = channels.filter((channel) =>
    ['healthy', 'probe'].includes(monitorChannelStatus(channel, props.at))
  ).length
  const availableCount = priority ? channels.length : healthy
  const totalCount = priority
    ? (props.route?.channels.length ?? 0)
    : channels.length
  const completed =
    props.route?.channels.reduce(
      (sum, channel) => sum + (channel.completed_count ?? 0),
      0
    ) ?? 0
  const succeeded =
    props.route?.channels.reduce(
      (sum, channel) => sum + (channel.success_count ?? 0),
      0
    ) ?? 0
  let metricTitle = observed
    ? t('Selection weighted discount')
    : t('Target weighted discount')
  let metricValue =
    discount == null ? '—' : `×${formatNumber(discount, locale)}`
  let metricDescription = observed
    ? t('Based on effective discounts and selection shares')
    : t('Based on effective discounts and target shares')
  if (priority) {
    metricTitle = t('Success rate')
    metricValue =
      completed > 0
        ? `${formatNumber((succeeded / completed) * 100, locale)}%`
        : '—'
    metricDescription = t('Last {{seconds}} seconds', {
      seconds: props.route?.window_seconds ?? 60,
    })
  }
  const metrics = [
    {
      key: 'selections',
      title: t('Routing selections'),
      value: props.route
        ? formatNumber(props.route.selection_count, locale)
        : '—',
      description: t('Last {{seconds}} seconds', {
        seconds: props.route?.window_seconds ?? 60,
      }),
    },
    {
      key: 'primary',
      title: t('Primary destination'),
      value: primary
        ? props.names?.get(primary.channel_id) || `#${primary.channel_id}`
        : '—',
      description: `${primary ? `${formatNumber(primary.target_share * 100, locale)}%` : '—'} · ${observed ? t('Selection share') : t('Target share')}`,
    },
    {
      key: 'quality',
      title: metricTitle,
      value: metricValue,
      description: metricDescription,
    },
    {
      key: 'availability',
      title: priority
        ? t('Available channels')
        : t('Verified healthy channels'),
      value: props.route ? formatNumber(availableCount, locale) : '—',
      suffix: ` / ${props.route ? formatNumber(totalCount, locale) : '—'}`,
      description: priority
        ? t('Channel auto-disable and recovery')
        : t('Quality samples are shared by channel and model'),
    },
  ]
  return (
    <section
      aria-label={t('Routing overview')}
      aria-busy={props.loading}
      data-routing-monitor-overview=''
      className={cn(
        'grid min-w-0 grid-cols-2',
        props.compact
          ? 'bg-muted/20 shrink-0 gap-x-3 gap-y-2 rounded-lg border px-3 py-2 @min-[28rem]/monitor-panel:grid-cols-4'
          : 'gap-3',
        !props.compact &&
          (props.embedded
            ? 'order-1 self-start xl:col-start-1 xl:row-start-1'
            : 'xl:grid-cols-4')
      )}
    >
      {metrics.map((metric) =>
        props.compact ? (
          <div key={metric.key} className='min-w-0' title={metric.description}>
            <p
              className='text-muted-foreground truncate text-[10px]'
              title={metric.title}
            >
              {metric.title}
            </p>
            {props.loading ? (
              <Skeleton className='mt-1 h-4 w-12' />
            ) : (
              <p
                className='truncate text-sm font-medium tabular-nums'
                title={`${metric.value}${metric.suffix ?? ''}`}
              >
                {metric.value}
                {metric.suffix}
              </p>
            )}
          </div>
        ) : (
          <Card key={metric.key} size='sm'>
            <CardContent>
              <p className='text-muted-foreground text-xs'>{metric.title}</p>
              {props.loading ? (
                <Skeleton className='my-2 h-7 w-20' />
              ) : (
                <p
                  className={cn(
                    'mt-2 font-medium break-all tabular-nums',
                    metric.key === 'primary' ? 'text-lg' : 'text-2xl'
                  )}
                >
                  {metric.value}
                  {metric.suffix && (
                    <span className='text-muted-foreground text-sm'>
                      {metric.suffix}
                    </span>
                  )}
                </p>
              )}
              <p className='text-muted-foreground text-xs tabular-nums'>
                {metric.description}
              </p>
            </CardContent>
          </Card>
        )
      )}
    </section>
  )
}
