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
import { CartesianGrid, Line, LineChart, XAxis, YAxis } from 'recharts'

import {
  StaticDataTable,
  type StaticDataTableColumn,
} from '@/components/data-table'
import { EmptyState } from '@/components/empty-state'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import {
  ChartContainer,
  ChartTooltip,
  ChartTooltipContent,
  type ChartConfig,
} from '@/components/ui/chart'
import { toIntlLocale } from '@/i18n/languages'
import { formatNumber } from '@/lib/format'
import { cn } from '@/lib/utils'

import type { MonitorChannel, MonitorEvent, MonitorRoute } from './api'
import { ChannelStatus } from './channel-status'
import { channelColor } from './route-data'

export function ChannelDetails(props: {
  route: MonitorRoute
  at: number
  names?: Map<number, string>
}) {
  const { t, i18n } = useTranslation()
  const locale = toIntlLocale(i18n.resolvedLanguage || i18n.language)
  const priority = props.route.mode === 'priority'
  const columns: StaticDataTableColumn<MonitorChannel>[] = [
    {
      id: 'name',
      header: t('Channel'),
      cell: (channel) =>
        props.names?.get(channel.channel_id) || `#${channel.channel_id}`,
    },
    {
      id: 'priority',
      header: t('Priority'),
      cell: (channel) => formatNumber(channel.priority, locale),
    },
    {
      id: 'weight',
      header: t('Weight'),
      cell: (channel) => formatNumber(channel.weight, locale),
    },
    {
      id: 'discount',
      header: t('Effective discount'),
      cell: (channel) =>
        channel.effective_cost_discount == null
          ? '—'
          : `×${formatNumber(channel.effective_cost_discount, locale)}`,
    },
    {
      id: 'target',
      header: t('Target share'),
      className: 'text-right',
      cell: (channel) =>
        props.route.aggregate
          ? '—'
          : `${formatNumber(channel.target_share * 100, locale)}%`,
    },
    {
      id: 'actual',
      header: t('Selection share'),
      className: 'text-right',
      cell: (channel) =>
        `${formatNumber(channel.selected_share * 100, locale)}%`,
    },
    {
      id: 'count',
      header: t('Routing selections'),
      className: 'text-right',
      cell: (channel) => formatNumber(channel.selection_count, locale),
    },
    {
      id: 'ttft',
      header: 'TTFT',
      className: 'text-right',
      cell: (channel) =>
        channel.ttft_ms == null
          ? '—'
          : `${formatNumber(channel.ttft_ms, locale)} ms`,
    },
    {
      id: 'tpot',
      header: 'TPOT',
      className: 'text-right',
      cell: (channel) =>
        channel.tpot_ms == null
          ? '—'
          : `${formatNumber(channel.tpot_ms, locale)} ms`,
    },
    {
      id: 'success',
      header: t('Success rate'),
      cell: (channel) =>
        channel.completed_count
          ? `${formatNumber(((channel.success_count ?? 0) / channel.completed_count) * 100, locale)}%`
          : '—',
    },
    {
      id: 'latency',
      header: t('Average latency'),
      cell: (channel) =>
        channel.latency_ms == null
          ? '—'
          : `${formatNumber(channel.latency_ms, locale)} ms`,
    },
    {
      id: 'state',
      header: t('Status'),
      cell: (channel) => (
        <ChannelStatus channel={channel} at={props.at} priority={priority} />
      ),
    },
  ]
  return (
    <StaticDataTable
      data={props.route.channels}
      getRowKey={(channel) => channel.channel_id}
      tableClassName='min-w-[720px]'
      columns={columns.filter((column) =>
        priority
          ? column.id !== 'target' && column.id !== 'discount'
          : column.id !== 'success' && column.id !== 'latency'
      )}
    />
  )
}

