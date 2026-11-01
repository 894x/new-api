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
import { cleanup, render, screen, within } from '@testing-library/react'
import { createInstance } from 'i18next'
import { I18nextProvider } from 'react-i18next'
import { afterEach, expect, it, vi } from 'vitest'

import { Accordion } from '@/components/ui/accordion'
import type { PricingModel } from '@/features/pricing/types'

import type { useApiKeyPricingData } from '../../hooks/use-api-key-pricing'
import { ApiKeyModelPriceItem } from '../api-key-model-price-item'

vi.mock('@visactor/react-vchart', () => ({ VChart: () => null }))

const i18n = createInstance()
await i18n.init({
  lng: 'en',
  resources: { en: { translation: {} } },
  initAsync: false,
})

const pricing = {
  usableGroup: { default: { desc: 'Default', ratio: 1 } },
  groupRatio: { default: 1 },
  priceRate: 1,
  usdExchangeRate: 7.3,
} as unknown as ReturnType<typeof useApiKeyPricingData>

function renderPrice(model: PricingModel) {
  return render(
    <I18nextProvider i18n={i18n}>
      <Accordion defaultValue={[model.model_name]}>
        <ApiKeyModelPriceItem
          entry={{ model, groups: ['default'], specialGroups: [] }}
          pricing={pricing}
          ratioFormatter={new Intl.NumberFormat('en')}
        />
      </Accordion>
    </I18nextProvider>
  )
}

afterEach(cleanup)

it('shows time-based discounts alongside the model and group prices', () => {
  renderPrice({
    id: 1,
    model_name: 'deepseek-v4.1-flash-1017',
    quota_type: 0,
    model_ratio: 1,
    completion_ratio: 1,
    enable_groups: ['default'],
    billing_mode: 'tiered_expr',
    billing_expr:
      '(tier("base", p * 0.27397260274 + c * 1.095890410959 + cr * 0.005479452055)) * (hour("Asia/Shanghai") >= 0 && hour("Asia/Shanghai") < 9 && weekday("Asia/Shanghai") >= 1 && weekday("Asia/Shanghai") < 6 ? 0.5 : 1) * (weekday("Asia/Shanghai") == 0 ? 0.5 : 1)',
  })

  expect(screen.getByText('Base Price')).toBeVisible()
  expect(screen.getByText('Pricing by Group')).toBeVisible()
  expect(screen.getByText('Conditional multipliers')).toBeVisible()
  const rules = screen.getByText('Conditional multipliers').parentElement
  if (!rules) throw new Error('Conditional multiplier rules are missing')
  expect(within(rules).getAllByText('0.5x')).toHaveLength(2)
})

it('shows pricing tier details for models with context-length tiers', () => {
  renderPrice({
    id: 2,
    model_name: 'tiered-model',
    quota_type: 0,
    model_ratio: 1,
    completion_ratio: 1,
    enable_groups: ['default'],
    billing_mode: 'tiered_expr',
    billing_expr:
      'len <= 32000 ? tier("short", p * 2 + c * 8) : tier("long", p * 4 + c * 16)',
  })

  expect(screen.getByText('Pricing by Group')).toBeVisible()
  expect(screen.getByText('Tiered price table')).toBeVisible()
})
