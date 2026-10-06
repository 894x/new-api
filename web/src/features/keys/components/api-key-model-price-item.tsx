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
  AccordionContent,
  AccordionItem,
  AccordionTrigger,
} from '@/components/ui/accordion'
import { Badge } from '@/components/ui/badge'
import {
  GroupPricingSection,
  PriceSection,
} from '@/features/pricing/components/model-details'
import { DEFAULT_TOKEN_UNIT } from '@/features/pricing/constants'
import { getConfiguredGroupRatio } from '@/features/pricing/lib/model-helpers'

import type { useApiKeyPricingData } from '../hooks/use-api-key-pricing'
import type { ApiKeyModelPrice } from '../lib/api-key-pricing'

type ApiKeyModelPriceItemProps = {
  entry: ApiKeyModelPrice
  pricing: ReturnType<typeof useApiKeyPricingData>
  ratioFormatter: Intl.NumberFormat
}

export function ApiKeyModelPriceItem(props: ApiKeyModelPriceItemProps) {
  const { t } = useTranslation()
  const usableGroup = Object.fromEntries(
    props.entry.groups.map((group) => [group, props.pricing.usableGroup[group]])
  )
  const groupRatio = props.entry.model.group_ratio || props.pricing.groupRatio
  return (
    <AccordionItem value={props.entry.model.model_name}>
      <AccordionTrigger className='gap-3 py-4'>
        <span className='flex min-w-0 flex-1 flex-col gap-2'>
          <span className='font-mono break-all'>
            {props.entry.model.model_name}
          </span>
          <span className='flex flex-wrap gap-1.5'>
            {props.entry.groups.map((group) => {
              const ratio = getConfiguredGroupRatio(groupRatio, group)
              const special = props.entry.specialGroups.includes(group)
              return (
                <Badge
                  key={group}
                  variant={special ? 'secondary' : 'outline'}
                  className='h-auto max-w-full break-all whitespace-normal'
                >
                  {group} · {props.ratioFormatter.format(ratio)}x ·{' '}
                  {special ? t('Model-specific ratio') : t('Group ratio')}
                  {special && Number.isFinite(ratio * 100) && (
                    <span className='ml-1'>
                      {t('Pay {{percent}}% of base price', {
                        percent: props.ratioFormatter.format(ratio * 100),
                      })}
                    </span>
                  )}
                </Badge>
              )
            })}
          </span>
        </span>
      </AccordionTrigger>
      <AccordionContent className='space-y-5 px-1 pb-5'>
        <PriceSection
          model={props.entry.model}
          priceRate={props.pricing.priceRate}
          usdExchangeRate={props.pricing.usdExchangeRate}
          tokenUnit={DEFAULT_TOKEN_UNIT}
          showRechargePrice={false}
        />
        <GroupPricingSection
          model={props.entry.model}
          groupRatio={groupRatio}
          usableGroup={usableGroup}
          autoGroups={[]}
          priceRate={props.pricing.priceRate}
          usdExchangeRate={props.pricing.usdExchangeRate}
          tokenUnit={DEFAULT_TOKEN_UNIT}
        />
      </AccordionContent>
    </AccordionItem>
  )
}
