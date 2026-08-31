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
import * as z from 'zod'

import type {
  GroupModelTieredRatioPolicy,
  GroupModelTieredRatios,
} from '../../types'

export type GroupModelTieredPolicyStatus =
  | 'disabled'
  | 'scheduled'
  | 'active'
  | 'expired'

type ParseResult =
  | { success: true; data: GroupModelTieredRatios }
  | { success: false; error: string }

const LOCALIZED_VALIDATION_MESSAGES = new Set([
  'Thresholds must be safe non-negative integers',
  'Activation time must be a Unix timestamp',
  'Ratio must be a finite number between 0 and 1',
  'Timezone is invalid',
  'Each policy must contain at least one tier',
  'End time must be later than start time',
  'First tier threshold must be 0',
  'Tier thresholds must be strictly increasing',
  'Tier ratios must not increase',
  'Model name is required',
  'Model name must be at most 255 UTF-8 bytes',
  'Charged-progress tier ratios must be greater than 0',
  'Group name is required',
  'Group name must be at most 128 UTF-8 bytes',
])

export function getGroupModelTieredValidationMessage(
  message: string | undefined
): string {
  return message && LOCALIZED_VALIDATION_MESSAGES.has(message)
    ? message
    : 'Tiered discount configuration is invalid'
}

const utf8Encoder = new TextEncoder()

export const GROUP_NAME_MAX_UTF8_BYTES = 128
export const MODEL_NAME_MAX_UTF8_BYTES = 255

export function getUtf8ByteLength(value: string): number {
  return utf8Encoder.encode(value).byteLength
}

function canonicalizeTimezone(timezone: string): string | null {
  if (timezone.trim() === '') return null

  try {
    return new Intl.DateTimeFormat('en-US', {
      timeZone: timezone,
    }).resolvedOptions().timeZone
  } catch {
    return null
  }
}

export function hasDuplicateJsonObjectKey(value: string): boolean {
  let cursor = 0

  const skipWhitespace = () => {
    while (/\s/.test(value[cursor] ?? '')) cursor += 1
  }

  const scanString = (): string => {
    const start = cursor
    cursor += 1
    while (cursor < value.length) {
      if (value[cursor] === '\\') {
        cursor += 2
        continue
      }
      if (value[cursor] === '"') {
        cursor += 1
        return JSON.parse(value.slice(start, cursor)) as string
      }
      cursor += 1
    }
    return ''
  }

  const scanValue = (): boolean => {
    skipWhitespace()
    if (value[cursor] === '{') return scanObject()
    if (value[cursor] === '[') return scanArray()
    if (value[cursor] === '"') {
      scanString()
      return false
    }
    while (cursor < value.length && !/[\s,}\]]/.test(value[cursor])) {
      cursor += 1
    }
    return false
  }

  const scanArray = (): boolean => {
    cursor += 1
    skipWhitespace()
    if (value[cursor] === ']') {
      cursor += 1
      return false
    }
    while (cursor < value.length) {
      if (scanValue()) return true
      skipWhitespace()
      if (value[cursor] === ']') {
        cursor += 1
        return false
      }
      cursor += 1
    }
    return false
  }

  const scanObject = (): boolean => {
    cursor += 1
    const keys = new Set<string>()
    skipWhitespace()
    if (value[cursor] === '}') {
      cursor += 1
      return false
    }
    while (cursor < value.length) {
      skipWhitespace()
      const key = scanString()
      if (keys.has(key)) return true
      keys.add(key)
      skipWhitespace()
      cursor += 1
      if (scanValue()) return true
      skipWhitespace()
      if (value[cursor] === '}') {
        cursor += 1
        return false
      }
      cursor += 1
    }
    return false
  }

  return scanValue()
}

const safeNonnegativeIntegerSchema = z
  .number({ error: 'Thresholds must be safe non-negative integers' })
  .int('Thresholds must be safe non-negative integers')
  .nonnegative('Thresholds must be safe non-negative integers')
  .refine(Number.isSafeInteger, {
    message: 'Thresholds must be safe non-negative integers',
  })

