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
import type { ReactElement } from 'react'
import { useTranslation } from 'react-i18next'

import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import { applyRechargeRate } from '@/features/pricing/lib/price'
import { formatBillingCurrencyFromUSD } from '@/lib/currency'

import type { TokenCostEstimate, TokenLanePrices } from '../lib'
import { Metric } from './Fields'

type PriceBreakdownProps = {
  dynamic: boolean
  totalTokens: number
  estimate: TokenCostEstimate
  lanePrices: TokenLanePrices
  priceRate: number
  usdExchangeRate: number
}

const FORMAT_OPTIONS = {
  digitsLarge: 4,
  digitsSmall: 6,
  abbreviate: false,
} as const

export function PriceBreakdown(props: PriceBreakdownProps): ReactElement {
  const { t } = useTranslation()

  if (props.dynamic) {
    const effectiveRate =
      props.totalTokens > 0
        ? (props.estimate.customerCostUSD * 1_000_000) / props.totalTokens
        : 0

    return (
      <Card>
        <CardHeader>
          <CardTitle>{t('Customer price breakdown')}</CardTitle>
          <CardDescription>
            {t(
              'Calculated from the current token mix and matched pricing tier.'
            )}
          </CardDescription>
        </CardHeader>
        <CardContent className='grid gap-3 sm:grid-cols-2'>
          <Metric
            label={t('Effective price per 1M tokens')}
            value={
              props.estimate.error
                ? '-'
                : formatBillingCurrencyFromUSD(effectiveRate, FORMAT_OPTIONS)
            }
          />
          <Metric
            label={t('Matched tier')}
            value={props.estimate.matchedTier || t('Dynamic pricing')}
          />
        </CardContent>
      </Card>
    )
  }

  const formatLanePrice = (price: number): string =>
    formatBillingCurrencyFromUSD(
      applyRechargeRate(price, true, props.priceRate, props.usdExchangeRate),
      FORMAT_OPTIONS
    )

  return (
    <Card>
      <CardHeader>
        <CardTitle>{t('Customer price breakdown')}</CardTitle>
        <CardDescription>
          {t('Per 1M tokens at the selected pricing group.')}
        </CardDescription>
      </CardHeader>
      <CardContent className='grid gap-3 sm:grid-cols-2 lg:grid-cols-4'>
        <Metric
          label={t('Input')}
          value={formatLanePrice(props.lanePrices.input)}
        />
        <Metric
          label={t('Output')}
          value={formatLanePrice(props.lanePrices.output)}
        />
        <Metric
          label={t('Cache read')}
          value={formatLanePrice(props.lanePrices.cacheRead)}
        />
        <Metric
          label={t('Cache write')}
          value={formatLanePrice(props.lanePrices.cacheWrite)}
        />
      </CardContent>
    </Card>
  )
}
