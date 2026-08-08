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
import assert from 'node:assert/strict'
import { describe, it } from 'node:test'

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
      cacheReadWriteRatio: 4,
    })

    assert.equal(result.inputTokens, 1_000_000)
    assert.equal(result.regularInputTokens, 600_000)
    assert.equal(result.cacheReadTokens, 400_000)
    assert.equal(result.cacheWriteTokens, 100_000)
    assert.ok(Math.abs(result.outputTokens - 333_333.33) < 0.01)
    assert.ok(Math.abs(result.costUSD - 2.98333333) < 1e-8)
    assert.ok(Math.abs(result.customerCostUSD - (2.98333333 * 4) / 7) < 1e-8)
  })

  it('uses the token scale as input volume and applies the cache read-write ratio', () => {
    const result = estimateTokenCost(model, 1, 1, 1, {
      totalTokens: 1000,
      inputRatio: 1,
      outputRatio: 1,
      cacheHitRate: 80,
      cacheReadWriteRatio: 10,
    })

    assert.equal(result.inputTokens, 1000)
    assert.equal(result.regularInputTokens, 200)
    assert.equal(result.cacheReadTokens, 800)
    assert.equal(result.cacheWriteTokens, 80)
    assert.equal(result.outputTokens, 1000)
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
      cacheReadWriteRatio: 4,
    })

    assert.equal(isTokenPricedModel(dynamicModel), true)
    assert.equal(result.error, null)
    assert.equal(result.matchedTier, 'long')
    assert.ok(Math.abs(result.costUSD - 3.285) < 1e-8)
  })

  it('calculates purchasable tokens independently of the usage token field', () => {
    const tokens = estimatePurchasableTokens(model, 1, 1, 1, 2.5, {
      inputRatio: 3,
      outputRatio: 1,
      cacheHitRate: 0,
      cacheReadWriteRatio: 1,
    })

    assert.equal(tokens, 750_000)
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
      cacheReadWriteRatio: 1,
    })

    assert.equal(tokens, 100)
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
      cacheReadWriteRatio: 1,
    })

    assert.equal(result.error, 'Invalid dynamic price result')
    assert.equal(result.costUSD, 0)
    assert.equal(result.customerCostUSD, 0)
  })
})
