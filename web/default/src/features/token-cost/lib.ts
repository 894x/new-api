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
import {
  splitBillingExprAndRequestRules,
  tryParseRequestRuleExpr,
} from '../pricing/lib/billing-expr'
import { applyRechargeRate, calculateTokenPrice } from '../pricing/lib/price'
import {
  evalExprLocally,
  type ExtraTokenValues,
} from '../pricing/lib/tier-expr'
import type { PricingModel } from '../pricing/types'

export const BUSINESS_PRESETS = [
  {
    labelKey: 'Simple chat',
    totalTokens: 2000,
    inputRatio: 3,
    outputRatio: 1,
    cacheHitRate: 0,
    cacheWriteRate: 0,
  },
  {
    labelKey: 'Customer support',
    totalTokens: 5000,
    inputRatio: 8,
    outputRatio: 1,
    cacheHitRate: 70,
    cacheWriteRate: 5,
  },
  {
    labelKey: 'RAG knowledge base',
    totalTokens: 15000,
    inputRatio: 15,
    outputRatio: 1,
    cacheHitRate: 40,
    cacheWriteRate: 5,
  },
  {
    labelKey: 'Long document summary',
    totalTokens: 50000,
    inputRatio: 10,
    outputRatio: 1,
    cacheHitRate: 10,
    cacheWriteRate: 0,
  },
  {
    labelKey: 'Coding assistant',
    totalTokens: 12000,
    inputRatio: 6,
    outputRatio: 1,
    cacheHitRate: 50,
    cacheWriteRate: 5,
  },
  {
    labelKey: 'Multi-step agent',
    totalTokens: 80000,
    inputRatio: 12,
    outputRatio: 1,
    cacheHitRate: 60,
    cacheWriteRate: 5,
  },
] as const

export type TokenCostInput = {
  totalTokens: number
  inputRatio: number
  outputRatio: number
  cacheHitRate: number
  cacheWriteRate: number
}

export type TokenLanePrices = {
  input: number
  output: number
  cacheRead: number
  cacheWrite: number
}

export type TokenCostEstimate = {
  inputTokens: number
  outputTokens: number
  regularInputTokens: number
  cacheReadTokens: number
  cacheWriteTokens: number
  costUSD: number
  customerCostUSD: number
  matchedTier: string
  hasRequestRules: boolean
  error: string | null
}

export function formatTokenCount(value: number): string {
  return new Intl.NumberFormat(undefined, { maximumFractionDigits: 0 }).format(
    value
  )
}

function validNumber(value: number, fallback = 0): number {
  return Number.isFinite(value) && value >= 0 ? value : fallback
}

function isDynamicPricingModel(model: PricingModel): boolean {
  return model.billing_mode === 'tiered_expr' && Boolean(model.billing_expr)
}

export function isTokenPricedModel(model: PricingModel): boolean {
  if (model.quota_type !== 0) return false
  return model.billing_mode !== 'tiered_expr' || Boolean(model.billing_expr)
}

export function getTokenLanePrices(
  model: PricingModel,
  groupRatio: number
): TokenLanePrices {
  const input = calculateTokenPrice(model, 'input', groupRatio)
  const output = calculateTokenPrice(model, 'output', groupRatio)
  const cacheRead = calculateTokenPrice(model, 'cache', groupRatio)
  const cacheWrite = calculateTokenPrice(model, 'create_cache', groupRatio)

  return {
    input,
    output,
    cacheRead: Number.isFinite(cacheRead) ? cacheRead : input,
    cacheWrite: Number.isFinite(cacheWrite) ? cacheWrite : input,
  }
}

function getTokenDistribution(
  value: TokenCostInput
): Omit<
  TokenCostEstimate,
  'costUSD' | 'customerCostUSD' | 'matchedTier' | 'hasRequestRules' | 'error'
> {
  const totalTokens = validNumber(value.totalTokens)
  const inputRatio = validNumber(value.inputRatio, 1)
  const outputRatio = validNumber(value.outputRatio, 1)
  const ratioTotal = Math.max(inputRatio + outputRatio, 1)
  const cacheHitRate = Math.min(validNumber(value.cacheHitRate), 100) / 100
  const cacheWriteRate =
    Math.min(validNumber(value.cacheWriteRate), 100 - cacheHitRate * 100) / 100
  const inputTokens = (totalTokens * inputRatio) / ratioTotal
  const outputTokens = totalTokens - inputTokens
  const cacheReadTokens = inputTokens * cacheHitRate
  const cacheWriteTokens = inputTokens * cacheWriteRate

  return {
    inputTokens,
    outputTokens,
    regularInputTokens: inputTokens - cacheReadTokens - cacheWriteTokens,
    cacheReadTokens,
    cacheWriteTokens,
  }
}

