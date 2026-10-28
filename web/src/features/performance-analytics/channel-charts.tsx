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
import { useContext, useMemo } from 'react'
import { useTranslation } from 'react-i18next'

import { Card, CardContent } from '@/components/ui/card'
import { toIntlLocale } from '@/i18n/languages'
import { formatNumber } from '@/lib/format'

import { PerformanceChartContext } from './chart-context'
import { PerformanceChartHeader } from './chart-header'
import {
  PERFORMANCE_CURVE_LABELS,
  buildLatencyChartSeries,
  formatPerformanceLatency,
} from './lib'
import { PerformanceChart } from './performance-chart'
import type {
  ChannelAnalyticsData,
  ChannelAnalyticsPoint,
  ChannelAnalyticsPercentiles,
  PerformanceMetricChartDatum,
} from './types'

export type TransportHistoryPoint = {
  ts: number
  connections: number
  activeRequests: number
}

export function ChannelLatencyChart(props: {
  title: string
  metric: keyof ChannelAnalyticsPoint['latency']
  points: ChannelAnalyticsPoint[]
  summary: ChannelAnalyticsPercentiles
  formatTime: (ts: number) => string
}) {
  const { i18n } = useTranslation()
  const locale = toIntlLocale(i18n.resolvedLanguage || i18n.language)
  const values = useMemo(
    () =>
      buildLatencyChartSeries(
        props.points.map((point) => ({
          ts: point.ts,
          latency: point.latency[props.metric],
        })),
        props.formatTime
      ),
    [props.points, props.metric, props.formatTime]
  )
  const statistics = PERFORMANCE_CURVE_LABELS.map((label) => {
    const field = `${label.toLowerCase()}_ms` as
      | 'avg_ms'
      | 'p50_ms'
      | 'p90_ms'
      | 'p95_ms'
      | 'p99_ms'
      | 'max_ms'
    const value = props.summary[field]
    return {
      label,
      value:
        props.summary.sample_count > 0 && typeof value === 'number'
          ? value
          : undefined,
    }
  })
  return (
    <ChannelTrendChart
      title={props.title}
      values={values}
      timestamps={props.points.map((point) => point.ts)}
      formatTime={props.formatTime}
      formatValue={(value) => formatPerformanceLatency(value, locale)}
      statistics={statistics}
    />
  )
}

export function ChannelActivityCharts(props: {
  data: ChannelAnalyticsData
  history: TransportHistoryPoint[]
  formatTime: (ts: number) => string
}) {
  const { t, i18n } = useTranslation()
  const locale = toIntlLocale(i18n.resolvedLanguage || i18n.language)
  const context = useContext(PerformanceChartContext)
  const number = (value: number) => formatNumber(value, locale)
  const points = props.data.series
  const historyStart = props.history[0]?.ts ?? context.end
  const historyEnd = props.history.at(-1)?.ts ?? context.end
  return (
    <div
      role='group'
      aria-label={t('Concurrency and live transport')}
      className='grid gap-3 md:grid-cols-2 xl:grid-cols-3'
    >
      <ChannelTrendChart
        title={t('Active request concurrency')}
        values={points.flatMap((point) => [
          {
            ts: point.ts,
            time: point.ts,
            label: props.formatTime(point.ts),
            series: t('Average'),
            value: point.active_concurrency.average,
          },
          {
            ts: point.ts,
            time: point.ts,
            label: props.formatTime(point.ts),
            series: t('Peak'),
            value: point.active_concurrency.maximum,
          },
        ])}
        timestamps={points.map((point) => point.ts)}
        formatTime={props.formatTime}
        formatValue={number}
        statistics={[
          {
            label: 'AVG',
            value: props.data.summary.active_concurrency.average,
          },
          {
            label: 'MAX',
            value: props.data.summary.active_concurrency.maximum,
          },
        ]}
      />
      <ChannelTrendChart
        title={t('Errors over time')}
        description={t('Error count by completed request bucket.')}
        values={points.map((point) => ({
          ts: point.ts,
          time: point.ts,
          label: props.formatTime(point.ts),
          value: point.error_count,
        }))}
        timestamps={points.map((point) => point.ts)}
        formatTime={props.formatTime}
        formatValue={number}
      />
      <PerformanceChartContext
        value={{
          ...context,
          group: `${context.group}-transport-session`,
          start: historyStart - 15,
          end: Math.max(historyEnd, historyStart + 15),
        }}
      >
        <ChannelTrendChart
          title={t('Live transport curve')}
          description={t('Browser-session samples')}
          values={props.history.flatMap((point) => [
            {
              ts: point.ts,
              time: point.ts,
              label: props.formatTime(point.ts),
              series: t('Shared pool connections'),
              value: point.connections,
            },
            {
              ts: point.ts,
              time: point.ts,
              label: props.formatTime(point.ts),
              series: t('Active requests'),
              value: point.activeRequests,
            },
          ])}
          timestamps={props.history.map((point) => point.ts)}
          formatTime={props.formatTime}
          formatValue={number}
        />
      </PerformanceChartContext>
    </div>
  )
}

function ChannelTrendChart(props: {
  title: string
  description?: string
  values: PerformanceMetricChartDatum[]
  timestamps: number[]
  formatTime: (ts: number) => string
  formatValue: (value: number) => string
  statistics?: Array<{ label: string; value?: number }>
}) {
  const { t } = useTranslation()
  return (
    <Card>
      <PerformanceChartHeader
        title={props.title}
        description={props.description}
        formatValue={props.formatValue}
        statistics={props.statistics}
      />
      <CardContent>
        {props.values.length ? (
          <PerformanceChart {...props} />
        ) : (
          <div className='text-muted-foreground flex h-56 items-center justify-center text-sm'>
            {t('No performance data available')}
          </div>
        )}
      </CardContent>
    </Card>
  )
}
