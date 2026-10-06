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

import { Button } from '@/components/ui/button'
import { toIntlLocale } from '@/i18n/languages'
import { formatNumber } from '@/lib/format'

import {
  getApiKeyModelPrices,
  type ApiKeyPricingSource,
} from '../lib/api-key-pricing'
import type { ApiKey } from '../types'
import { useApiKeys } from './api-keys-provider'

type ApiKeyPricingCellProps = {
  apiKey: ApiKey
  pricing: ApiKeyPricingSource
  isLoading: boolean
  error: unknown
}

export function ApiKeyPricingCell(props: ApiKeyPricingCellProps) {
  const { t, i18n } = useTranslation()
  const { setCurrentRow, setOpen } = useApiKeys()
  const count = getApiKeyModelPrices(props.apiKey, props.pricing).filter(
    (entry) => entry.specialGroups.length > 0
  ).length
  const locale = toIntlLocale(i18n.resolvedLanguage || i18n.language)
  return (
    <div className='flex min-w-0 flex-col items-start gap-1'>
      <Button
        variant='link'
        size='sm'
        className='h-auto max-w-full px-0 text-left whitespace-normal'
        onClick={() => {
          setCurrentRow(props.apiKey)
          setOpen('pricing')
        }}
      >
        {t('Model prices and discounts')}
      </Button>
      {!props.isLoading && !props.error && count > 0 && (
        <span className='text-muted-foreground text-xs'>
          {t('Special model prices: {{count}}', {
            count: formatNumber(count, locale),
          })}
        </span>
      )}
    </div>
  )
}
