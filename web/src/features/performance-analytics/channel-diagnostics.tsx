import { keepPreviousData, useQuery } from '@tanstack/react-query'
import { VChart } from '@visactor/react-vchart'
import { Activity, RefreshCw } from 'lucide-react'
import {
  type ReactNode,
  useCallback,
  useEffect,
  useMemo,
  useState,
} from 'react'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { Tabs, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { formatLatency } from '@/features/performance-metrics/lib/format'
import { useChartTheme } from '@/lib/use-chart-theme'
import { VCHART_OPTION } from '@/lib/vchart'

import { getChannelPerformanceAnalytics, getHTTPTransportMetrics } from './api'
import { buildPerformanceTimeAxis, formatPerformanceTimestamp } from './lib'
import { TransportComparison } from './transport-comparison'
import type {
  ChannelAnalyticsData,
  ChannelAnalyticsPoint,
  HTTPTransportMetric,
} from './types'

const CHANNEL_RANGE_OPTIONS = [
  { seconds: 10 * 60, label: '10 Minutes', bucket: 10 },
  { seconds: 60 * 60, label: '1 Hour', bucket: 60 },
  { seconds: 6 * 60 * 60, label: '6 Hours', bucket: 300 },
  { seconds: 24 * 60 * 60, label: '1 Day', bucket: 300 },
] as const

type ChannelRangeOption = (typeof CHANNEL_RANGE_OPTIONS)[number]

export function ChannelPerformanceDiagnostics({
  formatTime,
}: {
  formatTime?: (ts: number) => string
}) {
  const { t, i18n } = useTranslation()
  const [channelInput, setChannelInput] = useState('')
  const [channelId, setChannelId] = useState<number>()
  const [range, setRange] = useState<ChannelRangeOption>(
    CHANNEL_RANGE_OPTIONS[0]
  )
  const [rangeEnd, setRangeEnd] = useState(() => Math.floor(Date.now() / 1000))
  const queryParams = useMemo(
    () => ({
      ...(channelId ? { channel_id: channelId } : {}),
      start_timestamp: rangeEnd - range.seconds,
      end_timestamp: rangeEnd,
      bucket_seconds: range.bucket,
    }),
    [channelId, range, rangeEnd]
  )
  const query = useQuery({
    queryKey: ['performance-analytics', 'channel', queryParams],
    queryFn: () => getChannelPerformanceAnalytics(queryParams),
    select: (response) => response.data,
    placeholderData: keepPreviousData,
    staleTime: 15_000,
    retry: false,
  })
  const transportQuery = useQuery({
    queryKey: ['performance-analytics', 'transport'],
    queryFn: getHTTPTransportMetrics,
    select: (response) => response.data,
    refetchInterval: 15_000,
    staleTime: 5_000,
    retry: false,
  })
  const [transportHistory, setTransportHistory] = useState<
    Array<{ ts: number; connections: number; activeRequests: number }>
  >([])
  useEffect(() => {
    const metrics = transportQuery.data
    if (!metrics) return
    const activeRequests = metrics.reduce((total, metric) => {
      if (!channelId) return total + metric.active_requests
      return (
        total +
        Object.entries(metric.active_by_channel ?? {}).reduce(
          (channelTotal, [id, active]) =>
            channelTotal + (Number(id) === channelId ? active : 0),
          0
        )
      )
    }, 0)
    setTransportHistory((previous) =>
      [
        ...previous,
        {
          ts: Math.floor(Date.now() / 1000),
          connections: metrics.reduce(
            (total, metric) => total + metric.connections,
            0
          ),
          activeRequests,
        },
      ].slice(-40)
    )
  }, [channelId, transportQuery.data, transportQuery.dataUpdatedAt])
  const data = query.data
  const time = useCallback(
    (ts: number) =>
      formatTime?.(ts) ??
      formatPerformanceTimestamp(ts, i18n.resolvedLanguage || i18n.language),
    [formatTime, i18n.language, i18n.resolvedLanguage]
  )

  const submitChannel = useCallback(() => {
    const parsed = Number.parseInt(channelInput.trim(), 10)
    setChannelId(Number.isInteger(parsed) && parsed > 0 ? parsed : undefined)
    setTransportHistory([])
    setRangeEnd(Math.floor(Date.now() / 1000))
  }, [channelInput])

  const changeRange = useCallback((value: string) => {
    const next = CHANNEL_RANGE_OPTIONS.find(
      (option) => String(option.seconds) === value
    )
    if (next) {
      setRange(next)
      setRangeEnd(Math.floor(Date.now() / 1000))
    }
  }, [])

  let diagnosticsContent: ReactNode
  if (query.isError) {
    diagnosticsContent = (
      <Card>
        <CardContent className='text-muted-foreground flex min-h-32 items-center justify-center text-sm'>
          <Activity className='mr-2 size-4' />
          {t('Failed to load performance data')}
        </CardContent>
      </Card>
    )
  } else if (data) {
    diagnosticsContent = (
      <ChannelDiagnosticsContent
        data={data}
        formatTime={time}
        transport={transportQuery.data ?? []}
        transportHistory={transportHistory}
      />
    )
  } else {
    diagnosticsContent = (
      <Card>
        <CardContent className='text-muted-foreground flex min-h-32 items-center justify-center text-sm'>
          {t('No performance data available')}
        </CardContent>
      </Card>
    )
  }

  return (
    <div className='mt-4 space-y-3'>
      <Card size='sm'>
        <CardHeader>
          <CardTitle>{t('Channel transport diagnostics')}</CardTitle>
          <CardDescription>
            {t(
              'Shows active request concurrency, request-stage latency curves, and p50/p95/p99 statistics from admin timing data.'
            )}
          </CardDescription>
        </CardHeader>
        <CardContent className='flex flex-wrap items-end gap-2'>
          <label className='w-36 space-y-1'>
            <span className='text-muted-foreground block text-xs font-medium'>
              {t('Channel ID')}
            </span>
            <Input
              inputMode='numeric'
              placeholder={t('All channels')}
              value={channelInput}
              onChange={(event) => setChannelInput(event.target.value)}
              onKeyDown={(event) => {
                if (event.key === 'Enter') submitChannel()
              }}
            />
          </label>
          <div className='space-y-1'>
            <span className='text-muted-foreground block text-xs font-medium'>
              {t('Time range')}
            </span>
            <Tabs value={String(range.seconds)} onValueChange={changeRange}>
              <TabsList>
                {CHANNEL_RANGE_OPTIONS.map((option) => (
                  <TabsTrigger
                    key={option.seconds}
                    value={String(option.seconds)}
                    className='px-2.5 text-xs'
                  >
                    {t(option.label)}
                  </TabsTrigger>
                ))}
              </TabsList>
            </Tabs>
          </div>
          <Button
            variant='outline'
            size='sm'
            onClick={() => {
              setRangeEnd(Math.floor(Date.now() / 1000))
            }}
            disabled={query.isFetching}
          >
            <RefreshCw
              className={query.isFetching ? 'animate-spin' : undefined}
            />
            {t('Refresh')}
          </Button>
        </CardContent>
      </Card>

      {diagnosticsContent}
    </div>
  )
}

function ChannelDiagnosticsContent({
  data,
  formatTime,
  transport,
  transportHistory,
}: {
  data: ChannelAnalyticsData
  formatTime: (ts: number) => string
  transport: HTTPTransportMetric[]
  transportHistory: Array<{
    ts: number
    connections: number
    activeRequests: number
  }>
}) {
  const { t } = useTranslation()
  const summary = data.summary
  const cards = [
    [t('Requests'), summary.request_count.toLocaleString()],
    [t('Average concurrency'), summary.active_concurrency.average.toFixed(2)],
    [
      t('Peak concurrency'),
      summary.active_concurrency.maximum.toLocaleString(),
    ],
    [t('Total p50'), formatLatency(summary.latency.total_ms.p50_ms)],
    [t('Total p95'), formatLatency(summary.latency.total_ms.p95_ms)],
    [t('Total p99'), formatLatency(summary.latency.total_ms.p99_ms)],
  ]

  return (
    <>
      <p className='text-muted-foreground text-xs'>
        {t(
          'Historical concurrency is reconstructed from completed log records; unfinished requests and truncated scans are not included. Percentiles are approximate histogram upper bounds.'
        )}
      </p>
      {data.truncated && (
        <p role='alert' className='text-destructive text-sm'>
          {t(
            'Scan limit reached. These are partial statistics; select a shorter range or one channel.'
          )}
        </p>
      )}
      <div className='grid grid-cols-2 gap-2 sm:grid-cols-3 lg:grid-cols-6'>
        {cards.map(([label, value]) => (
          <Card key={label} size='sm'>
            <CardContent>
              <div className='text-muted-foreground text-xs font-medium'>
                {label}
              </div>
              <div className='mt-1 text-xl font-semibold tabular-nums'>
                {value}
              </div>
            </CardContent>
          </Card>
        ))}
      </div>
      <TransportPoolTable metrics={transport} />
      <TransportHistoryChart
        points={transportHistory}
        formatTime={formatTime}
      />
      <TransportComparison groups={data.transport_groups ?? []} />
      <div className='grid gap-3 xl:grid-cols-2'>
        <ChannelConcurrencyChart points={data.series} formatTime={formatTime} />
        <ChannelLatencyChart
          title={t('Connection acquisition and write time')}
          points={data.series}
          metric='acquire_ms'
          secondaryMetric='write_ms'
          formatTime={formatTime}
        />
        <ChannelLatencyChart
          title={t('Total request latency')}
          points={data.series}
          metric='total_ms'
          formatTime={formatTime}
        />
        <ChannelLatencyChart
          title={t('Upload and provider wait')}
          points={data.series}
          metric='upload_ms'
          secondaryMetric='provider_wait_ms'
          formatTime={formatTime}
        />
      </div>
    </>
  )
}

function TransportHistoryChart({
  points,
  formatTime,
}: {
  points: Array<{ ts: number; connections: number; activeRequests: number }>
  formatTime: (ts: number) => string
}) {
  const { t } = useTranslation()
  const chart = useChannelChartTheme()
  const values = points.flatMap((point) => [
    {
      time: point.ts,
      label: formatTime(point.ts),
      metric: t('Shared pool connections'),
      value: point.connections,
    },
    {
      time: point.ts,
      label: formatTime(point.ts),
      metric: t('Active requests'),
      value: point.activeRequests,
    },
  ])
  if (points.length < 2) return null
  return (
    <Card>
      <CardHeader>
        <CardTitle>{t('Live transport curve')}</CardTitle>
        <CardDescription>
          {t(
            'Live curves are browser-session samples. Connections cover all shared pools; active requests follow the selected channel and include streaming response lifetime.'
          )}
        </CardDescription>
      </CardHeader>
      <CardContent className='h-72'>
        <VChart
          key={`transport-history-${chart.resolvedTheme}`}
          spec={lineSpec(values, chart, formatTime, true)}
          option={VCHART_OPTION}
        />
      </CardContent>
    </Card>
  )
}

function TransportPoolTable({ metrics }: { metrics: HTTPTransportMetric[] }) {
  const { t } = useTranslation()
  if (metrics.length === 0) return null
  return (
    <Card>
      <CardHeader>
        <CardTitle>{t('Live HTTP transport pools')}</CardTitle>
        <CardDescription>
          {t(
            'Connections are reported at shared pool/shard scope; active requests are attributed to channels.'
          )}
        </CardDescription>
      </CardHeader>
      <CardContent className='overflow-x-auto'>
        <table className='w-full min-w-[640px] text-sm'>
          <thead className='text-muted-foreground text-left text-xs'>
            <tr>
              <th className='pb-2 pr-4'>{t('Pool')}</th>
              <th className='pb-2 pr-4'>{t('Protocol')}</th>
              <th className='pb-2 pr-4'>{t('Shard')}</th>
              <th className='pb-2 pr-4'>{t('Connections')}</th>
              <th className='pb-2 pr-4'>{t('Active requests')}</th>
              <th className='pb-2'>{t('Last negotiated')}</th>
            </tr>
          </thead>
          <tbody>
            {metrics.map((metric) => (
              <tr key={metric.pool_id} className='border-t'>
                <td className='py-2 pr-4 font-mono'>#{metric.pool_id}</td>
                <td className='py-2 pr-4'>{metric.protocol}</td>
                <td className='py-2 pr-4'>
                  {metric.shard + 1} / {metric.shards}
                </td>
                <td className='py-2 pr-4 tabular-nums'>{metric.connections}</td>
                <td className='py-2 pr-4 tabular-nums'>
                  {metric.active_requests}
                </td>
                <td className='py-2'>{metric.last_protocol || '—'}</td>
              </tr>
            ))}
          </tbody>
        </table>
      </CardContent>
    </Card>
  )
}

function ChannelConcurrencyChart({
  points,
  formatTime,
}: {
  points: ChannelAnalyticsPoint[]
  formatTime: (ts: number) => string
}) {
  const { t } = useTranslation()
  const chart = useChannelChartTheme()
  const values = points.flatMap((point) => [
    {
      time: point.ts,
      label: formatTime(point.ts),
      metric: t('Average'),
      value: point.active_concurrency.average,
    },
    {
      time: point.ts,
      label: formatTime(point.ts),
      metric: t('Peak'),
      value: point.active_concurrency.maximum,
    },
  ])
  return (
    <Card>
      <CardHeader>
        <CardTitle>{t('Active request concurrency')}</CardTitle>
        <CardDescription>
          {t(
            'Historical concurrency is reconstructed from completed log records; unfinished requests and truncated scans are not included. Percentiles are approximate histogram upper bounds.'
          )}
        </CardDescription>
      </CardHeader>
      <CardContent className='h-80'>
        <VChart
          key={`channel-concurrency-${chart.resolvedTheme}`}
          spec={lineSpec(values, chart, formatTime, true)}
          option={VCHART_OPTION}
        />
      </CardContent>
    </Card>
  )
}

function ChannelLatencyChart({
  title,
  points,
  metric,
  secondaryMetric,
  formatTime,
}: {
  title: string
  points: ChannelAnalyticsPoint[]
  metric: keyof ChannelAnalyticsPoint['latency']
  secondaryMetric?: keyof ChannelAnalyticsPoint['latency']
  formatTime: (ts: number) => string
}) {
  const chart = useChannelChartTheme()
  const fields = [metric, secondaryMetric].filter(
    (field): field is keyof ChannelAnalyticsPoint['latency'] => Boolean(field)
  )
  const values = points.flatMap((point) =>
    fields.flatMap((field) => {
      const percentiles = point.latency[field]
      if (!percentiles || percentiles.sample_count === 0) return []
      return (['p50_ms', 'p95_ms', 'p99_ms'] as const).map((percentile) => ({
        time: point.ts,
        label: formatTime(point.ts),
        metric: `${field}-${percentile.replace('_ms', '').toUpperCase()}`,
        value: percentiles[percentile],
      }))
    })
  )
  return (
    <Card>
      <CardHeader>
        <CardTitle>{title}</CardTitle>
        <CardDescription>p50 / p95 / p99</CardDescription>
      </CardHeader>
      <CardContent className='h-80'>
        <VChart
          key={`${String(metric)}-${String(secondaryMetric)}-${chart.resolvedTheme}`}
          spec={lineSpec(values, chart, formatTime, false)}
          option={VCHART_OPTION}
        />
      </CardContent>
    </Card>
  )
}

function useChannelChartTheme() {
  const { resolvedTheme } = useChartTheme()
  const chartTextColor =
    resolvedTheme === 'dark'
      ? 'rgba(255, 255, 255, 0.68)'
      : 'rgba(15, 23, 42, 0.58)'
  const chartGridColor =
    resolvedTheme === 'dark'
      ? 'rgba(255, 255, 255, 0.12)'
      : 'rgba(15, 23, 42, 0.12)'
  return { resolvedTheme, chartTextColor, chartGridColor }
}

function lineSpec(
  values: Array<Record<string, string | number>>,
  chart: ReturnType<typeof useChannelChartTheme>,
  formatTime: (ts: number) => string,
  integer: boolean
) {
  return {
    type: 'line' as const,
    data: [{ id: 'channel-diagnostics', values }],
    xField: 'time',
    yField: 'value',
    seriesField: 'metric',
    point: { visible: values.length <= 60 },
    legends: { visible: true, orient: 'top', position: 'end' },
    axes: [
      buildPerformanceTimeAxis(formatTime, chart.chartTextColor),
      {
        orient: 'left',
        label: {
          formatMethod: (value: number | string) =>
            integer
              ? String(Math.round(Number(value)))
              : formatLatency(Number(value)),
          style: { fill: chart.chartTextColor, fontSize: 10 },
        },
        grid: {
          visible: true,
          style: { lineDash: [3, 3], stroke: chart.chartGridColor },
        },
      },
    ],
    tooltip: {
      dimension: {
        title: {
          value: (datum: Record<string, unknown>) => String(datum?.label ?? ''),
        },
        content: [
          {
            key: (datum: Record<string, unknown>) =>
              String(datum?.metric ?? ''),
            value: (datum: Record<string, unknown>) =>
              integer
                ? String(Math.round(Number(datum?.value)))
                : formatLatency(Number(datum?.value)),
          },
        ],
      },
    },
    animationAppear: { duration: 300 },
    theme: chart.resolvedTheme === 'dark' ? 'dark' : 'light',
    background: 'transparent',
  }
}
