import { useQuery } from '@tanstack/react-query'
import { RefreshCw } from 'lucide-react'
import {
  type ReactNode,
  useCallback,
  useEffect,
  useId,
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
  CardHeader,
  CardTitle,
  CardDescription,
} from '@/components/ui/card'
import { Input } from '@/components/ui/input'
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
import { CompactDateTimeRangePicker } from '@/features/usage-logs/components/compact-date-time-range-picker'
import { dateToUnixTimestamp } from '@/lib/time'

import { getChannelPerformanceAnalytics, getHTTPTransportMetrics } from './api'
import type { TransportHistoryPoint } from './channel-charts'
import { ChannelDiagnosticsContent } from './channel-diagnostics-content'
import { PerformanceChartContext } from './chart-context'
import { PerformanceChartToolbar } from './chart-toolbar'
import { formatPerformanceTimestamp } from './lib'
import type { ChannelAnalyticsData } from './types'

const CHANNEL_RANGE_OPTIONS = [
  { seconds: 600, label: '10 Minutes', bucket: 10 },
  { seconds: 3600, label: '1 Hour', bucket: 60 },
  { seconds: 10800, label: '3 Hours', bucket: 300 },
  { seconds: 21600, label: '6 Hours', bucket: 300 },
  { seconds: 43200, label: '12 Hours', bucket: 300 },
  { seconds: 86400, label: '1 Day', bucket: 300 },
] as const
const ALL_CHANNELS_VALUE = '__all_channels__'
export type ChannelMonitoringOption = {
  id: number
  name: string
  status: number
}
type ChannelRangeOption = (typeof CHANNEL_RANGE_OPTIONS)[number]

