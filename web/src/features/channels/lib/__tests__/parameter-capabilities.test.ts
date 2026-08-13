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

import type { ParameterCapabilityConfig } from '../../types'
import {
  evaluateParameterCapabilities,
  resolveParameterCapabilities,
  stringifyParameterCapabilityConfig,
  validateParameterCapabilityConfig,
} from '../parameter-capabilities'

describe('parameter capability resolution', () => {
  it('applies channel defaults, pattern rules, and exact rules in precedence order', () => {
    const config: ParameterCapabilityConfig = {
      defaults: {
        temperature: { min: 0, max: 2, on_violation: 'reject' },
      },
      rules: [
        {
          selector: { type: 'exact', value: 'gpt-5-mini' },
          parameters: { temperature: { max: 1 } },
        },
        {
          selector: { type: 'pattern', value: 'gpt-5*' },
          parameters: { temperature: { on_violation: 'clamp' } },
        },
      ],
    }

    const resolved = resolveParameterCapabilities(config, 'gpt-5-mini')

    expect(resolved.temperature.capability).toEqual({
      min: 0,
      max: 1,
      on_violation: 'clamp',
    })
    expect(resolved.temperature.source).toBe('gpt-5-mini')
  })

  it('keeps explicit zero values and clamps only values outside the range', () => {
    const config: ParameterCapabilityConfig = {
      defaults: {
        temperature: { min: 0, max: 1, on_violation: 'clamp' },
      },
    }

    const zero = evaluateParameterCapabilities(config, 'model-a', {
      temperature: 0,
    })
    const outOfRange = evaluateParameterCapabilities(config, 'model-a', {
      temperature: 1.5,
    })

    expect(zero.request).toEqual({ temperature: 0 })
    expect(zero.evaluations[0].status).toBe('compatible')
    expect(outOfRange.request).toEqual({ temperature: 1 })
    expect(outOfRange.evaluations[0].status).toBe('clamped')
  })

  it('drops unsupported parameters only when the configured action allows it', () => {
    const dropConfig: ParameterCapabilityConfig = {
      defaults: {
        top_k: { supported: false, on_violation: 'drop' },
      },
    }
    const rejectConfig: ParameterCapabilityConfig = {
      defaults: {
        top_k: { supported: false, on_violation: 'reject' },
      },
    }

    const dropped = evaluateParameterCapabilities(dropConfig, 'model-a', {
      model: 'model-a',
      top_k: 10,
    })
    const rejected = evaluateParameterCapabilities(rejectConfig, 'model-a', {
      top_k: 10,
    })

    expect(dropped.compatible).toBe(true)
    expect(dropped.request).toEqual({ model: 'model-a' })
    expect(rejected.compatible).toBe(false)
    expect(rejected.request).toEqual({ top_k: 10 })
  })
})

describe('parameter capability configuration validation', () => {
  it('rejects inverted ranges and clamp rules without a boundary', () => {
    const errors = validateParameterCapabilityConfig({
      defaults: {
        temperature: { min: 2, max: 1 },
        top_p: { on_violation: 'clamp' },
      },
    })

    expect(errors).toHaveLength(2)
  })

  it('requires billing-sensitive quantities to reject incompatible values', () => {
    const errors = validateParameterCapabilityConfig({
      defaults: {
        max_tokens: { max: 4096, on_violation: 'clamp' },
        'inferenceConfig.maxTokens': {
          supported: false,
          on_violation: 'drop',
        },
      },
    })

    expect(errors).toHaveLength(2)
    expect(errors).toEqual(
      expect.arrayContaining([
        expect.objectContaining({
          code: 'unsafe_billing_action',
          path: 'max_tokens',
        }),
        expect.objectContaining({
          code: 'unsafe_billing_action',
          path: 'inferenceConfig.maxTokens',
        }),
      ])
    )
  })

  it('omits an entirely empty configuration from channel settings', () => {
    expect(
      stringifyParameterCapabilityConfig({ defaults: {}, rules: [] })
    ).toBe('')
  })
})