export function ShareHistory(props: {
  route: MonitorRoute
  names?: Map<number, string>
  fill?: boolean
  compact?: boolean
}) {
  const { t, i18n } = useTranslation()
  const locale = toIntlLocale(i18n.resolvedLanguage || i18n.language)
  const config: ChartConfig = {}
  props.route.channels.forEach((channel) => {
    config[`channel${channel.channel_id}`] = {
      label: props.names?.get(channel.channel_id) || `#${channel.channel_id}`,
      color: channelColor(channel.channel_id),
    }
  })
  const rows = props.route.history.map((bucket) => {
    const point: Record<string, number | null> = { at: bucket.at }
    props.route.channels.forEach((channel) => {
      const item = bucket.channels.find(
        (entry) => entry.channel_id === channel.channel_id
      )
      point[`channel${channel.channel_id}`] =
        bucket.selection_count > 0 ? (item?.selected_share ?? 0) * 100 : null
    })
    return point
  })
  const time = (value: number) =>
    new Date(value).toLocaleTimeString(locale, {
      hour: '2-digit',
      minute: '2-digit',
      second: '2-digit',
    })
  return (
    <Card
      role='region'
      aria-label={t('Traffic share history')}
      data-routing-history-panel=''
      className={cn(
        props.fill && 'xl:h-full xl:min-h-0',
        props.compact &&
          'h-full min-h-0 gap-2 rounded-none bg-transparent py-0 ring-0'
      )}
    >
      {!props.compact && (
        <CardHeader className='shrink-0'>
          <CardTitle>{t('Traffic share history')}</CardTitle>
          <p className='text-muted-foreground text-xs'>
            {t('Selection share over the last {{seconds}} seconds', {
              seconds: props.route.window_seconds,
            })}
          </p>
        </CardHeader>
      )}
      <CardContent
        className={cn(
          props.fill && 'xl:flex xl:min-h-0 xl:flex-1 xl:flex-col',
          props.compact && 'flex min-h-0 flex-1 flex-col px-3'
        )}
      >
        <div
          className={cn(
            'mb-3 flex flex-wrap gap-4 text-xs',
            props.fill && 'xl:max-h-12 xl:shrink-0 xl:overflow-y-auto',
            props.compact &&
              'mb-1 h-4 shrink-0 flex-nowrap gap-2 overflow-hidden'
          )}
        >
          {props.route.channels.map((channel) => (
            <span
              key={channel.channel_id}
              title={
                props.names?.get(channel.channel_id) || `#${channel.channel_id}`
              }
              className={cn(
                'flex min-w-0 items-center gap-1.5',
                props.compact && 'flex-1'
              )}
            >
              <i
                className='size-2 shrink-0 rounded-full'
                style={{ background: channelColor(channel.channel_id) }}
              />
              <span
                className={props.compact ? 'truncate text-[10px]' : 'break-all'}
              >
                {props.names?.get(channel.channel_id) ||
                  `#${channel.channel_id}`}
              </span>
            </span>
          ))}
        </div>
        {props.compact && rows.length === 0 ? (
          <EmptyState title={t('No Data')} className='min-h-0 p-3' />
        ) : (
          <ChartContainer
            className={cn(
              'h-48 w-full',
              props.fill && 'xl:aspect-auto xl:h-auto xl:min-h-0 xl:flex-1',
              props.compact && 'aspect-auto h-full min-h-0 flex-1'
            )}
            config={config}
            aria-label={t('Channel selection share history')}
          >
            <LineChart
              data={rows}
              margin={
                props.compact
                  ? { top: 4, right: 2, left: 2, bottom: 4 }
                  : { top: 10, right: 10, left: -15, bottom: 0 }
              }
              accessibilityLayer
            >
              <CartesianGrid vertical={false} />
              <XAxis
                hide={props.compact}
                dataKey='at'
                type='number'
                domain={['dataMin', 'dataMax']}
                tickFormatter={time}
                minTickGap={55}
              />
              <YAxis
                hide={props.compact}
                domain={[0, 100]}
                tickCount={3}
                tickFormatter={(value) => `${value}%`}
                width={50}
              />
              <ChartTooltip
                content={
                  <ChartTooltipContent
                    labelFormatter={(value) => time(Number(value))}
                    formatter={(value, name) => (
                      <span>
                        {config[String(name)]?.label}:{' '}
                        {formatNumber(Number(value), locale)}%
                      </span>
                    )}
                  />
                }
              />
              {props.route.channels.map((channel) => (
                <Line
                  key={channel.channel_id}
                  dataKey={`channel${channel.channel_id}`}
                  type='linear'
                  stroke={`var(--color-channel${channel.channel_id})`}
                  strokeWidth={2}
                  dot={false}
                  connectNulls={false}
                  isAnimationActive={false}
                />
              ))}
            </LineChart>
          </ChartContainer>
        )}
      </CardContent>
    </Card>
  )
}

