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
import { describe, expect, it } from 'vitest'

import {
  getPriceDetail,
  getPriceSummary,
  type ModelPricingSnapshot,
} from './model-pricing-snapshots'

const t = (key: string) => key
const cnyDisplay = { currencySymbol: '¥', exchangeRate: 7.3 }

describe('model pricing list currency display', () => {
  it('keeps USD summaries as the default', () => {
    const row: ModelPricingSnapshot = {
      name: 'usd-request-model',
      billingMode: 'per-request',
      price: '10',
      hasConflict: false,
    }

    expect(getPriceSummary(row, t)).toBe('$10 / request')
  })

  it('converts per-request summaries from stored USD to CNY', () => {
    const row: ModelPricingSnapshot = {
      name: 'cny-request-model',
      billingMode: 'per-request',
      price: '10',
      hasConflict: false,
    }

    expect(getPriceSummary(row, t, cnyDisplay)).toBe('¥73 / request')
  })

  it('converts token summaries and lane details to CNY', () => {
    const row: ModelPricingSnapshot = {
      name: 'cny-token-model',
      billingMode: 'per-token',
      ratio: '5',
      completionRatio: '2',
      cacheRatio: '0.1',
      hasConflict: false,
    }

    expect(getPriceSummary(row, t, cnyDisplay)).toBe('Input ¥73 · 2 extras')
    expect(getPriceDetail(row, t, cnyDisplay)).toBe('Output ¥146 · Cache ¥7.3')
  })

  it('does not mislabel an invalid CNY conversion as USD', () => {
    const row: ModelPricingSnapshot = {
      name: 'invalid-rate-model',
      billingMode: 'per-request',
      price: '10',
      hasConflict: false,
    }

    expect(
      getPriceSummary(row, t, { currencySymbol: '¥', exchangeRate: 0 })
    ).toBe('¥— / request')
  })
})
