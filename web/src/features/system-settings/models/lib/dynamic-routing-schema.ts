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

import {
  DYNAMIC_ROUTING_FLAT_DEFAULTS,
  type DynamicRoutingFlatSettings,
} from '../../types'

const positiveInteger = z
  .number()
  .int('Enter a whole number')
  .positive('Enter a number greater than zero')
  .max(Number.MAX_SAFE_INTEGER, 'Number is too large')

const nonNegativeInteger = z
  .number()
  .int('Enter a whole number')
  .min(0, 'Enter a non-negative number')
  .max(Number.MAX_SAFE_INTEGER, 'Number is too large')

export const MAX_DYNAMIC_ROUTING_DURATION_SECONDS = 9_223_372_036

const positiveDurationSeconds = positiveInteger.max(
  MAX_DYNAMIC_ROUTING_DURATION_SECONDS,
  'Duration is too large'
)

const nonNegativeDurationSeconds = nonNegativeInteger.max(
  MAX_DYNAMIC_ROUTING_DURATION_SECONDS,
  'Duration is too large'
)

const dynamicRoutingSettingsSchema = z
  .object({
    enabled: z.boolean(),
    max_samples: positiveInteger,
    max_age_seconds: positiveDurationSeconds,
    min_samples: positiveInteger,
    probe_fraction: z
      .number()
      .min(0, 'Probe fraction must be at least 0')
      .max(0.999, 'Probe fraction must be at most 0.999')
      .multipleOf(0.001, 'Use no more than three decimal places'),
    degradation_threshold: z
      .number()
      .min(1.01, 'Threshold must be at least 1.01')
      .multipleOf(0.01, 'Use no more than two decimal places'),
    recovery_threshold: z
      .number()
      .min(1.01, 'Threshold must be at least 1.01')
      .multipleOf(0.01, 'Use no more than two decimal places'),
    critical_threshold: z
      .number()
      .min(1.01, 'Threshold must be at least 1.01')
      .multipleOf(0.01, 'Use no more than two decimal places'),
    candidate_advantage: z
      .number()
      .min(1.01, 'Candidate advantage must be at least 1.01')
      .multipleOf(0.01, 'Use no more than two decimal places'),
    aggressiveness: z
      .number()
      .min(0.01, 'Aggressiveness must be at least 0.01')
      .max(1, 'Aggressiveness must be 1 or less')
      .multipleOf(0.01, 'Use no more than two decimal places'),
    recovery_step: z
      .number()
      .min(0.001, 'Recovery step must be at least 0.001')
      .max(1, 'Recovery step must be 1 or less')
      .multipleOf(0.001, 'Use no more than three decimal places'),
    cooldown_seconds: nonNegativeDurationSeconds,
    hard_failure_threshold: positiveInteger,
    hard_failure_cooldown_seconds: positiveDurationSeconds,
  })
  .superRefine((values, context) => {
    if (values.enabled && values.probe_fraction === 0) {
      context.addIssue({
        code: 'custom',
        path: ['probe_fraction'],
        message: 'Probe fraction must be greater than 0 when enabled',
      })
    }

    if (values.min_samples > values.max_samples) {
      context.addIssue({
        code: 'custom',
        path: ['min_samples'],
        message: 'Minimum samples cannot exceed maximum samples',
      })
    }

    if (values.hard_failure_threshold > values.max_samples) {
      context.addIssue({
        code: 'custom',
        path: ['hard_failure_threshold'],
        message: 'Hard failure threshold cannot exceed maximum samples',
      })
    }

    if (values.recovery_threshold >= values.degradation_threshold) {
      context.addIssue({
        code: 'custom',
        path: ['recovery_threshold'],
        message: 'Recovery threshold must be below degradation threshold',
      })
    }

    if (values.degradation_threshold >= values.critical_threshold) {
      context.addIssue({
        code: 'custom',
        path: ['critical_threshold'],
        message: 'Critical threshold must exceed degradation threshold',
      })
    }
  })

export const dynamicRoutingFormSchema = z.object({
  dynamic_routing_setting: dynamicRoutingSettingsSchema,
})

export type DynamicRoutingFormValues = z.infer<typeof dynamicRoutingFormSchema>

export function buildDynamicRoutingFormValues(
  values: DynamicRoutingFlatSettings
): DynamicRoutingFormValues {
  return {
    dynamic_routing_setting: {
      enabled:
        values['dynamic_routing_setting.enabled'] ??
        DYNAMIC_ROUTING_FLAT_DEFAULTS['dynamic_routing_setting.enabled'],
      max_samples:
        values['dynamic_routing_setting.max_samples'] ??
        DYNAMIC_ROUTING_FLAT_DEFAULTS['dynamic_routing_setting.max_samples'],
      max_age_seconds:
        values['dynamic_routing_setting.max_age_seconds'] ??
        DYNAMIC_ROUTING_FLAT_DEFAULTS[
          'dynamic_routing_setting.max_age_seconds'
        ],
      min_samples:
        values['dynamic_routing_setting.min_samples'] ??
        DYNAMIC_ROUTING_FLAT_DEFAULTS['dynamic_routing_setting.min_samples'],
      probe_fraction:
        values['dynamic_routing_setting.probe_fraction'] ??
        DYNAMIC_ROUTING_FLAT_DEFAULTS['dynamic_routing_setting.probe_fraction'],
      degradation_threshold:
        values['dynamic_routing_setting.degradation_threshold'] ??
        DYNAMIC_ROUTING_FLAT_DEFAULTS[
          'dynamic_routing_setting.degradation_threshold'
        ],
      recovery_threshold:
        values['dynamic_routing_setting.recovery_threshold'] ??
        DYNAMIC_ROUTING_FLAT_DEFAULTS[
          'dynamic_routing_setting.recovery_threshold'
        ],
      critical_threshold:
        values['dynamic_routing_setting.critical_threshold'] ??
        DYNAMIC_ROUTING_FLAT_DEFAULTS[
          'dynamic_routing_setting.critical_threshold'
        ],
      candidate_advantage:
        values['dynamic_routing_setting.candidate_advantage'] ??
        DYNAMIC_ROUTING_FLAT_DEFAULTS[
          'dynamic_routing_setting.candidate_advantage'
        ],
      aggressiveness:
        values['dynamic_routing_setting.aggressiveness'] ??
        DYNAMIC_ROUTING_FLAT_DEFAULTS['dynamic_routing_setting.aggressiveness'],
      recovery_step:
        values['dynamic_routing_setting.recovery_step'] ??
        DYNAMIC_ROUTING_FLAT_DEFAULTS['dynamic_routing_setting.recovery_step'],
      cooldown_seconds:
        values['dynamic_routing_setting.cooldown_seconds'] ??
        DYNAMIC_ROUTING_FLAT_DEFAULTS[
          'dynamic_routing_setting.cooldown_seconds'
        ],
      hard_failure_threshold:
        values['dynamic_routing_setting.hard_failure_threshold'] ??
        DYNAMIC_ROUTING_FLAT_DEFAULTS[
          'dynamic_routing_setting.hard_failure_threshold'
        ],
      hard_failure_cooldown_seconds:
        values['dynamic_routing_setting.hard_failure_cooldown_seconds'] ??
        DYNAMIC_ROUTING_FLAT_DEFAULTS[
          'dynamic_routing_setting.hard_failure_cooldown_seconds'
        ],
    },
  }
}
