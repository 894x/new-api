import { useTranslation } from 'react-i18next'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import type {
  ChannelTransportGroup,
  ChannelAnalyticsPercentiles,
} from './types'

export function TransportComparison({
  groups,
}: {
  groups: ChannelTransportGroup[]
}) {
  const { t } = useTranslation()
  const headers = [
    t('Channel ID'),
    t('Protocol'),
    t('Request body bytes'),
    t('Routing reason'),
    t('Threshold bytes'),
    t('Requests'),
    t('Errors'),
    t('Canceled'),
    t('Timeouts'),
    t('Connection reused'),
    t('Connection acquisition (ms)'),
    t('Write after acquisition (ms)'),
  ]
  if (!groups.length) return null
  const percentiles = (value: ChannelAnalyticsPercentiles) =>
    value.sample_count
      ? `${value.p50_ms} / ${value.p95_ms} / ${value.p99_ms} (${value.sample_count})`
      : '—'
  return (
    <Card>
      <CardHeader>
        <CardTitle>{t('Protocol and body size comparison')}</CardTitle>
        <CardDescription>
          {t(
            'P50 / P95 / P99 in milliseconds (sample count). Compare matching body-size groups; transport errors include cancellation and timeouts.'
          )}
        </CardDescription>
      </CardHeader>
      <CardContent className='overflow-x-auto'>
        <table className='w-full text-left text-xs'>
          <thead>
            <tr>
              {headers.map((label) => (
                <th key={label} className='whitespace-nowrap p-2'>
                  {label}
                </th>
              ))}
            </tr>
          </thead>
          <tbody>
            {groups.map((g) => (
              <tr
                key={`${g.channel_id}/${g.protocol}/${g.body_class}/${g.reason}/${g.threshold_bytes}`}
                className='border-t'
              >
                {[
                  g.channel_id || '—',
                  g.protocol || '—',
                  g.body_class,
                  g.reason,
                  g.threshold_bytes || '—',
                  g.count,
                  g.errors,
                  g.canceled,
                  g.timeouts,
                  g.reused,
                  percentiles(g.acquire_ms),
                  percentiles(g.write_ms),
                ].map((value, cell) => (
                  <td
                    key={headers[cell]}
                    className='whitespace-nowrap p-2 tabular-nums'
                  >
                    {value}
                  </td>
                ))}
              </tr>
            ))}
          </tbody>
        </table>
      </CardContent>
    </Card>
  )
}
