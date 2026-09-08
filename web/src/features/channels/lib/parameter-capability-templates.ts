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
import type { ParameterCapabilityConfig, ParameterCapability } from '../types'
import { resolveParameterCapabilities } from './parameter-capabilities'

export const PARAMETER_CAPABILITY_TEMPLATES = [
  {
    id: 'seedance-2.0',
    name: 'Seedance 2.0',
    model: 'doubao-seedance-2-0-260128',
    maxDuration: 15,
    resolutions: ['480p', '720p', '1080p', '4k'],
  },
  {
    id: 'seedance-2.0-fast',
    name: 'Seedance 2.0 Fast',
    model: 'doubao-seedance-2-0-fast-260128',
    maxDuration: 15,
    resolutions: ['480p', '720p'],
  },
  {
    id: 'seedance-2.0-mini',
    name: 'Seedance 2.0 Mini',
    model: 'doubao-seedance-2-0-mini-260615',
    maxDuration: 15,
    resolutions: ['480p', '720p'],
  },
  {
    id: 'seedance-2.5',
    name: 'Seedance 2.5',
    model: 'doubao-seedance-2-5-260628',
    maxDuration: 30,
    resolutions: ['480p', '720p', '1080p'],
  },
] as const

export type ParameterCapabilityTemplate =
  (typeof PARAMETER_CAPABILITY_TEMPLATES)[number]

export const PARAMETER_CAPABILITY_TEMPLATE_SOURCE =
  'https://ark.volcengine.com/region:cn-beijing/docs/82379/2298881?lang=zh'
export const PARAMETER_CAPABILITY_TEMPLATE_VERSION = '2026-09-08'

export function applyParameterCapabilityTemplate(
  config: ParameterCapabilityConfig,
  template: ParameterCapabilityTemplate,
  model: string,
  replace: boolean
): ParameterCapabilityConfig {
  const parameters: Record<string, ParameterCapability> = {
    duration: {
      supported: true,
      min: -1,
      max: template.maxDuration,
      allowed_values: [
        '-1',
        ...Array.from({ length: template.maxDuration - 3 }, (_, index) =>
          String(index + 4)
        ),
      ],
      on_violation: 'reject',
      participate_in_selection: false,
    },
    resolution: {
      supported: true,
      allowed_values: [...template.resolutions],
      on_violation: 'reject',
      participate_in_selection: false,
    },
    ratio: {
      supported: true,
      allowed_values: ['16:9', '4:3', '1:1', '3:4', '9:16', '21:9', 'adaptive'],
      on_violation: 'reject',
      participate_in_selection: false,
    },
    service_tier: {
      supported: true,
      allowed_values: ['default'],
      on_violation: 'reject',
      participate_in_selection: false,
    },
  }
  const targetModel = model.trim()
  const effective = resolveParameterCapabilities(config, targetModel)
  if (!replace) {
    for (const path of Object.keys(parameters)) {
      if (effective[path]) parameters[path] = { ...effective[path].capability }
    }
  }
  const rules = [...(config.rules || [])]
  const index = rules.findLastIndex(
    (rule) =>
      rule.selector.type === 'exact' && rule.selector.value === targetModel
  )
  const rule = {
    name: `${template.name} (${PARAMETER_CAPABILITY_TEMPLATE_VERSION})`,
    selector: { type: 'exact' as const, value: targetModel },
    parameters: {
      ...(index >= 0 ? rules[index].parameters : {}),
      ...parameters,
    },
  }
  if (index >= 0) {
    rules[index] = { ...rule, name: rules[index].name || rule.name }
  } else {
    rules.push(rule)
  }
  return { ...config, rules }
}
