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
export type ModelRateLimit = { rpm?: number | null; tpm?: number | null }
export type GroupRateLimit = {
  limits: [number, number, number]
  models: Record<string, ModelRateLimit>
}

export function parseGroupRateLimit(value: unknown): GroupRateLimit | null {
  let limits: unknown = value
  let models: unknown = {}
  if (typeof value === 'object' && value !== null && !Array.isArray(value)) {
    const config = value as Record<string, unknown>
    if (
      Object.keys(config).some((key) => key !== 'limits' && key !== 'models')
    ) {
      return null
    }
    limits = config.limits
    models = config.models === undefined ? {} : config.models
  }
  if (!Array.isArray(limits) || (limits.length !== 2 && limits.length !== 3)) {
    return null
  }
  if (
    !limits.every(
      (limit) => Number.isInteger(limit) && limit >= 0 && limit <= 2147483647
    )
  ) {
    return null
  }
  if (limits[1] < 1) return null
  if (typeof models !== 'object' || models === null || Array.isArray(models)) {
    return null
  }
  for (const [name, rule] of Object.entries(models)) {
    if (
      !name.trim() ||
      typeof rule !== 'object' ||
      rule === null ||
      Array.isArray(rule)
    ) {
      return null
    }
    for (const [key, limit] of Object.entries(rule)) {
      if (key !== 'rpm' && key !== 'tpm') return null
      if (
        limit !== null &&
        (!Number.isInteger(limit) ||
          typeof limit !== 'number' ||
          limit < 0 ||
          limit > 2147483647)
      ) {
        return null
      }
    }
  }
  return {
    limits: [limits[0], limits[1], limits[2] ?? 0],
    models: models as Record<string, ModelRateLimit>,
  }
}

export function isValidRateLimitJSON(value: string | undefined): boolean {
  if (!value || value.trim() === '') return true

  try {
    const parsed: unknown = JSON.parse(value)
    if (
      typeof parsed !== 'object' ||
      parsed === null ||
      Array.isArray(parsed)
    ) {
      return false
    }

    return Object.values(parsed).every(
      (value) => parseGroupRateLimit(value) !== null
    )
  } catch {
    return false
  }
}
