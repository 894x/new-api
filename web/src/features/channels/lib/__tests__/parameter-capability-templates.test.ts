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
  evaluateParameterCapabilities,
  resolveParameterCapabilities,
} from '../parameter-capabilities'
import {
  applyParameterCapabilityTemplate,
  PARAMETER_CAPABILITY_TEMPLATES,
} from '../parameter-capability-templates'

describe('Seedance capability templates', () => {
  it('accepts automatic duration and integer limits while rejecting gaps, fractions and strings', () => {
    const template = PARAMETER_CAPABILITY_TEMPLATES[0]
    const config = applyParameterCapabilityTemplate(
      {},
      template,
      'sls-alias',
      false
    )
    for (const duration of [-1, 4, 15]) {
      expect(
        evaluateParameterCapabilities(config, 'sls-alias', { duration })
          .compatible
      ).toBe(true)
    }
    for (const duration of [-2, 0, 3, 4.5, 16, '-1']) {
      expect(
        evaluateParameterCapabilities(config, 'sls-alias', { duration })
          .compatible
      ).toBe(false)
    }
    expect(
      evaluateParameterCapabilities(config, 'sls-alias', {}).compatible
    ).toBe(true)
    expect(
      evaluateParameterCapabilities(config, 'unrelated', { duration: 20 })
        .compatible
    ).toBe(true)
  })

  it('uses each model resolution and duration limits', () => {
    for (const template of PARAMETER_CAPABILITY_TEMPLATES) {
      const config = applyParameterCapabilityTemplate(
        {},
        template,
        template.model,
        false
      )
      expect(
        evaluateParameterCapabilities(config, template.model, {
          duration: template.maxDuration,
        }).compatible
      ).toBe(true)
      expect(
        evaluateParameterCapabilities(config, template.model, {
          duration: template.maxDuration + 1,
        }).compatible
      ).toBe(false)
      for (const resolution of ['480p', '720p', '1080p', '4k']) {
        expect(
          evaluateParameterCapabilities(config, template.model, { resolution })
            .compatible
        ).toBe(template.resolutions.some((value) => value === resolution))
      }
      expect(
        evaluateParameterCapabilities(config, template.model, {
          service_tier: 'flex',
        }).compatible
      ).toBe(false)
    }
  })

  it.each(PARAMETER_CAPABILITY_TEMPLATES)(
    'rejects frames and enabled draft for $name while accepting omitted parameters and disabled draft',
    (template) => {
      const config = applyParameterCapabilityTemplate(
        {},
        template,
        template.model,
        false
      )
      for (const request of [{ frames: 29 }, { frames: 0 }, { draft: true }]) {
        expect(
          evaluateParameterCapabilities(config, template.model, request)
            .compatible
        ).toBe(false)
      }
      for (const request of [{}, { draft: false }]) {
        expect(
          evaluateParameterCapabilities(config, template.model, request)
            .compatible
        ).toBe(true)
      }
    }
  )

  it('preserves existing effective constraints unless replacement is requested and does not duplicate rules', () => {
    const template = PARAMETER_CAPABILITY_TEMPLATES[0]
    const source = { defaults: { duration: { min: 5, max: 10 } } }
    const kept = applyParameterCapabilityTemplate(
      source,
      template,
      template.model,
      false
    )
    expect(
      resolveParameterCapabilities(kept, template.model).duration.capability.min
    ).toBe(5)
    const replaced = applyParameterCapabilityTemplate(
      kept,
      template,
      template.model,
      true
    )
    expect(replaced.rules).toHaveLength(1)
    expect(
      evaluateParameterCapabilities(replaced, template.model, { duration: -1 })
        .compatible
    ).toBe(true)
    expect(source).toEqual({ defaults: { duration: { min: 5, max: 10 } } })
  })
})
