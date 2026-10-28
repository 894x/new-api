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

import {
  Card,
  CardContent,
  CardHeader,
  CardTitle,
  CardDescription,
} from '@/components/ui/card'
import {
  Table,
  TableHeader,
  TableBody,
  TableRow,
  TableHead,
  TableCell,
} from '@/components/ui/table'
import { toIntlLocale } from '@/i18n/languages'
import { formatNumber } from '@/lib/format'

import {
  ChannelLatencyChart,
  ChannelActivityCharts,
  type TransportHistoryPoint,
} from './channel-charts'
import { formatPerformanceLatency } from './lib'
import { TransportComparison } from './transport-comparison'
import type { ChannelAnalyticsData, HTTPTransportMetric } from './types'

export function ChannelDiagnosticsContent(props: {
  data: ChannelAnalyticsData
  formatTime: (ts: number) => string
  transport: HTTPTransportMetric[]
  transportHistory: TransportHistoryPoint[]
}) {
  const { t, i18n } = useTranslation()
  const locale = toIntlLocale(i18n.resolvedLanguage || i18n.language)
  const summary = props.data.summary
  const total = summary.latency.total_ms
  const cards = [
    [t('Requests'), formatNumber(summary.request_count, locale)],
    [t('Errors'), formatNumber(summary.error_count, locale)],
    [
      t('Average concurrency'),
      formatNumber(summary.active_concurrency.average, locale),
    ],
    [
      t('Peak concurrency'),
      formatNumber(summary.active_concurrency.maximum, locale),
    ],
    [
      t('Average total latency'),
      total.avg_ms == null
        ? t('Unknown')
        : formatPerformanceLatency(total.avg_ms, locale),
    ],
    [
      t('Total p95'),
      total.sample_count
        ? formatPerformanceLatency(total.p95_ms, locale)
        : t('Unknown'),
    ],
    [
      t('Maximum total latency'),
      total.max_ms == null
        ? t('Unknown')
        : formatPerformanceLatency(total.max_ms, locale),
    ],
  ]
  const phases = [
    ['total_ms', t('Total request latency')],
    ['provider_wait_ms', t('Provider wait')],
    ['downstream_ms', t('Downstream time')],
    ['body_read_ms', t('Request body read')],
    ['upstream_queue_ms', t('Upstream queue')],
    ['upload_ms', t('Upload time')],
    ['acquire_ms', t('Connection acquisition')],
    ['write_ms', t('Request write')],
    ['headers_to_first_response_ms', t('Headers to first response')],
  ] as const
  return (
    <>
      <p className='text-muted-foreground text-xs'>
        {t('Time range')}:{' '}
        {props.formatTime(props.data.effective_start_timestamp)} –{' '}
        {props.formatTime(props.data.effective_end_timestamp)}
      </p>
      <p className='text-muted-foreground text-xs'>
        {t(
          'Historical concurrency is reconstructed from completed log records; unfinished requests and truncated scans are not included. Percentiles are approximate histogram upper bounds.'
        )}
      </p>
      {props.data.truncated && (
        <p role='alert' className='text-destructive text-sm'>
          {t(
            'Scan limit reached. These are partial statistics; select a shorter range or one channel.'
          )}
        </p>
      )}
      <div className='grid grid-cols-2 gap-2 sm:grid-cols-3 lg:grid-cols-4 xl:grid-cols-7'>
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
      <div
        role='group'
        aria-label={t('Request-stage latency')}
        className='grid gap-3 md:grid-cols-2 xl:grid-cols-3'
      >
        {phases.map(([metric, title]) => (
          <ChannelLatencyChart
            key={metric}
            title={title}
            metric={metric}
            points={props.data.series}
            summary={summary.latency[metric]}
            formatTime={props.formatTime}
          />
        ))}
      </div>
      <ChannelActivityCharts
        data={props.data}
        history={props.transportHistory}
        formatTime={props.formatTime}
      />
      <p className='text-muted-foreground text-xs'>
        {t(
          'Live curves are browser-session samples. Connections cover all shared pools; active requests follow the selected channel and include streaming response lifetime.'
        )}
      </p>
      <ChannelErrorSummary groups={props.data.error_groups} locale={locale} />
      <TransportPoolTable metrics={props.transport} />
      <TransportComparison groups={props.data.transport_groups ?? []} />
    </>
  )
}

function ChannelErrorSummary({
  groups,
  locale,
}: {
  groups: ChannelAnalyticsData['error_groups']
  locale?: Intl.LocalesArgument
}) {
  const { t } = useTranslation()
  return (
    <Card>
      <CardHeader>
        <CardTitle>{t('Error breakdown')}</CardTitle>
        <CardDescription>
          {t('Aggregated by error code, error type, and HTTP status.')}
        </CardDescription>
      </CardHeader>
      <CardContent>
        {groups.length === 0 ? (
          <div className='text-muted-foreground py-6 text-center text-sm'>
            {t('No channel errors in this time range')}
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
              <th className='pr-4 pb-2'>{t('Pool')}</th>
              <th className='pr-4 pb-2'>{t('Protocol')}</th>
              <th className='pr-4 pb-2'>{t('Shard')}</th>
              <th className='pr-4 pb-2'>{t('Connections')}</th>
              <th className='pr-4 pb-2'>{t('Active requests')}</th>
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
