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
import { BarChart3, PieChart as PieChartIcon, RefreshCw } from 'lucide-react'
import { useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import {
  Bar,
  BarChart,
  CartesianGrid,
  Cell,
  Pie,
  PieChart,
  XAxis,
  YAxis,
} from 'recharts'

import {
  StaticDataTable,
  type StaticDataTableColumn,
} from '@/components/data-table'
import { EmptyState } from '@/components/empty-state'
import { ErrorState } from '@/components/error-state'
import { Button } from '@/components/ui/button'
import {
  ChartContainer,
  ChartTooltip,
  ChartTooltipContent,
} from '@/components/ui/chart'
import { ToggleGroup, ToggleGroupItem } from '@/components/ui/toggle-group'
import { toIntlLocale } from '@/i18n/languages'
import { formatCompactNumber, formatNumber } from '@/lib/format'
import { ROLE } from '@/lib/roles'
import { requireServerSuccess } from '@/lib/server-error-message'
import { computeTimeRange } from '@/lib/time'
import { useAuthStore } from '@/stores/auth-store'

import { getStatusCodeStats } from '../../api'
import { getDefaultDays } from '../../lib/filters'
import type { DashboardFilters } from '../../types'
import { PanelWrapper } from '../ui/panel-wrapper'

const STATUS_COLORS = [
  'var(--muted-foreground)',
  'var(--chart-1)',
  'var(--chart-2)',
  'var(--chart-3)',
  'var(--chart-4)',
  'var(--destructive)',
]

type StatusChartRow = {
  statusCode: number
  label: string
  count: number
  fill: string
}

export function StatusCodeDistribution(props: { filters: DashboardFilters }) {
  const { t, i18n } = useTranslation()
  const locale = toIntlLocale(i18n.resolvedLanguage || i18n.language)
  const isAdmin = useAuthStore(
    (state) => (state.auth.user?.role ?? 0) >= ROLE.ADMIN
  )
  const [chartType, setChartType] = useState<'bar' | 'pie'>('bar')
  const params = useMemo(
    () => ({
      ...computeTimeRange(
        getDefaultDays(props.filters.time_granularity),
        props.filters.start_timestamp,
        props.filters.end_timestamp
      ),
      ...(props.filters.username && { username: props.filters.username }),
    }),
    [
      props.filters.start_timestamp,
      props.filters.end_timestamp,
      props.filters.time_granularity,
      props.filters.username,
    ]
  )
  const query = useQuery({
    queryKey: ['dashboard-status-codes', isAdmin, params],
    queryFn: async ({ signal }) =>
      requireServerSuccess(await getStatusCodeStats(params, signal)).data,
    enabled: isAdmin,
    refetchInterval: 60_000,
    staleTime: 30_000,
    retry: false,
    meta: { errorToast: false },
  })
  const stats = query.data
  const total = stats?.total ?? 0
  const rows = useMemo<StatusChartRow[]>(() => {
    const values = (stats?.items ?? []).map((item) => ({
      statusCode: item.status_code,
      label: `HTTP ${item.status_code}`,
      count: item.count,
      fill: STATUS_COLORS[Math.floor(item.status_code / 100)],
    }))
    if (stats?.unrecorded) {
      values.push({
        statusCode: 0,
        label: t('Not recorded'),
        count: stats.unrecorded,
        fill: STATUS_COLORS[0],
      })
    }
    return values
  }, [stats, t])
  const columns: StaticDataTableColumn<StatusChartRow>[] = [
    {
      id: 'status',
      header: t('Status Code'),
      cell: (row) => (
        <span className='inline-flex items-center gap-2'>
          <span
            className='size-2 shrink-0 rounded-full'
            style={{ background: row.fill }}
            aria-hidden='true'
          />
          {row.label}
        </span>
      ),
    },
    {
      id: 'count',
      header: t('Log records'),
      className: 'text-right',
      cell: (row) => formatNumber(row.count, locale),
    },
    {
      id: 'share',
      header: t('Percentage:'),
      className: 'text-right',
      cell: (row) =>
        `${formatNumber(total > 0 ? (row.count / total) * 100 : 0, locale)}%`,
    },
  ]
  const title = t('HTTP status code distribution')
  const chartLabel = chartType === 'bar' ? t('Bar Chart') : t('Pie Chart')
  const chartConfig = {
    count: { label: t('Log records'), color: 'var(--chart-1)' },
  }

  if (!isAdmin) return null

  return (
    <PanelWrapper
      title={<h3>{title}</h3>}
      description={t(
        'Consumption and error log records. Retries count separately; unrecorded status codes are not treated as HTTP 200.'
      )}
      loading={query.isLoading}
      height='h-80'
      headerActions={
        <div className='flex shrink-0 items-center gap-2'>
          <ToggleGroup
            value={[chartType]}
            onValueChange={(values) => {
              if (values[0] === 'bar' || values[0] === 'pie') {
                setChartType(values[0])
              }
            }}
            variant='outline'
            size='sm'
            aria-label={title}
          >
            <ToggleGroupItem
              value='bar'
              aria-label={t('Bar Chart')}
              title={t('Bar Chart')}
            >
              <BarChart3 className='size-4' />
            </ToggleGroupItem>
            <ToggleGroupItem
              value='pie'
              aria-label={t('Pie Chart')}
              title={t('Pie Chart')}
            >
              <PieChartIcon className='size-4' />
            </ToggleGroupItem>
          </ToggleGroup>
          <Button
            variant='outline'
            size='icon'
            className='size-8'
            aria-label={t('Refresh')}
            title={t('Refresh')}
            disabled={query.isFetching}
            onClick={() => void query.refetch()}
          >
            <RefreshCw className='size-4' />
          </Button>
        </div>
      }
    >
      {query.isError && (
        <ErrorState
          title={t('Failed to load status code distribution')}
          onRetry={() => void query.refetch()}
        />
      )}
      {!query.isError && total === 0 && (
        <EmptyState title={t('No data available')} />
      )}
      {!query.isError && total > 0 && (
        <>
          <div className='text-muted-foreground mb-3 flex flex-wrap gap-x-4 gap-y-1 text-xs'>
            <span>
              {t('Log records')}: {formatNumber(total, locale)}
            </span>
            <span>
              {t('Not recorded')}:{' '}
              {formatNumber(stats?.unrecorded ?? 0, locale)}
            </span>
          </div>
          <div className='grid min-w-0 gap-4 lg:grid-cols-[minmax(0,1fr)_minmax(16rem,22rem)]'>
            <div className='max-h-80 min-w-0 overflow-y-auto'>
              <ChartContainer
                config={chartConfig}
                className='aspect-auto w-full'
                style={{
                  height:
                    chartType === 'bar' ? Math.max(300, rows.length * 32) : 300,
                }}
                role='region'
                aria-label={`${title} — ${chartLabel}`}
              >
                {chartType === 'bar' ? (
                  <BarChart
                    accessibilityLayer
                    data={rows}
                    layout='vertical'
                    margin={{ top: 8, right: 16, bottom: 8, left: 0 }}
                  >
                    <CartesianGrid horizontal={false} />
                    <XAxis
                      type='number'
                      allowDecimals={false}
                      tickFormatter={(value: number) =>
                        formatCompactNumber(value, locale)
                      }
                    />
                    <YAxis
                      dataKey='label'
                      type='category'
                      width={110}
                      tickLine={false}
                      axisLine={false}
                    />
                    <ChartTooltip
                      content={
                        <ChartTooltipContent
                          formatter={(value) => (
                            <span>
                              {t('Log records')}:{' '}
                              {formatNumber(Number(value), locale)}
                            </span>
                          )}
                        />
                      }
                    />
                    <Bar dataKey='count' isAnimationActive={false} radius={4}>
                      {rows.map((row) => (
                        <Cell key={row.statusCode} fill={row.fill} />
                      ))}
                    </Bar>
                  </BarChart>
                ) : (
                  <PieChart accessibilityLayer>
                    <ChartTooltip
                      content={
                        <ChartTooltipContent
                          hideLabel
                          formatter={(value, name) => (
                            <span>
                              {String(name)}:{' '}
                              {formatNumber(Number(value), locale)}
                            </span>
                          )}
                        />
                      }
                    />
                    <Pie
                      data={rows}
                      dataKey='count'
                      nameKey='label'
                      innerRadius='45%'
                      outerRadius='80%'
                      stroke='var(--background)'
                      strokeWidth={2}
                      isAnimationActive={false}
                    >
                      {rows.map((row) => (
                        <Cell key={row.statusCode} fill={row.fill} />
                      ))}
                    </Pie>
                  </PieChart>
                )}
              </ChartContainer>
            </div>
            <StaticDataTable
              className='max-h-80 overflow-y-auto'
              tableProps={{ 'aria-label': title }}
              columns={columns}
              data={rows}
              getRowKey={(row) => row.statusCode}
            />
          </div>
        </>
      )}
    </PanelWrapper>
  )
}
