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
import type { PricingModel } from '@/features/pricing/types'

import type { ApiKey } from '../types'

export type ApiKeyPricingSource = {
  models: PricingModel[]
  currentGroup: string
  groupRatio: Record<string, number>
  groupModelRatio: Record<string, Record<string, number>>
  usableGroup: Record<string, unknown>
  autoGroups: string[]
  maxAutoGroups: number
}

export type ApiKeyModelPrice = {
  model: PricingModel
  groups: string[]
  specialGroups: string[]
}

export function getApiKeyModelPrices(
  apiKey: ApiKey,
  pricing: ApiKeyPricingSource
): ApiKeyModelPrice[] {
  const group = apiKey.group || pricing.currentGroup
  let groups: string[]
  if (group === 'auto') {
    if (apiKey.auto_groups?.length) {
      groups = [...new Set(apiKey.auto_groups)]
        .filter(
          (g) =>
            Object.hasOwn(pricing.usableGroup, g) &&
            Object.hasOwn(pricing.groupRatio, g)
        )
        .slice(0, pricing.maxAutoGroups)
    } else {
      groups = pricing.autoGroups
    }
  } else {
    groups = [group].filter(
      (g) =>
        Object.hasOwn(pricing.usableGroup, g) &&
        (!apiKey.group || Object.hasOwn(pricing.groupRatio, g))
    )
  }
  const limits = new Set((apiKey.model_limits || '').split(','))
  const entries: ApiKeyModelPrice[] = []
  for (const model of pricing.models) {
    if (apiKey.model_limits_enabled && !limits.has(model.model_name)) continue
    const availableGroups = groups.filter(
      (g) =>
        model.enable_groups.includes(g) || model.enable_groups.includes('all')
    )
    if (availableGroups.length === 0) continue
    entries.push({
      model: { ...model, enable_groups: availableGroups },
      groups: availableGroups,
      specialGroups: availableGroups.filter((g) =>
        Object.hasOwn(pricing.groupModelRatio[g] || {}, model.model_name)
      ),
    })
  }
  return entries.sort(
    (a, b) =>
      Number(b.specialGroups.length > 0) - Number(a.specialGroups.length > 0) ||
      a.model.model_name.localeCompare(b.model.model_name)
  )
}