export function ChannelPerformanceDiagnostics(props: {
  formatTime?: (ts: number) => string
  channelOptions?: ChannelMonitoringOption[]
}) {
  const { t, i18n } = useTranslation()
  const channelOptions = props.channelOptions
  const formatTime = props.formatTime
  const [channelId, setChannelId] = useState<number>()
  const [range, setRange] = useState<ChannelRangeOption>(
    CHANNEL_RANGE_OPTIONS[0]
  )
  const [rangeEnd, setRangeEnd] = useState(() => Math.floor(Date.now() / 1000))
  const [customRange, setCustomRange] = useState<{
    start: number
    end: number
  }>()
  const [rangeError, setRangeError] = useState(false)
  const [live, setLive] = useState(true)
  const [reset, setReset] = useState(0)
  const requestEnd = useRef(rangeEnd)
  const group = useId()
  const pause = useCallback(() => setLive(false), [])
  const queryParams = useMemo(
    () => ({
      ...(channelId ? { channel_id: channelId } : {}),
      start_timestamp: customRange?.start ?? rangeEnd - range.seconds,
      end_timestamp: customRange?.end ?? rangeEnd,
      bucket_seconds: customRange
        ? Math.max(
            10,
            Math.min(
              300,
              Math.ceil((customRange.end - customRange.start) / 120)
            )
          )
        : range.bucket,
    }),
    [channelId, customRange, range, rangeEnd]
  )
  const query = useQuery({
    queryKey: ['performance-analytics', 'channel', queryParams],
    queryFn: ({ signal }) => {
      if (customRange) {
        return getChannelPerformanceAnalytics(queryParams, signal)
      }
      const end = live ? Math.floor(Date.now() / 1000) : requestEnd.current
      requestEnd.current = end
      return getChannelPerformanceAnalytics(
        {
          ...queryParams,
          start_timestamp: end - range.seconds,
          end_timestamp: end,
        },
        signal
      )
    },
    select: (response) => response.data,
    refetchInterval: live ? 15000 : false,
    refetchOnWindowFocus: live,
    refetchOnReconnect: live,
    staleTime: 15000,
    retry: false,
    meta: { errorToast: false, redirectOnServerError: false },
  })
  const scope = JSON.stringify([
    channelId,
    range.seconds,
    customRange?.start,
    customRange?.end,
  ])
  const [lastResult, setLastResult] = useState<{
    scope: string
    data: ChannelAnalyticsData
  }>()
  useEffect(() => {
    if (query.data) setLastResult({ scope, data: query.data })
  }, [query.data, scope])
  const data =
    query.data ?? (lastResult?.scope === scope ? lastResult.data : undefined)
  useEffect(() => {
    if (
      channelOptions &&
      channelId &&
      !channelOptions.some((channel) => channel.id === channelId)
    ) {
      setChannelId(undefined)
    }
  }, [channelId, channelOptions])
  const transportQuery = useQuery({
    queryKey: ['performance-analytics', 'transport', channelId],
    queryFn: ({ signal }) => getHTTPTransportMetrics(signal),
    select: (response) => response.data,
    refetchInterval: live ? 15000 : false,
    refetchOnWindowFocus: live,
    refetchOnReconnect: live,
    staleTime: 5000,
    retry: false,
    meta: { errorToast: false, redirectOnServerError: false },
  })
  const [history, setHistory] = useState<{
    channelId?: number
    points: TransportHistoryPoint[]
  }>({ points: [] })
  useEffect(() => {
    const metrics = transportQuery.data
    if (!metrics) return
    const ts = Math.floor(Date.now() / 1000)
    const connections = metrics.reduce(
      (sum, metric) => sum + metric.connections,
      0
    )
    const activeRequests = metrics.reduce(
      (sum, metric) =>
        sum +
        (channelId
          ? (metric.active_by_channel?.[String(channelId)] ?? 0)
          : metric.active_requests),
      0
    )
    setHistory((previous) => ({
      channelId,
      points: [
        ...(previous.channelId === channelId
          ? previous.points.filter((point) => point.ts !== ts)
          : []),
        { ts, connections, activeRequests },
      ].slice(-40),
    }))
  }, [channelId, transportQuery.data, transportQuery.dataUpdatedAt])
  const time = useCallback(
    (ts: number) =>
      formatTime?.(ts) ??
      formatPerformanceTimestamp(ts, i18n.resolvedLanguage || i18n.language),
    [formatTime, i18n.language, i18n.resolvedLanguage]
  )
  const updateChannel = useCallback((value: string | null) => {
    const parsed = Number(value)
    setChannelId(
      value &&
        value !== ALL_CHANNELS_VALUE &&
        Number.isInteger(parsed) &&
        parsed > 0
        ? parsed
        : undefined
    )
  }, [])
  const changeRange = useCallback((value: string) => {
    const next = CHANNEL_RANGE_OPTIONS.find(
      (option) => String(option.seconds) === value
    )
    if (!next) return
    setRange(next)
    setCustomRange(undefined)
    setRangeError(false)
    setLive(true)
    setReset((value) => value + 1)
    const end = Math.floor(Date.now() / 1000)
    requestEnd.current = end
    setRangeEnd(end)
  }, [])
  const changeCustomRange = (value: { start?: Date; end?: Date }) => {
    const start = value.start ? dateToUnixTimestamp(value.start) : Number.NaN
    const end = value.end ? dateToUnixTimestamp(value.end) : Number.NaN
    if (
      !Number.isFinite(start) ||
      !Number.isFinite(end) ||
      start <= 0 ||
      end <= start ||
      end - start > 86400
    ) {
      setRangeError(true)
      return
    }
    setCustomRange({ start, end })
    setRangeError(false)
    setLive(false)
    setReset((value) => value + 1)
  }
  const refresh = () => {
    void transportQuery.refetch()
    if (customRange) {
      void query.refetch()
      return
    }
    const end = Math.floor(Date.now() / 1000)
    requestEnd.current = end
    setRangeEnd(end)
    if (end === rangeEnd) void query.refetch()
  }
  const resume = () => {
    changeRange(String(range.seconds))
    if (!customRange) refresh()
  }
  let diagnosticsContent: ReactNode
  if (!data && query.isLoading) {
    diagnosticsContent = (
      <div className='grid gap-3 md:grid-cols-2 xl:grid-cols-3'>
        {Array.from({ length: 12 }, (_, index) => (
          <Skeleton key={index} className='h-80 rounded-xl' />
        ))}
      </div>
    )
  } else if (!data && query.isError) {
    diagnosticsContent = (
      <ErrorState
        title={t('Failed to load performance data')}
        onRetry={refresh}
      />
    )
  } else if (data) {
    diagnosticsContent = (
      <>
        {query.isError && (
          <p role='status' className='text-destructive text-sm'>
            {t(
              'Performance data could not be refreshed. Showing the last result.'
            )}
          </p>
        )}
        {transportQuery.isError && (
          <p role='status' className='text-muted-foreground text-sm'>
            {t(
              'Live transport data could not be refreshed. Showing the last sample.'
            )}
          </p>
        )}
        <ChannelDiagnosticsContent
          data={data}
          formatTime={time}
          transport={transportQuery.data ?? []}
          transportHistory={
            history.channelId === channelId ? history.points : []
          }
        />
      </>
    )
  }
  return (
    <PerformanceChartContext
      value={{
        group: `${group}-${scope}`,
        start: data?.effective_start_timestamp ?? queryParams.start_timestamp,
        end: data?.effective_end_timestamp ?? queryParams.end_timestamp,
        reset,
        onExplore: pause,
      }}
    >
      <div className='space-y-3 sm:space-y-4'>
        <Card size='sm'>
          <CardHeader>
            <CardTitle>{t('Channel transport diagnostics')}</CardTitle>
            <CardDescription>
              {t(
                'Analyze request-stage latency, concurrency, transport pools, and channel errors.'
              )}
            </CardDescription>
          </CardHeader>
          <CardContent className='grid items-end gap-3 md:grid-cols-2 xl:grid-cols-[minmax(12rem,1fr)_auto_minmax(16rem,1fr)_auto]'>
            {channelOptions ? (
              <label className='min-w-0 space-y-1'>
                <span className='text-muted-foreground block text-xs font-medium'>
                  {t('Channel')}
                </span>
                <Select
                  value={channelId ? String(channelId) : ALL_CHANNELS_VALUE}
                  onValueChange={updateChannel}
                >
                  <SelectTrigger className='w-full'>
                    <SelectValue placeholder={t('All channels')}>
                      {channelId
                        ? `${channelOptions.find((channel) => channel.id === channelId)?.name ?? ''} (#${channelId})`
                        : t('All channels')}
                    </SelectValue>
                  </SelectTrigger>
                  <SelectContent align='start'>
                    <SelectGroup>
                      <SelectItem value={ALL_CHANNELS_VALUE}>
                        {t('All channels')}
                      </SelectItem>
                      {channelOptions.map((channel) => (
                        <SelectItem key={channel.id} value={String(channel.id)}>
                          {channel.name} (#{channel.id})
                        </SelectItem>
                      ))}
                    </SelectGroup>
                  </SelectContent>
                </Select>
              </label>
            ) : (
              <label className='min-w-0 space-y-1'>
                <span className='text-muted-foreground block text-xs font-medium'>
                  {t('Channel ID')}
                </span>
                <Input
                  inputMode='numeric'
                  placeholder={t('All channels')}
                  value={channelId ? String(channelId) : ''}
                  onChange={(event) => updateChannel(event.target.value)}
                />
              </label>
            )}
            <div className='space-y-1'>
              <span className='text-muted-foreground block text-xs font-medium'>
                {t('Time range')}
              </span>
              <Tabs
                value={customRange ? 'custom' : String(range.seconds)}
                onValueChange={changeRange}
              >
                <TabsList className='max-w-full flex-wrap group-data-horizontal/tabs:h-auto group-data-horizontal/tabs:min-h-8'>
                  {CHANNEL_RANGE_OPTIONS.map((option) => (
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
                presets={['today']}
                start={
                  customRange ? new Date(customRange.start * 1000) : undefined
                }
                end={customRange ? new Date(customRange.end * 1000) : undefined}
                onChange={changeCustomRange}
                className='h-8 min-w-0 text-xs'
              />
            </div>
            <Button
              variant='outline'
              size='sm'
              className='h-8 justify-self-start'
              onClick={refresh}
              disabled={query.isFetching || transportQuery.isFetching}
            >
              <RefreshCw
                className={query.isFetching ? 'animate-spin' : undefined}
              />
              {t('Refresh')}
            </Button>
            {rangeError && (
              <p
                role='alert'
                className='text-destructive text-sm md:col-span-2 xl:col-span-4'
              >
                {t('Select a valid time range of up to 24 hours.')}
              </p>
            )}
          </CardContent>
        </Card>
        <PerformanceChartToolbar
          live={live}
          onPause={pause}
          onResume={resume}
          onReset={() => setReset((value) => value + 1)}
          updatedAt={query.dataUpdatedAt}
          formatTime={time}
        />
        {diagnosticsContent}
      </div>
    </PerformanceChartContext>
  )
}
