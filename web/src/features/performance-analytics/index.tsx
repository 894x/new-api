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
import { keepPreviousData, useQuery } from '@tanstack/react-query'
import { Activity, ChartNoAxesCombined, Clock3, RefreshCw } from 'lucide-react'
import {
  type ReactNode,
  useCallback,
  useId,
  useEffect,
  useMemo,
  useRef,
  useState,
} from 'react'
import { useTranslation } from 'react-i18next'

import { ErrorState } from '@/components/error-state'
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
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { Tabs, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { buildPerformanceAnalyticsParams } from '@/features/dashboard/lib/filters'
import { formatUptimePct } from '@/features/performance-metrics/lib/format'
import { CompactDateTimeRangePicker } from '@/features/usage-logs/components/compact-date-time-range-picker'
import { toIntlLocale } from '@/i18n/languages'
import { formatNumber } from '@/lib/format'
import { dateToUnixTimestamp } from '@/lib/time'

import { getPerformanceAnalytics, getPerformanceAnalyticsOptions } from './api'
import { PerformanceChartContext } from './chart-context'
import { PerformanceChartHeader } from './chart-header'
import { PerformanceChartToolbar } from './chart-toolbar'
import {
  buildPerformanceMetricSeries,
  PERFORMANCE_CURVE_LABELS,
  buildPerformanceSeries,
  formatRequestBodySize,
  formatPerformanceTimestamp,
  formatTtftSeconds,
  formatPerformanceLatency,
  getPerformanceFilterLabel,
} from './lib'
import { PerformanceChart } from './performance-chart'
import type {
  PerformanceAnalyticsData,
  PerformanceAnalyticsPoint,
  PerformanceAnalyticsSummary,
  PerformanceMetric,
  PerformancePercentiles,
} from './types'

const ALL_VALUE = '__all__'
const RANGE_OPTIONS = [
  { seconds: 60 * 60, label: '1 Hour' },
  { seconds: 3 * 60 * 60, label: '3 Hours' },
  { seconds: 6 * 60 * 60, label: '6 Hours' },
  { seconds: 12 * 60 * 60, label: '12 Hours' },
  { seconds: 24 * 60 * 60, label: '1 Day' },
  { seconds: 7 * 24 * 60 * 60, label: '7 Days' },
  { seconds: 29 * 24 * 60 * 60, label: '29 Days' },
] as const
const USER_MONITORING_RANGE_OPTIONS = [
  { seconds: 10 * 60, label: '10 Minutes' },
  { seconds: 60 * 60, label: '1 Hour' },
  { seconds: 3 * 60 * 60, label: '3 Hours' },
  { seconds: 6 * 60 * 60, label: '6 Hours' },
  { seconds: 12 * 60 * 60, label: '12 Hours' },
  { seconds: 24 * 60 * 60, label: '1 Day' },
] as const

type PerformanceAnalyticsProps = {
  isAdmin: boolean
  standalone?: boolean
}

export function PerformanceAnalytics({
  isAdmin,
  standalone = false,
}: PerformanceAnalyticsProps) {
  const { t, i18n } = useTranslation()
  const [modelName, setModelName] = useState('')
  const [userId, setUserId] = useState<number>()
  const [tokenId, setTokenId] = useState<number>()
  const [channelId, setChannelId] = useState<number>()
  const [rangeSeconds, setRangeSeconds] = useState(
    standalone ? 10 * 60 : 24 * 60 * 60
  )
  const [rangeEnd, setRangeEnd] = useState(() => Math.floor(Date.now() / 1000))
  const requestWindowEnd = useRef(rangeEnd)
  const [customRange, setCustomRange] = useState<{
    start: number
    end: number
  }>()
  const [rangeError, setRangeError] = useState(false)

  const chartGroup = useId()
  const [live, setLive] = useState(true)
  const [chartReset, setChartReset] = useState(0)
  const pauseLive = useCallback(() => setLive(false), [])

  const optionsQuery = useQuery({
    queryKey: [
      'performance-analytics',
      'options',
      isAdmin ? 'admin' : 'self',
      isAdmin ? userId : undefined,
    ],
    queryFn: ({ signal }) =>
      getPerformanceAnalyticsOptions(isAdmin, userId, signal),
    select: (response) => response.data,
    placeholderData: keepPreviousData,
    staleTime: 60_000,
    retry: false,
    meta: { errorToast: false, redirectOnServerError: false },
  })

  const options = optionsQuery.data
  const models = useMemo(() => options?.models ?? [], [options?.models])
  const users = useMemo(() => options?.users ?? [], [options?.users])
  const channels = useMemo(() => options?.channels ?? [], [options?.channels])
  const tokens = useMemo(() => options?.tokens ?? [], [options?.tokens])

  useEffect(() => {
    if (standalone && userId === undefined && users.length > 0) {
      setUserId(users[0].id)
    }
  }, [standalone, userId, users])

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
          startTimestamp: customRange?.start ?? rangeEnd - rangeSeconds,
          endTimestamp: customRange?.end ?? rangeEnd,
          userId,
          tokenId,
          channelId,
        },
        isAdmin
      ),
    [
      channelId,
      customRange,
      isAdmin,
      modelName,
      rangeEnd,
      rangeSeconds,
      tokenId,
      userId,
    ]
  )

  const analyticsQuery = useQuery({
    queryKey: [
      'performance-analytics',
      'series',
      isAdmin ? 'admin' : 'self',
      queryParams,
    ],
    queryFn: ({ signal }) => {
      if (customRange) {
        return getPerformanceAnalytics(queryParams, isAdmin, signal)
      }
      const endTimestamp = live
        ? Math.floor(Date.now() / 1000)
        : requestWindowEnd.current
      requestWindowEnd.current = endTimestamp
      return getPerformanceAnalytics(
        {
          ...queryParams,
          start_timestamp: endTimestamp - rangeSeconds,
          end_timestamp: endTimestamp,
        },
        isAdmin,
        signal
      )
    },
    select: (response) => response.data,
    enabled: modelName.length > 0 && (!standalone || userId !== undefined),
    staleTime: 30_000,
    refetchInterval: live ? 15_000 : false,
    refetchOnWindowFocus: live,
    refetchOnReconnect: live,
    refetchIntervalInBackground: false,
    retry: false,
    meta: { errorToast: false, redirectOnServerError: false },
  })

  const handleRangeChange = useCallback((seconds: number) => {
    setCustomRange(undefined)
    setRangeError(false)
    setLive(true)
    setRangeSeconds(seconds)
    setChartReset((value) => value + 1)
    const now = Math.floor(Date.now() / 1000)
    requestWindowEnd.current = now
    setRangeEnd(now)
  }, [])

  const handleRefresh = useCallback(() => {
    if (customRange) {
      void optionsQuery.refetch()
      void analyticsQuery.refetch()
      return
    }
    const now = Math.floor(Date.now() / 1000)
    requestWindowEnd.current = now
    setRangeEnd(now)
    void optionsQuery.refetch()
    if (modelName && now === rangeEnd) {
      void analyticsQuery.refetch()
    }
  }, [analyticsQuery, customRange, modelName, optionsQuery, rangeEnd])

  const resumeLive = () => {
    handleRangeChange(rangeSeconds)
    if (!customRange) handleRefresh()
  }

  const handleCustomRangeChange = (range: { start?: Date; end?: Date }) => {
    const start = range.start ? dateToUnixTimestamp(range.start) : Number.NaN
    const end = range.end ? dateToUnixTimestamp(range.end) : Number.NaN
    if (
      !Number.isFinite(start) ||
      !Number.isFinite(end) ||
      start <= 0 ||
      end <= start ||
      end - start > 30 * 24 * 3600
    ) {
      setRangeError(true)
      return
    }
    setRangeError(false)
    setCustomRange({ start, end })
    setLive(false)
    setChartReset((value) => value + 1)
  }

  const handleUserChange = useCallback((value: string | null) => {
    setUserId(value && value !== ALL_VALUE ? Number(value) : undefined)
    setTokenId(undefined)
  }, [])

  const formatTime = useCallback(
    (ts: number) =>
      formatPerformanceTimestamp(ts, i18n.resolvedLanguage || i18n.language),
    [i18n.language, i18n.resolvedLanguage]
  )
  const locale = toIntlLocale(i18n.resolvedLanguage || i18n.language)

  const resultScope = JSON.stringify([
    isAdmin,
    modelName,
    userId,
    tokenId,
    isAdmin ? channelId : undefined,
    rangeSeconds,
    customRange?.start,
    customRange?.end,
  ])
  const [lastResult, setLastResult] = useState<{
    scope: string
    data: PerformanceAnalyticsData
  }>()
  useEffect(() => {
    if (analyticsQuery.data) {
      setLastResult({ scope: resultScope, data: analyticsQuery.data })
    }
  }, [analyticsQuery.data, resultScope])
  const data =
    analyticsQuery.data ??
    (lastResult?.scope === resultScope ? lastResult.data : undefined)
  const loading = optionsQuery.isLoading || analyticsQuery.isLoading
  const fetching = optionsQuery.isFetching || analyticsQuery.isFetching
  const hasData = Boolean(
    data &&
    (data.summary.request_count > 0 ||
      data.summary.error_count > 0 ||
      data.series.some((point) => point.error_count > 0))
  )
  let analyticsContent: ReactNode = null

  if (loading && !data) {
    analyticsContent = <PerformanceAnalyticsSkeleton />
  } else if ((optionsQuery.isError || analyticsQuery.isError) && !data) {
    analyticsContent = (
      <ErrorState
        title={t('Failed to load performance data')}
        description={t('Refresh the page or try again later.')}
        onRetry={handleRefresh}
      />
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
        {(optionsQuery.isError || analyticsQuery.isError) && (
          <p role='status' className='text-destructive text-sm'>
            {t(
              'Performance data could not be refreshed. Showing the last result.'
            )}
          </p>
        )}
        {data.errors_unavailable && (
          <p role='status' className='text-muted-foreground text-sm'>
            {t(
              'Error breakdown is unavailable. Other performance statistics are available.'
            )}
          </p>
        )}
        {data.source === 'usage_logs' && (
          <p role='status' className='text-muted-foreground text-xs'>
            {t(
              'Channel statistics use recorded usage logs. Missing historical measurements are omitted.'
            )}
            {data.truncated && (
              <span className='text-destructive ml-1'>
                {t(
                  'The channel log scan reached its limit; these results are partial. Narrow the time range.'
                )}
              </span>
            )}
          </p>
        )}
        <SummaryCards
          summary={data.summary}
          locale={locale}
          errorsUnavailable={data.errors_unavailable}
        />
        <div
          role='group'
          aria-label={t('Latency and cache performance')}
          className='grid gap-3 md:grid-cols-2 xl:grid-cols-3'
        >
          <LatencyChart
            title='TTFT'
            description={t('First response time')}
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
          <MetricChart
            title={t('Cache hit rate')}
            description={t('Share of input tokens served from cache')}
            metric='cache_hit_rate'
            summary={data.summary}
            points={data.series}
            formatTime={formatTime}
            formatValue={formatUptimePct}
          />
        </div>
        <div
          role='group'
          aria-label={t('Request payload and cache writes')}
          className='grid gap-3 md:grid-cols-2 xl:grid-cols-3'
        >
          <MetricChart
            title={t('Request body size')}
            metric='request_body_avg_bytes'
            summary={data.summary}
            points={data.series}
            formatTime={formatTime}
            formatValue={(value) => formatRequestBodySize(value, locale)}
            formatTooltipValue={(value) =>
              `${formatRequestBodySize(value, locale)} (${formatNumber(value, locale)} B)`
            }
          />
          <MetricChart
            title={t('Cache Write')}
            description={t('Upstream cache write tokens')}
            metric='cache_write_tokens'
            summary={data.summary}
            points={data.series}
            formatTime={formatTime}
            formatValue={(value) => formatNumber(value, locale)}
          />
        </div>
        <div
          role='group'
          aria-label={t('Throughput and errors')}
          className='grid gap-3 md:grid-cols-2 xl:grid-cols-3'
        >
          <MetricChart
            title='RPM'
            description={t('Requests completed per minute')}
            metric='rpm'
            points={data.series}
            formatTime={formatTime}
            formatValue={(value) => formatThroughput(value, locale)}
          />
          <MetricChart
            title='TPM'
            description={t('Input and output tokens processed per minute')}
            metric='tpm'
            points={data.series}
            formatTime={formatTime}
            formatValue={(value) => formatThroughput(value, locale)}
          />
          {!data.errors_unavailable && (
            <MetricChart
              title='Errors'
              description={t('Errors recorded for the selected filters')}
              metric='error_count'
              points={data.series}
              formatTime={formatTime}
              formatValue={(value) => formatNumber(value, locale)}
            />
          )}
        </div>
        {!data.errors_unavailable &&
          (!isAdmin || userId !== undefined || channelId !== undefined) && (
            <UserErrorSummary
              groups={data.error_groups ?? []}
              scannedLogs={data.scanned_logs}
              truncated={data.truncated}
              locale={locale}
            />
          )}
      </>
    )
  }

  let userFilterValue: string | null = ALL_VALUE
  if (standalone) {
    userFilterValue = null
  }
  if (userId) {
    userFilterValue = String(userId)
  }

  return (
    <PerformanceChartContext
      value={{
        group: `${chartGroup}-${resultScope}`,
        start:
          data?.effective_start_timestamp ??
          customRange?.start ??
          rangeEnd - rangeSeconds,
        end: data?.effective_end_timestamp ?? customRange?.end ?? rangeEnd,
        reset: chartReset,
        onExplore: pauseLive,
      }}
    >
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
                {t(
                  'Percentiles are estimated from request histograms; MAX is the measured maximum. Missing historical measurements are omitted.'
                )}
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
          <CardContent className='grid items-end gap-3 sm:grid-cols-2 lg:grid-cols-3 xl:grid-cols-4'>
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
                value={userFilterValue}
                placeholder={t('All users')}
                disabled={standalone && users.length === 0}
                onValueChange={handleUserChange}
                options={[
                  ...(standalone
                    ? []
                    : [{ value: ALL_VALUE, label: t('All users') }]),
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
            {isAdmin && (
              <FilterSelect
                label={t('Channel')}
                value={channelId ? String(channelId) : ALL_VALUE}
                placeholder={t('All channels')}
                onValueChange={(value) =>
                  setChannelId(
                    value && value !== ALL_VALUE ? Number(value) : undefined
                  )
                }
                options={[
                  { value: ALL_VALUE, label: t('All channels') },
                  ...channels.map((channel) => ({
                    value: String(channel.id),
                    label: `${channel.name} (#${channel.id})`,
                  })),
                ]}
              />
            )}
            <div className='min-w-0 space-y-1 sm:col-span-2 lg:col-span-3 xl:col-span-2'>
              <span className='text-muted-foreground block text-xs font-medium'>
                {t('Time range')}
              </span>
              <Tabs
                value={customRange ? 'custom' : String(rangeSeconds)}
                className='max-w-full'
                onValueChange={(value) => handleRangeChange(Number(value))}
              >
                <TabsList className='max-w-full flex-wrap group-data-horizontal/tabs:h-auto group-data-horizontal/tabs:min-h-8'>
                  {(standalone
                    ? USER_MONITORING_RANGE_OPTIONS
                    : RANGE_OPTIONS
                  ).map((option) => (
                    <TabsTrigger
                      key={option.seconds}
                      value={String(option.seconds)}
                      className='h-6 px-2.5 text-xs'
                    >
                      {t(option.label)}
                    </TabsTrigger>
                  ))}
                </TabsList>
              </Tabs>
            </div>
            <div className='min-w-0 space-y-1'>
              <span className='text-muted-foreground block text-xs font-medium'>
                {t('Date Range')}
              </span>
              <CompactDateTimeRangePicker
                start={
                  customRange ? new Date(customRange.start * 1000) : undefined
                }
                end={customRange ? new Date(customRange.end * 1000) : undefined}
                onChange={handleCustomRangeChange}
                className='h-8 min-w-0 text-xs'
              />
            </div>
            <Button
              variant='outline'
              size='sm'
              className='h-8 justify-self-start'
              onClick={handleRefresh}
              disabled={fetching}
            >
              <RefreshCw className={fetching ? 'animate-spin' : undefined} />
              {t('Refresh')}
            </Button>
            {rangeError && (
              <p
                role='alert'
                className='text-destructive text-sm sm:col-span-2 lg:col-span-3 xl:col-span-4'
              >
                {t('Select a valid time range of up to 30 days.')}
              </p>
            )}
          </CardContent>
        </Card>

        <PerformanceChartToolbar
          live={live}
          onPause={pauseLive}
          onResume={resumeLive}
          onReset={() => setChartReset((value) => value + 1)}
          updatedAt={analyticsQuery.dataUpdatedAt}
          formatTime={formatTime}
        />
        {analyticsContent}
      </div>
    </PerformanceChartContext>
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
    <label className='min-w-0 space-y-1'>
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

function SummaryCards({
  summary,
  locale,
  errorsUnavailable,
}: {
  errorsUnavailable?: boolean
  summary: PerformanceAnalyticsSummary
  locale: Intl.LocalesArgument
}) {
  const { t } = useTranslation()
  const cards = [
    {
      label: t('Average request body size'),
      value:
        summary.request_body_avg_bytes == null
          ? t('Unknown')
          : formatRequestBodySize(summary.request_body_avg_bytes, locale),
    },
    {
      label: t('Maximum request body size'),
      value: !summary.request_body_samples
        ? t('Unknown')
        : formatRequestBodySize(summary.request_body_max_bytes ?? 0, locale),
    },
    {
      label: t('Cache Read'),
      value: !summary.cache_read_samples
        ? t('Unknown')
        : formatNumber(summary.cache_read_tokens ?? 0, locale),
    },
    {
      label: t('Cache Write'),
      value: !summary.cache_write_samples
        ? t('Unknown')
        : formatNumber(summary.cache_write_tokens ?? 0, locale),
    },
    ...(summary.cache_write_5m_tokens
      ? [
          {
            label: t('Cache Write (5m)'),
            value: formatNumber(summary.cache_write_5m_tokens, locale),
          },
        ]
      : []),
    ...(summary.cache_write_1h_tokens
      ? [
          {
            label: t('Cache Write (1h)'),
            value: formatNumber(summary.cache_write_1h_tokens, locale),
          },
        ]
      : []),
    {
      label: t('Requests'),
      value: formatNumber(summary.request_count, locale),
    },
    {
      label: t('Success rate'),
      value: formatUptimePct(summary.success_rate),
    },
    {
      label: t('Errors'),
      value: errorsUnavailable
        ? t('Unknown')
        : formatNumber(summary.error_count, locale),
    },
    {
      label: t('Cache hit rate'),
      value:
        summary.cache_read_samples === 0
          ? t('Unknown')
          : formatUptimePct(summary.cache_hit_rate),
    },
    {
      label: t('Average RPM'),
      value: formatThroughput(summary.rpm, locale),
    },
    {
      label: t('Average TPM'),
      value: formatThroughput(summary.tpm, locale),
    },
    {
      label: t('TTFT samples'),
      value: formatNumber(summary.ttft.sample_count, locale),
    },
    {
      label: t('TPOT samples'),
      value: formatNumber(summary.tpot.sample_count, locale),
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

function formatThroughput(
  value: number,
  locale?: Intl.LocalesArgument
): string {
  return formatNumber(value, locale)
}

function UserErrorSummary({
  groups,
  scannedLogs,
  truncated,
  locale,
}: {
  groups: PerformanceAnalyticsData['error_groups']
  scannedLogs: number
  truncated: boolean
  locale: Intl.LocalesArgument
}) {
  const { t } = useTranslation()
  return (
    <Card>
      <CardHeader>
        <CardTitle>{t('Error breakdown')}</CardTitle>
        <CardDescription>
          {t('Errors grouped by code, type, and HTTP status.')}
          {scannedLogs > 0 &&
            ` ${t('Scanned logs')}: ${formatNumber(scannedLogs, locale)}.`}
        </CardDescription>
      </CardHeader>
      <CardContent>
        {truncated && (
          <p role='alert' className='text-destructive mb-3 text-sm'>
            {t(
              'The error log scan reached its limit; these results are partial.'
            )}
          </p>
        )}
        {groups.length === 0 ? (
          <div className='text-muted-foreground py-6 text-center text-sm'>
            {t('No user errors in this time range')}
          </div>
        ) : (
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>{t('Code')}</TableHead>
                <TableHead>{t('Type')}</TableHead>
                <TableHead>{t('Status Code')}</TableHead>
                <TableHead className='text-right'>{t('Count')}</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {groups.map((group) => (
                <TableRow
                  key={`${group.error_code}-${group.error_type}-${group.status_code}`}
                >
                  <TableCell className='font-mono'>
                    {group.error_code}
                  </TableCell>
                  <TableCell>{group.error_type}</TableCell>
                  <TableCell>{group.status_code || '—'}</TableCell>
                  <TableCell className='text-right'>
                    {formatNumber(group.count, locale)}
                  </TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        )}
      </CardContent>
    </Card>
  )
}

function LatencyChart(props: {
  title: string
  description: string
  metric: 'ttft' | 'tpot'
  summary: PerformancePercentiles
  points: PerformanceAnalyticsPoint[]
  formatTime: (ts: number) => string
}) {
  const { t, i18n } = useTranslation()
  const series = useMemo(
    () => buildPerformanceSeries(props.points, props.metric, props.formatTime),
    [props.formatTime, props.metric, props.points]
  )
  const locale = toIntlLocale(i18n.resolvedLanguage || i18n.language)
  const formatValue = (value: number) =>
    props.metric === 'ttft'
      ? formatTtftSeconds(value, locale)
      : formatPerformanceLatency(value, locale)

  return (
    <Card>
      <PerformanceChartHeader
        title={t(props.title)}
        description={props.description}
        formatValue={formatValue}
        statistics={[
          ['AVG', props.summary.avg_ms],
          ['P50', props.summary.p50_ms],
          ['P90', props.summary.p90_ms],
          ['P95', props.summary.p95_ms],
          ['P99', props.summary.p99_ms],
          ['MAX', props.summary.max_ms],
        ].map(([label, value]) => ({
          label: String(label),
          value:
            props.summary.sample_count > 0 && typeof value === 'number'
              ? value
              : undefined,
        }))}
      />
      <CardContent>
        {series.length > 0 ? (
          <PerformanceChart
            title={t(props.title)}
            values={series}
            timestamps={props.points.map((point) => point.ts)}
            formatTime={props.formatTime}
            formatValue={formatValue}
            formatTooltipValue={(value) =>
              props.metric === 'ttft'
                ? `${formatValue(value)} (${formatNumber(value, locale)} ms)`
                : formatValue(value)
            }
          />
        ) : (
          <div className='text-muted-foreground flex h-56 items-center justify-center gap-2 text-sm'>
            <Clock3 className='size-4' />
            {t('No performance data available')}
          </div>
        )}
      </CardContent>
    </Card>
  )
}

function MetricChart(props: {
  className?: string
  summary?: PerformanceAnalyticsSummary
  title: string
  description?: string
  metric: PerformanceMetric
  points: PerformanceAnalyticsPoint[]
  formatTime: (ts: number) => string
  formatValue: (value: number) => string
  formatTooltipValue?: (value: number) => string
}) {
  const { t } = useTranslation()
  const series = useMemo(
    () =>
      buildPerformanceMetricSeries(
        props.points,
        props.metric,
        props.formatTime
      ),
    [props.formatTime, props.metric, props.points]
  )

  const statistics = useMemo(() => {
    if (!props.summary) return undefined
    const summarySeries = buildPerformanceMetricSeries(
      [{ ...props.summary, ts: 0 }],
      props.metric,
      String
    )
    return PERFORMANCE_CURVE_LABELS.map((label) => ({
      label,
      value: summarySeries.find((datum) => datum.percentile === label)?.value,
    }))
  }, [props.metric, props.summary])

  return (
    <Card className={props.className}>
      <PerformanceChartHeader
        title={t(props.title)}
        description={props.description}
        formatValue={props.formatValue}
        statistics={statistics}
      />
      <CardContent>
        {series.length > 0 ? (
          <PerformanceChart
            title={t(props.title)}
            values={series}
            timestamps={props.points.map((point) => point.ts)}
            formatTime={props.formatTime}
            formatValue={props.formatValue}
            formatTooltipValue={props.formatTooltipValue}
          />
        ) : (
          <div className='text-muted-foreground flex h-56 items-center justify-center gap-2 text-sm'>
            <Activity className='size-4' />
            {t('No performance data available')}
          </div>
        )}
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
      {(
        [
          ['latency', 3],
          ['request', 2],
          ['traffic', 3],
        ] as const
      ).map(([row, count]) => (
        <div key={row} className='grid gap-3 md:grid-cols-2 xl:grid-cols-3'>
          {Array.from({ length: count }, (_, index) => (
            <Skeleton key={index} className='h-[320px] rounded-xl' />
          ))}
        </div>
      ))}
    </div>
  )
}