const unixTimestampSchema = z
  .number({ error: 'Activation time must be a Unix timestamp' })
  .int('Activation time must be a Unix timestamp')
  .nonnegative('Activation time must be a Unix timestamp')
  .refine(Number.isSafeInteger, {
    message: 'Activation time must be a Unix timestamp',
  })

const tierSchema = z
  .object({
    min_monthly_original_quota: safeNonnegativeIntegerSchema,
    ratio: z
      .number({ error: 'Ratio must be a finite number between 0 and 1' })
      .refine(Number.isFinite, {
        message: 'Ratio must be a finite number between 0 and 1',
      })
      .min(0, 'Ratio must be a finite number between 0 and 1')
      .max(1, 'Ratio must be a finite number between 0 and 1'),
  })
  .strict()

export const groupModelTieredRatioPolicySchema = z
  .object({
    enabled: z.boolean(),
    effective_from: unixTimestampSchema,
    effective_until: unixTimestampSchema.nullable(),
    timezone: z.string().transform((timezone, context) => {
      const canonicalTimezone = canonicalizeTimezone(timezone)
      if (canonicalTimezone !== null) return canonicalTimezone
      context.addIssue({ code: 'custom', message: 'Timezone is invalid' })
      return z.NEVER
    }),
    tiers: z
      .array(tierSchema)
      .min(1, 'Each policy must contain at least one tier'),
  })
  .strict()
  .superRefine((policy, context) => {
    if (
      policy.effective_until !== null &&
      policy.effective_until <= policy.effective_from
    ) {
      context.addIssue({
        code: 'custom',
        path: ['effective_until'],
        message: 'End time must be later than start time',
      })
    }

    if (
      policy.tiers.length > 0 &&
      policy.tiers[0].min_monthly_original_quota !== 0
    ) {
      context.addIssue({
        code: 'custom',
        path: ['tiers', 0, 'min_monthly_original_quota'],
        message: 'First tier threshold must be 0',
      })
    }

    for (let index = 1; index < policy.tiers.length; index += 1) {
      const previousTier = policy.tiers[index - 1]
      const tier = policy.tiers[index]

      if (
        tier.min_monthly_original_quota <=
        previousTier.min_monthly_original_quota
      ) {
        context.addIssue({
          code: 'custom',
          path: ['tiers', index, 'min_monthly_original_quota'],
          message: 'Tier thresholds must be strictly increasing',
        })
      }

      if (tier.ratio > previousTier.ratio) {
        context.addIssue({
          code: 'custom',
          path: ['tiers', index, 'ratio'],
          message: 'Tier ratios must not increase',
        })
      }
    }
  })

const modelPolicyMapSchema = z
  .record(z.string(), groupModelTieredRatioPolicySchema)
  .superRefine((modelPolicies, context) => {
    for (const modelName of Object.keys(modelPolicies)) {
      if (modelName.trim() === '') {
        context.addIssue({
          code: 'custom',
          path: [modelName],
          message: 'Model name is required',
        })
        continue
      }
      if (getUtf8ByteLength(modelName) > MODEL_NAME_MAX_UTF8_BYTES) {
        context.addIssue({
          code: 'custom',
          path: [modelName],
          message: 'Model name must be at most 255 UTF-8 bytes',
        })
      }
    }
  })

const groupConfigSchema = z
  .object({
    progress_basis: z.enum(['original', 'charged']),
    models: modelPolicyMapSchema,
  })
  .strict()
  .superRefine((groupConfig, context) => {
    if (groupConfig.progress_basis !== 'charged') return

    for (const [modelName, policy] of Object.entries(groupConfig.models)) {
      for (let index = 0; index < policy.tiers.length; index += 1) {
        if (policy.tiers[index].ratio > 0) continue
        context.addIssue({
          code: 'custom',
          path: ['models', modelName, 'tiers', index, 'ratio'],
          message: 'Charged-progress tier ratios must be greater than 0',
        })
      }
    }
  })

