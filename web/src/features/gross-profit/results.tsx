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
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import {
  ChartContainer,
  ChartLegend,
  ChartLegendContent,
  ChartTooltip,
  ChartTooltipContent,
} from '@/components/ui/chart'
import { toIntlLocale } from '@/i18n/languages'
import { formatBillingCurrencyFromUSD } from '@/lib/currency'
import { formatNumber } from '@/lib/format'
import { useSystemConfigStore } from '@/stores/system-config-store'

import type {
  GrossProfitChannel,
  GrossProfitReport,
  GrossProfitUnknown,
} from './types'

export function GrossProfitResults(props: { report: GrossProfitReport }) {
  const { t, i18n } = useTranslation()
  const locale = toIntlLocale(i18n.resolvedLanguage || i18n.language)
  useSystemConfigStore((state) => state.config.currency)
  const money = (value: number) =>
    formatBillingCurrencyFromUSD(value, { locale, abbreviate: false })
  const percent = (value: number | null) =>
    value == null ? '—' : `${formatNumber(value * 100, locale)}%`
  const summary = props.report.summary
  const cards = [
    [t('Known-cost revenue'), money(summary.known_revenue_usd)],
    [t('Upstream cost'), money(summary.cost_usd)],
    [
      t('Known-cost gross profit'),
      summary.known_requests > 0 ? money(summary.profit_usd) : '—',
    ],
    [t('Gross margin'), percent(summary.profit_margin)],
    [t('Revenue coverage'), percent(summary.revenue_coverage)],
  ]
  const columns: StaticDataTableColumn<GrossProfitChannel>[] = [
    { id: 'channel', header: t('Channel'), cell: (row) => row.channel_name },
    {
      id: 'requests',
      header: t('Requests'),
      cell: (row) => formatNumber(row.known_requests, locale),
    },
    {
      id: 'revenue',
      header: t('Known-cost revenue'),
      cell: (row) => money(row.known_revenue_usd),
    },
    {
      id: 'cost',
      header: t('Upstream cost'),
      cell: (row) => money(row.cost_usd),
    },
    {
      id: 'profit',
      header: t('Gross profit'),
      cell: (row) => money(row.profit_usd),
    },
    {
      id: 'margin',
      header: t('Gross margin'),
      cell: (row) => percent(row.profit_margin),
    },
  ]
  const unknownColumns: StaticDataTableColumn<GrossProfitUnknown>[] = [
    { id: 'channel', header: t('Channel'), cell: (row) => row.channel_name },
    {
      id: 'requests',
      header: t('Requests'),
      cell: (row) => formatNumber(row.requests, locale),
    },
    {
      id: 'revenue',
      header: t('Revenue'),
      cell: (row) => money(row.revenue_usd),
    },
    {
      id: 'reason',
      header: t('Reason'),
      cell: (row) => {
        if (row.reason === 'discount_unconfigured') {
          return t('Channel discount not configured')
        }
        if (row.reason === 'snapshot_missing') {
          return t('Historical cost snapshot unavailable')
        }
        if (row.reason === 'discount_unavailable') {
          return t('Channel discount unavailable')
        }
        return t('Billing information unavailable')
      },
    },
    { id: 'profit', header: t('Gross profit'), cell: () => t('Unknown') },
  ]
  return (
    <div className='space-y-4'>
      <div className='grid grid-cols-2 gap-3 lg:grid-cols-5'>
        {cards.map(([label, value]) => (
          <Card key={label} size='sm'>
            <CardHeader>
              <CardTitle className='text-muted-foreground text-sm'>
                {label}
              </CardTitle>
            </CardHeader>
            <CardContent className='text-xl font-semibold break-words tabular-nums'>
              {value}
            </CardContent>
          </Card>
        ))}
      </div>
      <p className='text-muted-foreground text-sm'>
        {t(
          'Gross profit covers only calls with a known cost. Revenue coverage is their share of total call revenue.'
        )}
      </p>
      <Card>
        <CardHeader>
          <CardTitle>{t('Gross profit trend')}</CardTitle>
        </CardHeader>
        <CardContent>
          <ChartContainer
            className='h-64 w-full'
            config={{
              profit_usd: { label: t('Gross profit'), color: 'var(--chart-1)' },
              known_revenue_usd: {
                label: t('Known-cost revenue'),
                color: 'var(--chart-2)',
              },
            }}
          >
            <LineChart
              accessibilityLayer
              data={props.report.series.map((point) => ({
                ...point,
                profit_usd: point.known_requests > 0 ? point.profit_usd : null,
              }))}
            >
              <CartesianGrid vertical={false} />
              <XAxis
                dataKey='day'
                tickFormatter={(value: string) => value.slice(5)}
              />
              <YAxis
                width={80}
                tickFormatter={(value: number) => money(value)}
              />
              <ChartTooltip
                content={
                  <ChartTooltipContent
                    labelFormatter={(value) => String(value)}
                    formatter={(value, name) => (
                      <span>
                        {name === 'profit_usd'
                          ? t('Gross profit')
                          : t('Known-cost revenue')}
                        : {money(Number(value))}
                      </span>
                    )}
                  />
                }
              />
              <Line
                type='monotone'
                dataKey='known_revenue_usd'
                stroke='var(--color-known_revenue_usd)'
                dot={false}
                isAnimationActive={false}
              />
              <Line
                type='monotone'
                dataKey='profit_usd'
                stroke='var(--color-profit_usd)'
                dot={false}
                isAnimationActive={false}
              />
              <ChartLegend content={<ChartLegendContent />} />
            </LineChart>
          </ChartContainer>
        </CardContent>
      </Card>
      <Card>
        <CardHeader>
          <CardTitle>{t('Gross profit by channel')}</CardTitle>
        </CardHeader>
        <CardContent>
          <StaticDataTable
            columns={columns}
            data={props.report.channels}
            getRowKey={(row) => row.channel_id}
            emptyContent={t('No calls with a known cost in this period')}
          />
        </CardContent>
      </Card>
      <Card>
        <CardHeader>
          <CardTitle>{t('Unknown-cost calls')}</CardTitle>
          <p className='text-muted-foreground text-sm'>
            {t('Revenue')}: {money(summary.unknown_revenue_usd)} ·{' '}
            {t('Requests')}: {formatNumber(summary.unknown_requests, locale)}
          </p>
        </CardHeader>
        <CardContent>
          <StaticDataTable
            columns={unknownColumns}
            data={props.report.unknown}
            getRowKey={(row) => `${row.channel_id}:${row.reason}`}
            emptyContent={t('No unknown-cost calls in this period')}
          />
        </CardContent>
      </Card>
    </div>
  )
}
