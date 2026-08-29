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
import { useQuery } from '@tanstack/react-query'
import { VChart } from '@visactor/react-vchart'
import { Activity, ChartNoAxesCombined, Clock3, RefreshCw } from 'lucide-react'
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
import {
  Empty,
  EmptyDescription,
  EmptyHeader,
  EmptyMedia,
  EmptyTitle,
} from '@/components/ui/empty'
import {
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { Skeleton } from '@/components/ui/skeleton'
import { Tabs, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { buildPerformanceAnalyticsParams } from '@/features/dashboard/lib/filters'
import {
  formatLatency,
  formatUptimePct,
} from '@/features/performance-metrics/lib/format'
import { useChartTheme } from '@/lib/use-chart-theme'
import { VCHART_OPTION } from '@/lib/vchart'

import { getPerformanceAnalytics, getPerformanceAnalyticsOptions } from './api'
import {
  buildPerformanceMetricSeries,
  buildPerformanceSeries,
  buildPerformanceTimeAxis,
  formatPerformanceTimestamp,
  getPerformanceFilterLabel,
} from './lib'
import type {
  PerformanceAnalyticsPoint,
  PerformanceAnalyticsSummary,
  PerformanceMetric,
  PerformancePercentiles,
} from './types'

const ALL_VALUE = '__all__'
const RANGE_OPTIONS = [
  { days: 1, label: '1 Day' },
  { days: 7, label: '7 Days' },
  { days: 29, label: '29 Days' },
] as const

type PerformanceAnalyticsProps = {
  isAdmin: boolean
}

export function PerformanceAnalytics({ isAdmin }: PerformanceAnalyticsProps) {
  const { t, i18n } = useTranslation()
  const [modelName, setModelName] = useState('')
  const [userId, setUserId] = useState<number>()
  const [tokenId, setTokenId] = useState<number>()
  const [rangeDays, setRangeDays] = useState(1)
  const [rangeEnd, setRangeEnd] = useState(() => Math.floor(Date.now() / 1000))

  const optionsQuery = useQuery({
    queryKey: [
      'performance-analytics',
      'options',
      isAdmin ? 'admin' : 'self',
      isAdmin ? userId : undefined,
    ],
    queryFn: () => getPerformanceAnalyticsOptions(isAdmin, userId),
    select: (response) => response.data,
    staleTime: 60_000,
    retry: false,
  })

  const options = optionsQuery.data
  const models = useMemo(() => options?.models ?? [], [options?.models])
  const users = useMemo(() => options?.users ?? [], [options?.users])
  const tokens = useMemo(() => options?.tokens ?? [], [options?.tokens])

  useEffect(() => {
    if (models.length === 0) {
      setModelName('')
      return
    }
    if (!models.includes(modelName)) {
      setModelName(models[0])
    }
  }, [modelName, models])

  useEffect(() => {
    if (tokenId && !tokens.some((token) => token.id === tokenId)) {
      setTokenId(undefined)
    }
  }, [tokenId, tokens])

  const queryParams = useMemo(
    () =>
      buildPerformanceAnalyticsParams(
        {
          model: modelName,
          startTimestamp: rangeEnd - rangeDays * 24 * 60 * 60,
          endTimestamp: rangeEnd,
          userId,
          tokenId,
        },
        isAdmin
      ),
    [isAdmin, modelName, rangeDays, rangeEnd, tokenId, userId]
  )

  const analyticsQuery = useQuery({
    queryKey: [
      'performance-analytics',
      'series',
      isAdmin ? 'admin' : 'self',
      queryParams,
    ],
    queryFn: () => getPerformanceAnalytics(queryParams, isAdmin),
    select: (response) => response.data,
    enabled: modelName.length > 0,
    staleTime: 30_000,
    retry: false,
  })

  const handleRangeChange = useCallback((days: number) => {
    setRangeDays(days)
    setRangeEnd(Math.floor(Date.now() / 1000))
  }, [])

  const handleRefresh = useCallback(() => {
    setRangeEnd(Math.floor(Date.now() / 1000))
    void optionsQuery.refetch()
    if (modelName) {
      void analyticsQuery.refetch()
    }
  }, [analyticsQuery, modelName, optionsQuery])

  const handleUserChange = useCallback((value: string | null) => {
    setUserId(value && value !== ALL_VALUE ? Number(value) : undefined)
    setTokenId(undefined)
  }, [])

  const formatTime = useCallback(
    (ts: number) =>
      formatPerformanceTimestamp(ts, i18n.resolvedLanguage || i18n.language),
    [i18n.language, i18n.resolvedLanguage]
  )

  const data = analyticsQuery.data
  const loading = optionsQuery.isLoading || analyticsQuery.isLoading
  const fetching = optionsQuery.isFetching || analyticsQuery.isFetching
  const hasData = Boolean(data && data.summary.request_count > 0)
  let analyticsContent: ReactNode = null

  if (loading && !data) {
    analyticsContent = <PerformanceAnalyticsSkeleton />
  } else if (optionsQuery.isError || analyticsQuery.isError) {
    analyticsContent = (
      <Empty className='min-h-80 border'>
        <EmptyHeader>
          <EmptyMedia variant='icon'>
            <Activity />
          </EmptyMedia>
          <EmptyTitle>{t('Failed to load performance data')}</EmptyTitle>
          <EmptyDescription>
            {t('Refresh the page or try again later.')}
          </EmptyDescription>
        </EmptyHeader>
      </Empty>
    )
  } else if (!modelName || !hasData) {
    analyticsContent = (
      <Empty className='min-h-80 border'>
        <EmptyHeader>
          <EmptyMedia variant='icon'>
            <ChartNoAxesCombined />
          </EmptyMedia>
          <EmptyTitle>{t('No performance data available')}</EmptyTitle>
          <EmptyDescription>
            {t('Try another model, user, API key, or time range.')}
          </EmptyDescription>
        </EmptyHeader>
      </Empty>
    )
  } else if (data) {
    analyticsContent = (
      <>
        <div className='text-muted-foreground text-xs'>
          {t('Time range')}: {formatTime(data.effective_start_timestamp)} –{' '}
          {formatTime(data.effective_end_timestamp)}
        </div>
        <SummaryCards summary={data.summary} />
        <div className='grid gap-3 xl:grid-cols-2'>
          <LatencyChart
            title='TTFT'
            description={t('Time to first streamed response')}
            metric='ttft'
            summary={data.summary.ttft}
            points={data.series}
            formatTime={formatTime}
          />
          <LatencyChart
            title='TPOT'
            description={t('Time per output token')}
            metric='tpot'
            summary={data.summary.tpot}
            points={data.series}
            formatTime={formatTime}
          />
        </div>
        <div className='grid gap-3 xl:grid-cols-2'>
          <MetricChart
            title={t('Cache hit rate')}
            description={t('Share of input tokens served from cache')}
            metric='cache_hit_rate'
            points={data.series}
            formatTime={formatTime}
            formatValue={formatUptimePct}
          />
          <MetricChart
            title='RPM'
            description={t('Requests completed per minute')}
            metric='rpm'
            points={data.series}
            formatTime={formatTime}
            formatValue={formatThroughput}
          />
          <MetricChart
            className='xl:col-span-2'
            title='TPM'
            description={t('Input and output tokens processed per minute')}
            metric='tpm'
            points={data.series}
            formatTime={formatTime}
            formatValue={formatThroughput}
          />
        </div>
      </>
    )
  }

  return (
    <div className='space-y-3 sm:space-y-4'>
      <Card size='sm'>
        <CardHeader>
          <CardTitle>{t('Performance percentiles')}</CardTitle>
          <CardDescription>
            <span className='block'>
              {t(
                'Analyze TTFT and TPOT percentiles by model, user, and API key.'
              )}
            </span>
            <span className='block'>
              {t('Percentiles are estimated from latency buckets.')}
            </span>
            <span className='block'>
              {t('Includes cache hit rate, RPM, and TPM throughput.')}
            </span>
            <span className='block'>
              {t(
                'Recent data may lag by one performance metrics flush interval.'
              )}
            </span>
          </CardDescription>
        </CardHeader>
        <CardContent className='flex flex-wrap items-end gap-2'>
          <FilterSelect
            label={t('Model')}
            value={modelName || null}
            placeholder={t('Select a model')}
            disabled={optionsQuery.isLoading || models.length === 0}
            onValueChange={(value) => setModelName(value ?? '')}
            options={models.map((model) => ({ value: model, label: model }))}
          />
          {isAdmin && (
            <FilterSelect
              label={t('User')}
              value={userId ? String(userId) : ALL_VALUE}
              placeholder={t('All users')}
              onValueChange={handleUserChange}
              options={[
                { value: ALL_VALUE, label: t('All users') },
                ...users.map((user) => ({
                  value: String(user.id),
                  label: user.username,
                })),
              ]}
            />
          )}
          <FilterSelect
            label={t('API Key')}
            value={tokenId ? String(tokenId) : ALL_VALUE}
            placeholder={t('All API keys')}
            onValueChange={(value) =>
              setTokenId(
                value && value !== ALL_VALUE ? Number(value) : undefined
              )
            }
            options={[
              { value: ALL_VALUE, label: t('All API keys') },
              ...tokens.map((token) => ({
                value: String(token.id),
                label: token.name
                  ? `${token.name} (#${token.id})`
                  : `#${token.id}`,
              })),
            ]}
          />
          <div className='space-y-1'>
            <span className='text-muted-foreground block text-xs font-medium'>
              {t('Time range')}
            </span>
            <Tabs
              value={String(rangeDays)}
              onValueChange={(value) => handleRangeChange(Number(value))}
            >
              <TabsList>
                {RANGE_OPTIONS.map((option) => (
                  <TabsTrigger
                    key={option.days}
                    value={String(option.days)}
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
            onClick={handleRefresh}
            disabled={fetching}
          >
            <RefreshCw className={fetching ? 'animate-spin' : undefined} />
            {t('Refresh')}
          </Button>
        </CardContent>
      </Card>

      {analyticsContent}
    </div>
  )
}

function FilterSelect(props: {
  label: string
  value: string | null
  placeholder: string
  disabled?: boolean
  options: Array<{ value: string; label: string }>
  onValueChange: (value: string | null) => void
}) {
  return (
    <label className='min-w-40 flex-1 space-y-1 sm:max-w-64'>
      <span className='text-muted-foreground block text-xs font-medium'>
        {props.label}
      </span>
      <Select
        value={props.value}
        onValueChange={props.onValueChange}
        disabled={props.disabled}
      >
        <SelectTrigger className='w-full'>
          <SelectValue placeholder={props.placeholder}>
            {getPerformanceFilterLabel(props.value, props.options)}
          </SelectValue>
        </SelectTrigger>
        <SelectContent align='start'>
          <SelectGroup>
            {props.options.map((option) => (
              <SelectItem key={option.value} value={option.value}>
                {option.label}
              </SelectItem>
            ))}
          </SelectGroup>
        </SelectContent>
      </Select>
    </label>
  )
}

function SummaryCards({ summary }: { summary: PerformanceAnalyticsSummary }) {
  const { t } = useTranslation()
  const cards = [
    {
      label: t('Requests'),
      value: summary.request_count.toLocaleString(),
    },
    {
      label: t('Success rate'),
      value: formatUptimePct(summary.success_rate),
    },
    {
      label: t('Cache hit rate'),
      value: formatUptimePct(summary.cache_hit_rate),
    },
    {
      label: t('Average RPM'),
      value: formatThroughput(summary.rpm),
    },
    {
      label: t('Average TPM'),
      value: formatThroughput(summary.tpm),
    },
    {
      label: t('TTFT samples'),
      value: summary.ttft.sample_count.toLocaleString(),
    },
    {
      label: t('TPOT samples'),
      value: summary.tpot.sample_count.toLocaleString(),
    },
  ]

  return (
    <div className='grid grid-cols-2 gap-2 sm:grid-cols-3 lg:grid-cols-4 xl:grid-cols-7'>
      {cards.map((card) => (
        <Card key={card.label} size='sm'>
          <CardContent>
            <div className='text-muted-foreground text-xs font-medium'>
              {card.label}
            </div>
            <div className='mt-1 text-2xl font-semibold tabular-nums'>
              {card.value}
            </div>
          </CardContent>
        </Card>
      ))}
    </div>
  )
}

function formatThroughput(value: number): string {
  return value.toLocaleString(undefined, { maximumFractionDigits: 2 })
}

function LatencyChart(props: {
  title: string
  description: string
  metric: 'ttft' | 'tpot'
  summary: PerformancePercentiles
  points: PerformanceAnalyticsPoint[]
  formatTime: (ts: number) => string
}) {
  const { t } = useTranslation()
  const { resolvedTheme, themeReady } = useChartTheme()
  const formatTime = props.formatTime
  const chartTextColor =
    resolvedTheme === 'dark'
      ? 'rgba(255, 255, 255, 0.68)'
      : 'rgba(15, 23, 42, 0.58)'
  const chartGridColor =
    resolvedTheme === 'dark'
      ? 'rgba(255, 255, 255, 0.12)'
      : 'rgba(15, 23, 42, 0.12)'
  const series = useMemo(
    () => buildPerformanceSeries(props.points, props.metric, formatTime),
    [formatTime, props.metric, props.points]
  )
  const spec = useMemo(
    () => ({
      type: 'line' as const,
      data: [{ id: `${props.metric}-percentiles`, values: series }],
      xField: 'time',
      yField: 'value',
      seriesField: 'percentile',
      point: { visible: series.length <= 60 },
      legends: {
        visible: true,
        orient: 'top',
        position: 'end',
      },
      axes: [
        buildPerformanceTimeAxis(formatTime, chartTextColor),
        {
          orient: 'left',
          label: {
            formatMethod: (value: number | string) =>
              formatLatency(Number(value)),
            style: { fill: chartTextColor, fontSize: 10 },
          },
          grid: {
            visible: true,
            style: { lineDash: [3, 3], stroke: chartGridColor },
          },
        },
      ],
      tooltip: {
        dimension: {
          title: {
            value: (datum: Record<string, unknown>) =>
              String(datum?.label ?? ''),
          },
          content: [
            {
              key: (datum: Record<string, unknown>) =>
                String(datum?.percentile ?? ''),
              value: (datum: Record<string, unknown>) =>
                formatLatency(Number(datum?.value)),
            },
          ],
        },
      },
      animationAppear: { duration: 400 },
    }),
    [chartGridColor, chartTextColor, formatTime, props.metric, series]
  )

  return (
    <Card>
      <CardHeader>
        <CardTitle>{t(props.title)}</CardTitle>
        <CardDescription>{props.description}</CardDescription>
      </CardHeader>
      <CardContent>
        <div className='mb-3 grid grid-cols-3 gap-2'>
          {[
            ['P50', props.summary.p50_ms],
            ['P90', props.summary.p90_ms],
            ['P99', props.summary.p99_ms],
          ].map(([label, value]) => (
            <div key={String(label)} className='bg-muted/50 rounded-lg p-2'>
              <div className='text-muted-foreground text-xs'>{label}</div>
              <div className='font-mono text-sm font-semibold tabular-nums'>
                {formatLatency(Number(value))}
              </div>
            </div>
          ))}
        </div>
        <div className='h-72'>
          {themeReady && series.length > 0 ? (
            <VChart
              key={`${props.metric}-${resolvedTheme}`}
              spec={{
                ...spec,
                theme: resolvedTheme === 'dark' ? 'dark' : 'light',
                background: 'transparent',
              }}
              option={VCHART_OPTION}
            />
          ) : (
            <div className='text-muted-foreground flex h-full items-center justify-center text-sm'>
              <Clock3 className='mr-2 size-4' />
              {t('No performance data available')}
            </div>
          )}
        </div>
      </CardContent>
    </Card>
  )
}

function MetricChart(props: {
  className?: string
  title: string
  description: string
  metric: PerformanceMetric
  points: PerformanceAnalyticsPoint[]
  formatTime: (ts: number) => string
  formatValue: (value: number) => string
}) {
  const { t } = useTranslation()
  const { resolvedTheme, themeReady } = useChartTheme()
  const formatTime = props.formatTime
  const formatValue = props.formatValue
  const chartTextColor =
    resolvedTheme === 'dark'
      ? 'rgba(255, 255, 255, 0.68)'
      : 'rgba(15, 23, 42, 0.58)'
  const chartGridColor =
    resolvedTheme === 'dark'
      ? 'rgba(255, 255, 255, 0.12)'
      : 'rgba(15, 23, 42, 0.12)'
  const series = useMemo(
    () => buildPerformanceMetricSeries(props.points, props.metric, formatTime),
    [formatTime, props.metric, props.points]
  )
  const spec = useMemo(
    () => ({
      type: 'line' as const,
      data: [{ id: `${props.metric}-series`, values: series }],
      xField: 'time',
      yField: 'value',
      point: { visible: series.length <= 60 },
      axes: [
        buildPerformanceTimeAxis(formatTime, chartTextColor),
        {
          orient: 'left',
          label: {
            formatMethod: (value: number | string) =>
              formatValue(Number(value)),
            style: { fill: chartTextColor, fontSize: 10 },
          },
          grid: {
            visible: true,
            style: { lineDash: [3, 3], stroke: chartGridColor },
          },
        },
      ],
      tooltip: {
        dimension: {
          title: {
            value: (datum: Record<string, unknown>) =>
              String(datum?.label ?? ''),
          },
          content: [
            {
              key: props.title,
              value: (datum: Record<string, unknown>) =>
                formatValue(Number(datum?.value)),
            },
          ],
        },
      },
      animationAppear: { duration: 400 },
    }),
    [
      chartGridColor,
      chartTextColor,
      formatTime,
      formatValue,
      props.metric,
      props.title,
      series,
    ]
  )

  return (
    <Card className={props.className}>
      <CardHeader>
        <CardTitle>{t(props.title)}</CardTitle>
        <CardDescription>{props.description}</CardDescription>
      </CardHeader>
      <CardContent>
        <div className='h-72'>
          {themeReady && series.length > 0 ? (
            <VChart
              key={`${props.metric}-${resolvedTheme}`}
              spec={{
                ...spec,
                theme: resolvedTheme === 'dark' ? 'dark' : 'light',
                background: 'transparent',
              }}
              option={VCHART_OPTION}
            />
          ) : (
            <div className='text-muted-foreground flex h-full items-center justify-center text-sm'>
              <Activity className='mr-2 size-4' />
              {t('No performance data available')}
            </div>
          )}
        </div>
      </CardContent>
    </Card>
  )
}

function PerformanceAnalyticsSkeleton() {
  return (
    <div className='space-y-3'>
      <div className='grid grid-cols-2 gap-2 sm:grid-cols-3 lg:grid-cols-4 xl:grid-cols-7'>
        {['requests', 'success', 'cache', 'rpm', 'tpm', 'ttft', 'tpot'].map(
          (key) => (
            <Skeleton key={key} className='h-24 rounded-xl' />
          )
        )}
      </div>
      <div className='grid gap-3 xl:grid-cols-2'>
        <Skeleton className='h-[430px] rounded-xl' />
        <Skeleton className='h-[430px] rounded-xl' />
      </div>
      <div className='grid gap-3 xl:grid-cols-2'>
        <Skeleton className='h-[390px] rounded-xl' />
        <Skeleton className='h-[390px] rounded-xl' />
        <Skeleton className='h-[390px] rounded-xl xl:col-span-2' />
      </div>
    </div>
  )
}