function EventLabel(props: { event: MonitorEvent }) {
  const { t } = useTranslation()
  if (props.event.kind === 'channel_disabled') {
    return <>{t('Channel disabled by existing rules')}</>
  }
  if (props.event.kind === 'channel_enabled') {
    return <>{t('Channel restored by health checks')}</>
  }
  if (props.event.kind === 'initialized') {
    return <>{t('Routing allocation initialized')}</>
  }
  if (props.event.kind === 'candidates_changed') {
    return <>{t('Candidate channels changed')}</>
  }
  if (props.event.kind === 'failure') {
    return <>{t('Channel failure')}</>
  }
  if (props.event.kind === 'degraded') {
    return <>{t('Degraded')}</>
  }
  if (props.event.kind === 'recovery') {
    return <>{t('Channel recovery changed allocation')}</>
  }
  return <>{t('Target allocation changed')}</>
}

export function RoutingEvents(props: {
  route: MonitorRoute
  names?: Map<number, string>
  fill?: boolean
  compact?: boolean
}) {
  const { t, i18n } = useTranslation()
  const locale = toIntlLocale(i18n.resolvedLanguage || i18n.language)
  return (
    <Card
      role='region'
      aria-label={t('Routing changes')}
      data-routing-events-panel=''
      className={cn(
        props.fill && 'xl:h-full xl:min-h-0',
        props.compact &&
          'h-full min-h-0 gap-2 rounded-none bg-transparent py-0 ring-0'
      )}
    >
      {!props.compact && (
        <CardHeader className='shrink-0'>
          <CardTitle>{t('Routing changes')}</CardTitle>
        </CardHeader>
      )}
      <CardContent
        data-routing-event-scroll=''
        className={cn(
          props.fill &&
            'xl:min-h-0 xl:flex-1 xl:overflow-y-auto xl:overscroll-contain',
          props.compact &&
            'min-h-0 flex-1 overflow-y-auto overscroll-contain px-3'
        )}
      >
        {props.route.events.length === 0 && (
          <p className='text-muted-foreground py-4 text-sm'>
            {t('No allocation changes in this observation window')}
          </p>
        )}
        <ol className='divide-y' aria-label={t('Routing changes')}>
          {props.route.events.map((event) => (
            <li
              key={`${event.node_id ?? ''}-${event.at}-${event.kind}-${event.allocations.map((allocation) => `${allocation.channel_id}:${allocation.share}`).join(',')}`}
              className='py-3 first:pt-0 last:pb-0'
            >
              <div className='flex items-start justify-between gap-3'>
                <p className='text-sm'>
                  <EventLabel event={event} />
                  {event.node_name && (
                    <span className='text-muted-foreground ml-2 text-xs'>
                      {event.node_name}
                    </span>
                  )}
                </p>
                <time className='text-muted-foreground shrink-0 text-xs tabular-nums'>
                  {new Date(event.at).toLocaleTimeString(locale, {
                    hour: '2-digit',
                    minute: '2-digit',
                    second: '2-digit',
                  })}
                </time>
              </div>
              <p className='text-muted-foreground mt-1 text-xs break-words'>
                {event.channel_ids
                  .map((id) => props.names?.get(id) || `#${id}`)
                  .join(' · ')}{' '}
                {event.allocations
                  .map(
                    (allocation) =>
                      `${props.names?.get(allocation.channel_id) || `#${allocation.channel_id}`} ${formatNumber(allocation.share * 100, locale)}%`
                  )
                  .join(' · ')}
              </p>
            </li>
          ))}
        </ol>
      </CardContent>
    </Card>
  )
}
