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
import { useQuery } from '@tanstack/react-query'
import { useMemo, useState, type ReactElement } from 'react'
import { useTranslation } from 'react-i18next'

import { PublicLayout } from '@/components/layout'
import { PageTransition } from '@/components/page-transition'
import { Skeleton } from '@/components/ui/skeleton'
import { useStatus } from '@/hooks/use-status'
import { convertBillingCurrencyToUSD } from '@/lib/currency'

import { getTokenCostPricing } from './api'
import { BusinessPresets } from './components/BusinessPresets'
import { CalculatorForm } from './components/CalculatorForm'
import { EstimateSummary } from './components/EstimateSummary'
import { PriceBreakdown } from './components/PriceBreakdown'
import {
  estimatePurchasableTokens,
  estimateTokenCost,
  getTokenLanePrices,
  isTokenPricedModel,
  type TokenCostInput,
} from './lib'

const DEFAULT_INPUT: TokenCostInput = {
  totalTokens: 5000,
  inputRatio: 4,
  outputRatio: 1,
  cacheHitRate: 0,
  cacheWriteRate: 0,
}

function parseNumber(value: string, fallback: number): number {
  const parsed = Number(value)
  return Number.isFinite(parsed) && parsed >= 0 ? parsed : fallback
}

export function TokenCost(): ReactElement {
  const { t } = useTranslation()
  const { status } = useStatus()
  const { data, isLoading, isError } = useQuery({
    queryKey: ['token-cost-pricing'],
    queryFn: getTokenCostPricing,
    staleTime: 5 * 60 * 1000,
  })
  const [modelName, setModelName] = useState('')
  const [groupName, setGroupName] = useState('')
  const [budget, setBudget] = useState(10)
  const [input, setInput] = useState<TokenCostInput>(DEFAULT_INPUT)
  const models = useMemo(
    () =>
      [...(data?.data ?? [])]
        .filter(isTokenPricedModel)
        .sort((a, b) => a.model_name.localeCompare(b.model_name)),
    [data?.data]
  )
  const model =
    models.find((item) => item.model_name === modelName) ?? models[0]
  const groups = useMemo(() => {
    if (!model) return []
    if (model.enable_groups.includes('all')) {
      return Object.keys(data?.group_ratio ?? {}).filter(
        (name) => name !== 'auto'
      )
    }
    return model.enable_groups.filter(
      (name) => data?.group_ratio[name] !== undefined
    )
  }, [data?.group_ratio, model])
  const defaultGroup =
    data?.current_group && groups.includes(data.current_group)
      ? data.current_group
      : groups.reduce(
          (best, name) =>
            data && data.group_ratio[name] < data.group_ratio[best]
              ? name
              : best,
          groups[0] ?? ''
        )
  const group = groups.includes(groupName) ? groupName : defaultGroup

  if (isLoading) return <TokenCostLoading />
  if (isError || !model || !data) {
    return (
      <PublicLayout showMainContainer={false}>
        <div className='text-muted-foreground mx-auto max-w-3xl px-4 py-24 text-center'>
          {isError
            ? t('Unable to load pricing data.')
            : t('No token-priced models are currently available.')}
        </div>
      </PublicLayout>
    )
  }

  const groupRatio = data.group_ratio[group] ?? 1
  const priceRate = Math.max(Number(status?.price ?? 1), 0.001)
  const usdExchangeRate = Math.max(
    Number(status?.usd_exchange_rate ?? priceRate),
    0.001
  )
  const estimate = estimateTokenCost(
    model,
    groupRatio,
    priceRate,
    usdExchangeRate,
    input
  )
  const purchasableTokens = estimatePurchasableTokens(
    model,
    groupRatio,
    priceRate,
    usdExchangeRate,
    convertBillingCurrencyToUSD(budget),
    {
      inputRatio: input.inputRatio,
      outputRatio: input.outputRatio,
      cacheHitRate: input.cacheHitRate,
      cacheWriteRate: input.cacheWriteRate,
    }
  )
  const lanePrices = getTokenLanePrices(model, groupRatio)
  const dynamic =
    model.billing_mode === 'tiered_expr' && Boolean(model.billing_expr)

  const updateInput = (key: keyof TokenCostInput, value: string): void => {
    setInput((current) => {
      const nextValue = parseNumber(value, current[key])
      if (key === 'cacheHitRate') {
        const cacheHitRate = Math.min(nextValue, 100)
        return {
          ...current,
          cacheHitRate,
          cacheWriteRate: Math.min(current.cacheWriteRate, 100 - cacheHitRate),
        }
      }
      if (key === 'cacheWriteRate') {
        return {
          ...current,
          cacheWriteRate: Math.min(nextValue, 100 - current.cacheHitRate),
        }
      }
      return { ...current, [key]: nextValue }
    })
  }

  return (
    <PublicLayout showMainContainer={false}>
      <PageTransition className='mx-auto w-full max-w-6xl space-y-6 px-3 pt-16 pb-10 sm:px-6 sm:pt-20'>
        <header className='mx-auto max-w-3xl text-center'>
          <h1 className='text-3xl font-bold tracking-tight sm:text-4xl'>
            {t('Token Cost Calculator')}
          </h1>
          <p className='text-muted-foreground mt-3 text-sm sm:text-base'>
            {t(
              'Estimate customer-facing costs from your configured model prices, input/output mix, and cache usage.'
            )}
          </p>
        </header>

        <div className='grid gap-5 lg:grid-cols-[minmax(0,1.35fr)_minmax(320px,0.65fr)]'>
          <CalculatorForm
            models={models}
            model={model}
            groups={groups}
            group={group}
            pricing={data}
            input={input}
            onModelChange={setModelName}
            onGroupChange={setGroupName}
            onInputChange={updateInput}
          />
          <EstimateSummary
            totalTokens={input.totalTokens}
            budget={budget}
            purchasableTokens={purchasableTokens}
            estimate={estimate}
            onBudgetChange={(value) => setBudget(parseNumber(value, budget))}
          />
        </div>

        <PriceBreakdown
          dynamic={dynamic}
          totalTokens={input.totalTokens}
          estimate={estimate}
          lanePrices={lanePrices}
          priceRate={priceRate}
          usdExchangeRate={usdExchangeRate}
        />
        <BusinessPresets onSelect={setInput} />
      </PageTransition>
    </PublicLayout>
  )
}

function TokenCostLoading(): ReactElement {
  return (
    <PublicLayout showMainContainer={false}>
      <div className='mx-auto max-w-6xl space-y-5 px-4 pt-20'>
        <Skeleton className='mx-auto h-20 max-w-2xl' />
        <Skeleton className='h-[360px] w-full' />
        <Skeleton className='h-[180px] w-full' />
      </div>
    </PublicLayout>
  )
}
