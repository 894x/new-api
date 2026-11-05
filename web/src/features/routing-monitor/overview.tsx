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

import { Card, CardContent } from '@/components/ui/card'
import { Skeleton } from '@/components/ui/skeleton'
import { toIntlLocale } from '@/i18n/languages'
import { formatNumber } from '@/lib/format'
import { cn } from '@/lib/utils'

import type { MonitorRoute } from './api'
import { monitorChannelStatus, weightedDiscount } from './route-data'

export function RoutingOverview(props: {
  route?: MonitorRoute
  at: number
  names?: Map<number, string>
  loading: boolean
  embedded?: boolean
}) {
  const { t, i18n } = useTranslation()
  const locale = toIntlLocale(i18n.resolvedLanguage || i18n.language)
  const channels =
    props.route?.channels.filter((channel) => channel.active) ?? []
  const primary = channels.reduce<(typeof channels)[number] | undefined>(
    (current, channel) =>
      !current || channel.target_share > current.target_share
        ? channel
        : current,
    undefined
  )
  const discount = props.route ? weightedDiscount(props.route.channels) : null
  const healthy = channels.filter((channel) =>
    ['healthy', 'probe'].includes(monitorChannelStatus(channel, props.at))
  ).length
  return (
    <section
      aria-label={t('Routing overview')}
      aria-busy={props.loading}
      data-routing-monitor-overview=''
      className={cn(
        'grid min-w-0 grid-cols-2 gap-3',
        props.embedded
          ? 'order-1 self-start xl:col-start-1 xl:row-start-1'
          : 'xl:grid-cols-4'
      )}
    >
      <Card size='sm'>
        <CardContent>
          <p className='text-muted-foreground text-xs'>
            {t('Routing selections')}
          </p>
          {props.loading ? (
            <Skeleton className='my-2 h-7 w-20' />
          ) : (
            <p className='mt-2 text-2xl font-medium tabular-nums'>
              {props.route
                ? formatNumber(props.route.selection_count, locale)
                : '—'}
            </p>
          )}
          <p className='text-muted-foreground text-xs'>
            {t('Last {{seconds}} seconds', {
              seconds: props.route?.window_seconds ?? 60,
            })}
          </p>
        </CardContent>
      </Card>
      <Card size='sm'>
        <CardContent>
          <p className='text-muted-foreground text-xs'>
            {t('Primary destination')}
          </p>
          {props.loading ? (
            <Skeleton className='my-2 h-7 w-24' />
          ) : (
            <p className='mt-2 text-lg font-medium break-all'>
              {primary
                ? props.names?.get(primary.channel_id) ||
                  `#${primary.channel_id}`
                : '—'}
            </p>
          )}
          <p className='text-muted-foreground text-xs tabular-nums'>
            {primary
              ? `${formatNumber(primary.target_share * 100, locale)}%`
              : '—'}{' '}
            · {t('Target share')}
          </p>
        </CardContent>
      </Card>
      <Card size='sm'>
        <CardContent>
          <p className='text-muted-foreground text-xs'>
            {t('Target weighted discount')}
          </p>
          {props.loading ? (
            <Skeleton className='my-2 h-7 w-20' />
          ) : (
            <p className='mt-2 text-2xl font-medium tabular-nums'>
              {discount == null ? '—' : `×${formatNumber(discount, locale)}`}
            </p>
          )}
          <p className='text-muted-foreground text-xs'>
            {t('Based on effective discounts and target shares')}
          </p>
        </CardContent>
      </Card>
      <Card size='sm'>
        <CardContent>
          <p className='text-muted-foreground text-xs'>
            {t('Verified healthy channels')}
          </p>
          {props.loading ? (
            <Skeleton className='my-2 h-7 w-20' />
          ) : (
            <p className='mt-2 text-2xl font-medium tabular-nums'>
              {props.route ? formatNumber(healthy, locale) : '—'}{' '}
              <span className='text-muted-foreground text-sm'>
                / {props.route ? formatNumber(channels.length, locale) : '—'}
              </span>
            </p>
          )}
          <p className='text-muted-foreground text-xs'>
            {t('Quality samples are shared by channel and model')}
          </p>
        </CardContent>
      </Card>
    </section>
  )
}
