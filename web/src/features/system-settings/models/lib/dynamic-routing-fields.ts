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
import type { DynamicRoutingSettings } from '../../types'
import { MAX_DYNAMIC_ROUTING_DURATION_SECONDS } from './dynamic-routing-schema'

type NumericSettingKey = Exclude<keyof DynamicRoutingSettings, 'enabled'>

export type DynamicRoutingNumericFieldDefinition = {
  name: `dynamic_routing_setting.${NumericSettingKey}`
  labelKey: string
  descriptionKey: string
  min: number
  max?: number
  step: number
}

export const DYNAMIC_ROUTING_SAMPLING_FIELDS: DynamicRoutingNumericFieldDefinition[] =
  [
    {
      name: 'dynamic_routing_setting.max_samples',
      labelKey: 'Maximum samples',
      descriptionKey:
        'Maximum recent TTFT/TPOT observations retained for each channel and model.',
      min: 1,
      step: 1,
    },
    {
      name: 'dynamic_routing_setting.max_age_seconds',
      labelKey: 'Maximum sample age (seconds)',
      descriptionKey:
        'Observations older than this time are ignored, even when the sample limit is not reached.',
      min: 1,
      max: MAX_DYNAMIC_ROUTING_DURATION_SECONDS,
      step: 1,
    },
    {
      name: 'dynamic_routing_setting.min_samples',
      labelKey: 'Minimum samples',
      descriptionKey:
        'Valid TTFT/TPOT observations required before comparing a channel.',
      min: 1,
      step: 1,
    },
    {
      name: 'dynamic_routing_setting.probe_fraction',
      labelKey: 'Probe traffic fraction',
      descriptionKey:
        'Traffic reserved for learning about alternatives. It must be above zero when enabled.',
      min: 0,
      max: 0.999,
      step: 0.001,
    },
  ]

export const DYNAMIC_ROUTING_SENSITIVITY_FIELDS: DynamicRoutingNumericFieldDefinition[] =
  [
    {
      name: 'dynamic_routing_setting.recovery_threshold',
      labelKey: 'Recovery threshold',
      descriptionKey:
        'Relative score below which a channel may begin recovering traffic.',
      min: 1.01,
      step: 0.01,
    },
    {
      name: 'dynamic_routing_setting.degradation_threshold',
      labelKey: 'Degradation threshold',
      descriptionKey:
        'Relative score at which traffic starts moving away from a channel.',
      min: 1.01,
      step: 0.01,
    },
    {
      name: 'dynamic_routing_setting.critical_threshold',
      labelKey: 'Critical threshold',
      descriptionKey: 'Relative score that triggers rapid traffic shedding.',
      min: 1.01,
      step: 0.01,
    },
    {
      name: 'dynamic_routing_setting.candidate_advantage',
      labelKey: 'Candidate advantage',
      descriptionKey:
        'Required relative advantage before promoting another channel.',
      min: 1.01,
      step: 0.01,
    },
    {
      name: 'dynamic_routing_setting.aggressiveness',
      labelKey: 'Aggressiveness',
      descriptionKey:
        'How quickly traffic leaves a degraded channel, from greater than 0 through 1.',
      min: 0.01,
      max: 1,
      step: 0.01,
    },
  ]

export const DYNAMIC_ROUTING_RECOVERY_FIELDS: DynamicRoutingNumericFieldDefinition[] =
  [
    {
      name: 'dynamic_routing_setting.recovery_step',
      labelKey: 'Recovery step',
      descriptionKey:
        'Maximum traffic share restored per recovery step, from greater than 0 through 1.',
      min: 0.001,
      max: 1,
      step: 0.001,
    },
    {
      name: 'dynamic_routing_setting.cooldown_seconds',
      labelKey: 'Switch cooldown (seconds)',
      descriptionKey:
        'Minimum stabilization time between ordinary allocation changes.',
      min: 0,
      max: MAX_DYNAMIC_ROUTING_DURATION_SECONDS,
      step: 1,
    },
    {
      name: 'dynamic_routing_setting.hard_failure_threshold',
      labelKey: 'Hard failure threshold',
      descriptionKey:
        'Recent hard failures required before temporarily ejecting a channel.',
      min: 1,
      step: 1,
    },
    {
      name: 'dynamic_routing_setting.hard_failure_cooldown_seconds',
      labelKey: 'Hard failure cooldown (seconds)',
      descriptionKey:
        'How long an ejected channel waits before receiving a small recovery probe.',
      min: 1,
      max: MAX_DYNAMIC_ROUTING_DURATION_SECONDS,
      step: 1,
    },
  ]