function estimateDynamicCost(
  model: PricingModel,
  distribution: ReturnType<typeof getTokenDistribution>
): {
  costUSD: number
  matchedTier: string
  hasRequestRules: boolean
  error: string | null
} {
  const expression = (model.billing_expr || '').trim().replace(/^v\d+:/, '')
  const { billingExpr, requestRuleExpr } =
    splitBillingExprAndRequestRules(expression)
  const extraTokenValues: ExtraTokenValues = {
    cacheReadTokens: distribution.cacheReadTokens,
    cacheCreateTokens: distribution.cacheWriteTokens,
    cacheCreate1hTokens: 0,
    imageTokens: 0,
    imageOutputTokens: 0,
    audioInputTokens: 0,
    audioOutputTokens: 0,
  }
  const result = evalExprLocally(
    billingExpr,
    distribution.regularInputTokens,
    distribution.outputTokens,
    extraTokenValues
  )
  const invalidCost =
    !Number.isFinite(result.cost) || result.cost < 0
      ? 'Invalid dynamic price result'
      : null

  return {
    costUSD: invalidCost ? 0 : result.cost / 1_000_000,
    matchedTier: result.matchedTier,
    hasRequestRules: Boolean(
      tryParseRequestRuleExpr(requestRuleExpr || '')?.length
    ),
    error: result.error || invalidCost,
  }
}

export function estimateTokenCost(
  model: PricingModel,
  groupRatio: number,
  priceRate: number,
  usdExchangeRate: number,
  value: TokenCostInput
): TokenCostEstimate {
  const distribution = getTokenDistribution(value)
  let costUSD = 0
  let matchedTier = ''
  let hasRequestRules = false
  let error: string | null = null

  if (isDynamicPricingModel(model)) {
    const dynamic = estimateDynamicCost(model, distribution)
    costUSD = dynamic.costUSD * Math.max(groupRatio, 0)
    matchedTier = dynamic.matchedTier
    hasRequestRules = dynamic.hasRequestRules
    error = dynamic.error
  } else {
    const lanePrices = getTokenLanePrices(model, groupRatio)
    costUSD =
      (distribution.regularInputTokens * lanePrices.input +
        distribution.cacheReadTokens * lanePrices.cacheRead +
        distribution.cacheWriteTokens * lanePrices.cacheWrite +
        distribution.outputTokens * lanePrices.output) /
      1_000_000
  }

  return {
    ...distribution,
    costUSD,
    customerCostUSD: applyRechargeRate(
      costUSD,
      true,
      Math.max(priceRate, 0),
      Math.max(usdExchangeRate, 0.001)
    ),
    matchedTier,
    hasRequestRules,
    error,
  }
}

export function estimatePurchasableTokens(
  model: PricingModel,
  groupRatio: number,
  priceRate: number,
  usdExchangeRate: number,
  budgetUSD: number,
  mix: Omit<TokenCostInput, 'totalTokens'>
): number {
  const budget = validNumber(budgetUSD)
  if (budget <= 0) return 0

  const estimateAt = (totalTokens: number): TokenCostEstimate =>
    estimateTokenCost(model, groupRatio, priceRate, usdExchangeRate, {
      ...mix,
      totalTokens,
    })

  const maximumTokens = 1_000_000_000_000_000
  let upperBound = 1
  let upperEstimate = estimateAt(upperBound)
  while (
    !upperEstimate.error &&
    upperEstimate.customerCostUSD <= budget &&
    upperBound < maximumTokens
  ) {
    upperBound = Math.min(upperBound * 2, maximumTokens)
    upperEstimate = estimateAt(upperBound)
  }

  if (
    upperEstimate.error ||
    (upperBound === maximumTokens && upperEstimate.customerCostUSD <= budget)
  ) {
    return 0
  }

  let lowerBound = 0
  while (lowerBound + 1 < upperBound) {
    const midpoint = Math.floor((lowerBound + upperBound) / 2)
    const estimate = estimateAt(midpoint)
    if (estimate.error || estimate.customerCostUSD > budget) {
      upperBound = midpoint
    } else {
      lowerBound = midpoint
    }
  }
  return lowerBound
}
