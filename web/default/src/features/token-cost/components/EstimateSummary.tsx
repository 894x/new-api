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
import { formatBillingCurrencyFromUSD } from '@/lib/currency'

import { formatTokenCount, type TokenCostEstimate } from '../lib'
import { Metric, NumberField } from './Fields'

type EstimateSummaryProps = {
  totalTokens: number
  budget: number
  purchasableTokens: number
  estimate: TokenCostEstimate
  onBudgetChange: (value: string) => void
}

export function EstimateSummary(props: EstimateSummaryProps): ReactElement {
  const { t } = useTranslation()

  return (
    <Card className='border-primary/25 bg-primary/[0.03]'>
      <CardHeader>
        <CardTitle>{t('Estimated customer payment')}</CardTitle>
        <CardDescription>
          {t('For the token volume and usage mix above.')}
        </CardDescription>
      </CardHeader>
      <CardContent className='space-y-5'>
        <div>
          <div className='text-primary text-4xl font-semibold tracking-tight'>
            {props.estimate.error
              ? '-'
              : formatBillingCurrencyFromUSD(props.estimate.customerCostUSD, {
                  digitsLarge: 4,
                  digitsSmall: 6,
                  abbreviate: false,
                })}
          </div>
          <p className='text-muted-foreground mt-1 text-sm'>
            {formatTokenCount(props.totalTokens)} {t('tokens')}
          </p>
        </div>
        <div className='grid grid-cols-2 gap-3 text-sm'>
          <Metric
            label={t('Regular input')}
            value={formatTokenCount(props.estimate.regularInputTokens)}
          />
          <Metric
            label={t('Cache read')}
            value={formatTokenCount(props.estimate.cacheReadTokens)}
          />
          <Metric
            label={t('Cache write')}
            value={formatTokenCount(props.estimate.cacheWriteTokens)}
          />
          <Metric
            label={t('Output')}
            value={formatTokenCount(props.estimate.outputTokens)}
          />
        </div>
        {props.estimate.error && (
          <p className='text-destructive text-sm'>
            {t("Unable to calculate this model's dynamic price.")}
          </p>
        )}
        {props.estimate.hasRequestRules && (
          <p className='text-muted-foreground text-sm'>
            {t('Request-specific pricing rules are not included.')}
          </p>
        )}
        <div className='border-t pt-4'>
          <NumberField
            label={t('Budget')}
            value={props.budget}
            onChange={props.onBudgetChange}
          />
          <p className='text-muted-foreground mt-3 text-sm'>
            {t('This budget buys approximately {{tokens}} tokens.', {
              tokens: formatTokenCount(props.purchasableTokens),
            })}
          </p>
        </div>
      </CardContent>
    </Card>
  )
}