export const groupModelTieredRatiosSchema = z
  .record(z.string(), groupConfigSchema)
  .superRefine((groups, context) => {
    for (const groupName of Object.keys(groups)) {
      if (groupName.trim() === '') {
        context.addIssue({
          code: 'custom',
          path: [groupName],
          message: 'Group name is required',
        })
        continue
      }
      if (getUtf8ByteLength(groupName) > GROUP_NAME_MAX_UTF8_BYTES) {
        context.addIssue({
          code: 'custom',
          path: [groupName],
          message: 'Group name must be at most 128 UTF-8 bytes',
        })
      }
    }
  })

function isObjectRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === 'object' && value !== null && !Array.isArray(value)
}

function isPolicyLike(value: unknown): boolean {
  return isObjectRecord(value) && 'enabled' in value && 'tiers' in value
}

function isCanonicalGroupCandidate(value: Record<string, unknown>): boolean {
  if (typeof value.progress_basis === 'string') return true
  return isObjectRecord(value.models) && !isPolicyLike(value.models)
}

export function normalizeGroupModelTieredRatiosShape(value: unknown): unknown {
  if (!isObjectRecord(value)) return value

  return Object.fromEntries(
    Object.entries(value).map(([groupName, groupValue]) => {
      if (!isObjectRecord(groupValue)) return [groupName, groupValue]
      if (!isCanonicalGroupCandidate(groupValue)) {
        return [groupName, { progress_basis: 'original', models: groupValue }]
      }
      return [groupName, groupValue]
    })
  )
}

export function parseGroupModelTieredRatiosJson(value: string): ParseResult {
  let parsed: unknown
  try {
    parsed = JSON.parse(value)
  } catch {
    return { success: false, error: 'Tiered discount JSON is invalid' }
  }

  if (isObjectRecord(parsed)) {
    for (const [groupName, groupValue] of Object.entries(parsed)) {
      if (groupName === '__proto__') {
        return {
          success: false,
          error: 'Group and model names cannot use __proto__',
        }
      }
      if (!isObjectRecord(groupValue)) continue

      const candidateModelMaps = [groupValue]
      if (isObjectRecord(groupValue.models)) {
        candidateModelMaps.push(groupValue.models)
      }
      if (
        candidateModelMaps.some((modelMap) =>
          Object.keys(modelMap).includes('__proto__')
        )
      ) {
        return {
          success: false,
          error: 'Group and model names cannot use __proto__',
        }
      }
    }
  }

  if (hasDuplicateJsonObjectKey(value)) {
    return {
      success: false,
      error: 'Duplicate group and model policies are not allowed',
    }
  }

  const result = groupModelTieredRatiosSchema.safeParse(
    normalizeGroupModelTieredRatiosShape(parsed)
  )
  if (!result.success) {
    return {
      success: false,
      error: getGroupModelTieredValidationMessage(
        result.error.issues[0]?.message
      ),
    }
  }

  return { success: true, data: result.data }
}

export function serializeGroupModelTieredRatios(
  value: GroupModelTieredRatios
): string {
  return JSON.stringify(value, null, 2)
}

export function normalizeGroupModelTieredRatiosJson(value: string): string {
  const result = parseGroupModelTieredRatiosJson(value)
  return result.success ? JSON.stringify(result.data) : value
}

export function formatGroupModelTieredRatiosForTextarea(value: string): string {
  const result = parseGroupModelTieredRatiosJson(value)
  return result.success ? serializeGroupModelTieredRatios(result.data) : value
}

export function getGroupModelTieredPolicyStatus(
  policy: GroupModelTieredRatioPolicy,
  now: number
): GroupModelTieredPolicyStatus {
  if (!policy.enabled) return 'disabled'
  if (policy.effective_from > now) return 'scheduled'
  if (policy.effective_until !== null && policy.effective_until <= now) {
    return 'expired'
  }
  return 'active'
}
