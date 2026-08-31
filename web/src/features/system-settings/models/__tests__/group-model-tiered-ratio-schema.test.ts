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
import { describe, expect, test } from 'vitest'

import type {
  GroupModelTieredRatioPolicy,
  GroupModelTieredRatios,
} from '../../types'
import {
  getGroupModelTieredPolicyStatus,
  groupModelTieredRatiosSchema,
  parseGroupModelTieredRatiosJson,
  serializeGroupModelTieredRatios,
} from '../lib/group-model-tiered-ratio-schema'

const validPolicy = {
  enabled: true,
  effective_from: 1_788_192_000,
  effective_until: 1_790_870_400,
  timezone: 'Asia/Shanghai',
  tiers: [
    { min_monthly_original_quota: 0, ratio: 1 },
    { min_monthly_original_quota: 500_000, ratio: 0.9 },
    { min_monthly_original_quota: 2_500_000, ratio: 0.8 },
  ],
} satisfies GroupModelTieredRatioPolicy

function originalProgressGroup(
  models: Record<string, GroupModelTieredRatioPolicy>
) {
  return { progress_basis: 'original' as const, models }
}

describe('group-model tiered ratio schema', () => {
  test('normalizes legacy group model maps to canonical original-progress groups', () => {
    expect(
      parseGroupModelTieredRatiosJson(
        JSON.stringify({ premium: { 'gpt-5': validPolicy } })
      )
    ).toEqual({
      success: true,
      data: {
        premium: {
          progress_basis: 'original',
          models: { 'gpt-5': validPolicy },
        },
      },
    })
  })

  test.each([
    ['progress_basis', { premium: { models: { 'gpt-5': validPolicy } } }],
    ['models', { premium: { progress_basis: 'charged' } }],
  ])('rejects a canonical wrapper missing %s', (_field, config) => {
    expect(
      parseGroupModelTieredRatiosJson(JSON.stringify(config)).success
    ).toBe(false)
  })

  test.each(['models', 'progress_basis'])(
    'keeps a legacy model literally named %s compatible',
    (modelName) => {
      expect(
        parseGroupModelTieredRatiosJson(
          JSON.stringify({ premium: { [modelName]: validPolicy } })
        )
      ).toEqual({
        success: true,
        data: {
          premium: {
            progress_basis: 'original',
            models: { [modelName]: validPolicy },
          },
        },
      })
    }
  )

  test('accepts only original or charged as a canonical group progress basis', () => {
    const charged = {
      premium: {
        progress_basis: 'charged',
        models: { 'gpt-5': validPolicy },
      },
    }

    expect(groupModelTieredRatiosSchema.safeParse(charged).success).toBe(true)
    expect(
      groupModelTieredRatiosSchema.safeParse({
        premium: { ...charged.premium, progress_basis: 'invoice' },
      }).success
    ).toBe(false)
  })

  test('rejects a zero ratio at the ratio field when charged progress is enabled', () => {
    const result = groupModelTieredRatiosSchema.safeParse({
      premium: {
        progress_basis: 'charged',
        models: {
          'gpt-5': {
            ...validPolicy,
            tiers: [
              { min_monthly_original_quota: 0, ratio: 0.8 },
              { min_monthly_original_quota: 500_000, ratio: 0 },
            ],
          },
        },
      },
    })

    expect(result.success).toBe(false)
    if (!result.success) {
      expect(result.error.issues[0]).toMatchObject({
        path: ['premium', 'models', 'gpt-5', 'tiers', 1, 'ratio'],
        message: 'Charged-progress tier ratios must be greater than 0',
      })
    }
  })

  test('accepts an empty object and losslessly round-trips exact and wildcard models', () => {
    expect(parseGroupModelTieredRatiosJson('{}')).toEqual({
      success: true,
      data: {},
    })

    const config: GroupModelTieredRatios = {
      premium: originalProgressGroup({
        'gpt-5': validPolicy,
        '*': {
          ...validPolicy,
          effective_until: null,
          timezone: 'UTC',
        },
      }),
    }

    const serialized = serializeGroupModelTieredRatios(config)
    expect(parseGroupModelTieredRatiosJson(serialized)).toEqual({
      success: true,
      data: config,
    })
  })

  test.each([
    ['utc', 'UTC'],
    ['asia/shanghai', 'Asia/Shanghai'],
  ])('canonicalizes browser-accepted timezone %s to %s', (input, expected) => {
    const result = parseGroupModelTieredRatiosJson(
      JSON.stringify({
        premium: originalProgressGroup({
          'gpt-5': { ...validPolicy, timezone: input },
        }),
      })
    )

    expect(result.success).toBe(true)
    if (result.success) {
      expect(result.data.premium.models['gpt-5'].timezone).toBe(expected)
      expect(
        JSON.parse(serializeGroupModelTieredRatios(result.data)).premium.models[
          'gpt-5'
        ].timezone
      ).toBe(expected)
    }
  })

  test.each([
    ['empty group', { '': originalProgressGroup({ 'gpt-5': validPolicy }) }],
    ['empty model', { premium: originalProgressGroup({ '': validPolicy }) }],
    [
      'missing tiers',
      {
        premium: originalProgressGroup({
          'gpt-5': { ...validPolicy, tiers: [] },
        }),
      },
    ],
    [
      'first threshold is not zero',
      {
        premium: originalProgressGroup({
          'gpt-5': {
            ...validPolicy,
            tiers: [{ min_monthly_original_quota: 1, ratio: 1 }],
          },
        }),
      },
    ],
    [
      'threshold is not an integer',
      {
        premium: originalProgressGroup({
          'gpt-5': {
            ...validPolicy,
            tiers: [{ min_monthly_original_quota: 0.5, ratio: 1 }],
          },
        }),
      },
    ],
    [
      'threshold is unsafe',
      {
        premium: originalProgressGroup({
          'gpt-5': {
            ...validPolicy,
            tiers: [
              { min_monthly_original_quota: 0, ratio: 1 },
              {
                min_monthly_original_quota: Number.MAX_SAFE_INTEGER + 1,
                ratio: 0.9,
              },
            ],
          },
        }),
      },
    ],
    [
      'thresholds do not strictly increase',
      {
        premium: originalProgressGroup({
          'gpt-5': {
            ...validPolicy,
            tiers: [
              { min_monthly_original_quota: 0, ratio: 1 },
              { min_monthly_original_quota: 0, ratio: 0.9 },
            ],
          },
        }),
      },
    ],
    [
      'ratio is outside the discount range',
      {
        premium: originalProgressGroup({
          'gpt-5': {
            ...validPolicy,
            tiers: [{ min_monthly_original_quota: 0, ratio: 1.01 }],
          },
        }),
      },
    ],
    [
      'ratios increase in a later tier',
      {
        premium: originalProgressGroup({
          'gpt-5': {
            ...validPolicy,
            tiers: [
              { min_monthly_original_quota: 0, ratio: 0.8 },
              { min_monthly_original_quota: 500_000, ratio: 0.9 },
            ],
          },
        }),
      },
    ],
    [
      'timezone is invalid',
      {
        premium: originalProgressGroup({
          'gpt-5': { ...validPolicy, timezone: 'Mars/Olympus_Mons' },
        }),
      },
    ],
    [
      'timezone depends on the server local setting',
      {
        premium: originalProgressGroup({
          'gpt-5': { ...validPolicy, timezone: 'Local' },
        }),
      },
    ],
    [
      'end time is not later than start time',
      {
        premium: originalProgressGroup({
          'gpt-5': {
            ...validPolicy,
            effective_until: validPolicy.effective_from,
          },
        }),
      },
    ],
  ])('rejects %s', (_label, config) => {
    expect(groupModelTieredRatiosSchema.safeParse(config).success).toBe(false)
  })

  test('enforces the 128-byte UTF-8 group-name boundary', () => {
    const groupAtLimit = `${'组'.repeat(42)}ab`
    const groupOverLimit = `${groupAtLimit}c`

    expect(
      groupModelTieredRatiosSchema.safeParse({
        [groupAtLimit]: originalProgressGroup({ 'gpt-5': validPolicy }),
      }).success
    ).toBe(true)

    const result = groupModelTieredRatiosSchema.safeParse({
      [groupOverLimit]: originalProgressGroup({ 'gpt-5': validPolicy }),
    })
    expect(result.success).toBe(false)
    if (!result.success) {
      expect(result.error.issues[0]?.message).toBe(
        'Group name must be at most 128 UTF-8 bytes'
      )
    }
  })

  test('enforces the 255-byte UTF-8 model-name boundary', () => {
    const modelAtLimit = '模'.repeat(85)
    const modelOverLimit = `${modelAtLimit}a`

    expect(
      groupModelTieredRatiosSchema.safeParse({
        premium: originalProgressGroup({ [modelAtLimit]: validPolicy }),
      }).success
    ).toBe(true)

    const result = groupModelTieredRatiosSchema.safeParse({
      premium: originalProgressGroup({ [modelOverLimit]: validPolicy }),
    })
    expect(result.success).toBe(false)
    if (!result.success) {
      expect(result.error.issues[0]?.message).toBe(
        'Model name must be at most 255 UTF-8 bytes'
      )
    }
  })

  test('rejects malformed JSON instead of replacing it with an empty config', () => {
    const result = parseGroupModelTieredRatiosJson('{"premium":')

    expect(result.success).toBe(false)
    if (!result.success) {
      expect(result.error).toBe('Tiered discount JSON is invalid')
    }
  })

  test.each([
    [
      'an unsupported progress basis',
      {
        premium: {
          progress_basis: 'net',
          models: { 'gpt-5': validPolicy },
        },
      },
    ],
    [
      'an unsupported policy field',
      {
        premium: {
          progress_basis: 'original',
          models: { 'gpt-5': { ...validPolicy, unexpected: true } },
        },
      },
    ],
    [
      'a non-boolean enabled value',
      {
        premium: {
          progress_basis: 'original',
          models: { 'gpt-5': { ...validPolicy, enabled: 'yes' } },
        },
      },
    ],
  ])('uses a localized generic error for %s', (_label, config) => {
    expect(parseGroupModelTieredRatiosJson(JSON.stringify(config))).toEqual({
      success: false,
      error: 'Tiered discount configuration is invalid',
    })
  })

  test.each([
    [
      'group',
      `{"premium":{"gpt-5":${JSON.stringify(validPolicy)}},"premium":{"*":${JSON.stringify(validPolicy)}}}`,
    ],
    [
      'model within one group',
      `{"premium":{"gpt-5":${JSON.stringify(validPolicy)},"gpt-5":${JSON.stringify(validPolicy)}}}`,
    ],
    [
      'policy field',
      `{"premium":{"gpt-5":{"enabled":true,"enabled":false,"effective_from":1788192000,"effective_until":null,"timezone":"UTC","tiers":[{"min_monthly_original_quota":0,"ratio":1}]}}}`,
    ],
    [
      'tier field',
      `{"premium":{"gpt-5":{"enabled":false,"effective_from":1788192000,"effective_until":null,"timezone":"UTC","tiers":[{"min_monthly_original_quota":0,"ratio":1,"ratio":0.9}]}}}`,
    ],
  ])('rejects a duplicate %s key in raw JSON', (_label, value) => {
    expect(parseGroupModelTieredRatiosJson(value)).toEqual({
      success: false,
      error: 'Duplicate group and model policies are not allowed',
    })
  })

  test.each([
    ['legacy group', `{"__proto__":{"gpt-5":${JSON.stringify(validPolicy)}}}`],
    [
      'canonical group',
      `{"__proto__":{"progress_basis":"original","models":{"gpt-5":${JSON.stringify(validPolicy)}}}}`,
    ],
    [
      'legacy model',
      `{"premium":{"__proto__":${JSON.stringify(validPolicy)}}}`,
    ],
    [
      'canonical model',
      `{"premium":{"progress_basis":"original","models":{"__proto__":${JSON.stringify(validPolicy)}}}}`,
    ],
    [
      'Unicode-escaped canonical model',
      `{"premium":{"progress_basis":"original","models":{"\\u005f\\u005fproto__":${JSON.stringify(validPolicy)}}}}`,
    ],
  ])(
    'rejects an exact __proto__ %s key without silently dropping it',
    (_label, value) => {
      expect(parseGroupModelTieredRatiosJson(value)).toEqual({
        success: false,
        error: 'Group and model names cannot use __proto__',
      })
    }
  )
})

describe('group-model tiered ratio policy status', () => {
  const now = 1_788_624_000

  test.each([
    ['disabled', { ...validPolicy, enabled: false }, 'disabled'],
    ['scheduled', { ...validPolicy, effective_from: now + 1 }, 'scheduled'],
    ['expired', { ...validPolicy, effective_until: now }, 'expired'],
    [
      'active',
      { ...validPolicy, effective_from: now, effective_until: null },
      'active',
    ],
  ])(
    'returns %s at the exact timestamp boundaries',
    (_label, policy, status) => {
      expect(getGroupModelTieredPolicyStatus(policy, now)).toBe(status)
    }
  )
})
