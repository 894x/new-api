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

import type { PricingModel } from '../pricing/types'
import {
  estimatePurchasableTokens,
  estimateTokenCost,
  isTokenPricedModel,
} from './lib'

const model: PricingModel = {
  id: 1,
  model_name: 'example-model',
  quota_type: 0,
  model_ratio: 1,
  completion_ratio: 2,
  cache_ratio: 0.25,
  create_cache_ratio: 1.25,
  enable_groups: ['default'],
}

describe('token cost estimate', () => {
  it('charges each token lane and applies recharge currency conversion', () => {
    const result = estimateTokenCost(model, 1, 4, 7, {
      totalTokens: 1_000_000,
      inputRatio: 3,
      outputRatio: 1,
      cacheHitRate: 40,
      cacheWriteRate: 10,
    })

    expect(result.inputTokens).toBe(750_000)
    expect(result.regularInputTokens).toBe(375_000)
    expect(result.cacheReadTokens).toBe(300_000)
    expect(result.cacheWriteTokens).toBe(75_000)
    expect(result.outputTokens).toBe(250_000)
    expect(result.costUSD).toBeCloseTo(2.0875)
    expect(result.customerCostUSD).toBeCloseTo((2.0875 * 4) / 7)
  })

  it('does not allow cache reads and writes to exceed the input tokens', () => {
    const result = estimateTokenCost(model, 1, 1, 1, {
      totalTokens: 1000,
      inputRatio: 1,
      outputRatio: 1,
      cacheHitRate: 80,
      cacheWriteRate: 80,
    })

    expect(result.regularInputTokens).toBe(0)
    expect(result.cacheReadTokens + result.cacheWriteTokens).toBe(500)
  })

  it('evaluates versioned dynamic pricing with the selected token mix', () => {
    const dynamicModel: PricingModel = {
      ...model,
      billing_mode: 'tiered_expr',
      billing_expr:
        'v1:len <= 200000 ? tier("standard", p*2 + c*8 + cr*0.5 + cc*2.5) : tier("long", p*4 + c*12 + cr*1 + cc*5)',
    }

    const result = estimateTokenCost(dynamicModel, 1.5, 1, 1, {
      totalTokens: 300_000,
      inputRatio: 3,
      outputRatio: 1,
      cacheHitRate: 40,
      cacheWriteRate: 10,
    })

    expect(isTokenPricedModel(dynamicModel)).toBe(true)
    expect(result.error).toBeNull()
    expect(result.matchedTier).toBe('long')
    expect(result.costUSD).toBeCloseTo(2.32875)
  })

  it('calculates purchasable tokens independently of the usage token field', () => {
    const tokens = estimatePurchasableTokens(model, 1, 1, 1, 2.5, {
      inputRatio: 3,
      outputRatio: 1,
      cacheHitRate: 0,
      cacheWriteRate: 0,
    })

    expect(tokens).toBe(1_000_000)
  })

  it('respects dynamic tier boundaries in reverse budget calculation', () => {
    const dynamicModel: PricingModel = {
      ...model,
      billing_mode: 'tiered_expr',
      billing_expr:
        'len <= 100 ? tier("low", p*1 + c*1) : tier("high", p*2 + c*2)',
    }

    const tokens = estimatePurchasableTokens(dynamicModel, 1, 1, 1, 0.0002, {
      inputRatio: 1,
      outputRatio: 0,
      cacheHitRate: 0,
      cacheWriteRate: 0,
    })

    expect(tokens).toBe(100)
  })

  it('never exposes a negative dynamic charge', () => {
    const invalidModel: PricingModel = {
      ...model,
      billing_mode: 'tiered_expr',
      billing_expr: 'p * -1',
    }

    const result = estimateTokenCost(invalidModel, 1, 1, 1, {
      totalTokens: 1000,
      inputRatio: 1,
      outputRatio: 0,
      cacheHitRate: 0,
      cacheWriteRate: 0,
    })

    expect(result.error).toBe('Invalid dynamic price result')
    expect(result.costUSD).toBe(0)
    expect(result.customerCostUSD).toBe(0)
  })
})
