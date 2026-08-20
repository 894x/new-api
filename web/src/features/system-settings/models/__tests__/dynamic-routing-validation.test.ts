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
import assert from 'node:assert/strict'
import { describe, test } from 'node:test'

import { DYNAMIC_ROUTING_FLAT_DEFAULTS } from '../../types'
import {
  buildDynamicRoutingFormValues,
  dynamicRoutingFormSchema,
  MAX_DYNAMIC_ROUTING_DURATION_SECONDS,
} from '../lib/dynamic-routing-schema'

function buildValidValues() {
  return buildDynamicRoutingFormValues(DYNAMIC_ROUTING_FLAT_DEFAULTS)
}

describe('dynamic routing settings validation', () => {
  test('accepts the tuned disabled-by-default configuration', () => {
    const values = buildValidValues()
    const result = dynamicRoutingFormSchema.safeParse(values)

    assert.equal(result.success, true)
    assert.deepEqual(values.dynamic_routing_setting, {
      enabled: false,
      max_samples: 60,
      max_age_seconds: 90,
      min_samples: 3,
      probe_fraction: 0.015,
      degradation_threshold: 1.3,
      recovery_threshold: 1.1,
      critical_threshold: 1.9,
      candidate_advantage: 1.1,
      aggressiveness: 0.9,
      recovery_step: 0.02,
      cooldown_seconds: 3,
      hard_failure_threshold: 1,
      hard_failure_cooldown_seconds: 30,
    })
  })

  test('accepts both enabled-positive-probe and disabled-zero-probe states', () => {
    const disabled = buildValidValues()
    disabled.dynamic_routing_setting.probe_fraction = 0
    assert.equal(dynamicRoutingFormSchema.safeParse(disabled).success, true)

    const enabled = buildValidValues()
    enabled.dynamic_routing_setting.enabled = true
    assert.equal(dynamicRoutingFormSchema.safeParse(enabled).success, true)
  })

  test('rejects a zero probe fraction when routing is enabled', () => {
    const enabled = buildValidValues()
    enabled.dynamic_routing_setting.enabled = true
    enabled.dynamic_routing_setting.probe_fraction = 0
    const result = dynamicRoutingFormSchema.safeParse(enabled)

    assert.equal(result.success, false)
    if (result.success) return
    assert.deepEqual(result.error.issues[0]?.path, [
      'dynamic_routing_setting',
      'probe_fraction',
    ])
  })

  test('rejects a minimum sample count above the retained sample count', () => {
    const values = buildValidValues()
    values.dynamic_routing_setting.min_samples = 61
    const result = dynamicRoutingFormSchema.safeParse(values)

    assert.equal(result.success, false)
    if (result.success) return
    assert.deepEqual(result.error.issues[0]?.path, [
      'dynamic_routing_setting',
      'min_samples',
    ])
  })

  test('enforces recovery below degradation below critical thresholds', () => {
    const values = buildValidValues()
    values.dynamic_routing_setting.recovery_threshold = 1.4
    values.dynamic_routing_setting.degradation_threshold = 1.3
    values.dynamic_routing_setting.critical_threshold = 1.2
    const result = dynamicRoutingFormSchema.safeParse(values)

    assert.equal(result.success, false)
    if (result.success) return
    const issuePaths = new Set(
      result.error.issues.map((issue) => issue.path.join('.'))
    )
    assert.equal(
      issuePaths.has('dynamic_routing_setting.recovery_threshold'),
      true
    )
    assert.equal(
      issuePaths.has('dynamic_routing_setting.critical_threshold'),
      true
    )
  })

  test('rejects fractions outside their supported ranges', () => {
    const values = buildValidValues()
    values.dynamic_routing_setting.probe_fraction = 1
    values.dynamic_routing_setting.aggressiveness = 0
    values.dynamic_routing_setting.recovery_step = 1.1
    const result = dynamicRoutingFormSchema.safeParse(values)

    assert.equal(result.success, false)
    if (result.success) return
    const issuePaths = new Set(
      result.error.issues.map((issue) => issue.path.join('.'))
    )
    assert.equal(issuePaths.has('dynamic_routing_setting.probe_fraction'), true)
    assert.equal(issuePaths.has('dynamic_routing_setting.aggressiveness'), true)
    assert.equal(issuePaths.has('dynamic_routing_setting.recovery_step'), true)
  })

  test('aligns accepted decimal precision with numeric input steps', () => {
    const values = buildValidValues()
    values.dynamic_routing_setting.probe_fraction = 0.9995
    values.dynamic_routing_setting.degradation_threshold = 1.305
    values.dynamic_routing_setting.aggressiveness = 0.905
    values.dynamic_routing_setting.recovery_step = 0.0205
    const result = dynamicRoutingFormSchema.safeParse(values)

    assert.equal(result.success, false)
    if (result.success) return
    const issuePaths = new Set(
      result.error.issues.map((issue) => issue.path.join('.'))
    )
    assert.equal(issuePaths.has('dynamic_routing_setting.probe_fraction'), true)
    assert.equal(
      issuePaths.has('dynamic_routing_setting.degradation_threshold'),
      true
    )
    assert.equal(issuePaths.has('dynamic_routing_setting.aggressiveness'), true)
    assert.equal(issuePaths.has('dynamic_routing_setting.recovery_step'), true)
  })

  test('aligns exclusive lower bounds with the smallest input step', () => {
    const values = buildValidValues()
    values.dynamic_routing_setting.recovery_threshold = 1.009
    values.dynamic_routing_setting.candidate_advantage = 1.009
    values.dynamic_routing_setting.aggressiveness = 0.009
    values.dynamic_routing_setting.recovery_step = 0.0009
    const result = dynamicRoutingFormSchema.safeParse(values)

    assert.equal(result.success, false)
    if (result.success) return
    const issuePaths = new Set(
      result.error.issues.map((issue) => issue.path.join('.'))
    )
    assert.equal(
      issuePaths.has('dynamic_routing_setting.recovery_threshold'),
      true
    )
    assert.equal(
      issuePaths.has('dynamic_routing_setting.candidate_advantage'),
      true
    )
    assert.equal(issuePaths.has('dynamic_routing_setting.aggressiveness'), true)
    assert.equal(issuePaths.has('dynamic_routing_setting.recovery_step'), true)
  })

  test('requires integral sample counts and valid durations', () => {
    const values = buildValidValues()
    values.dynamic_routing_setting.max_samples = 10.5
    values.dynamic_routing_setting.max_age_seconds = 0
    values.dynamic_routing_setting.cooldown_seconds = -1
    values.dynamic_routing_setting.hard_failure_cooldown_seconds = 0
    const result = dynamicRoutingFormSchema.safeParse(values)

    assert.equal(result.success, false)
    if (result.success) return
    const issuePaths = new Set(
      result.error.issues.map((issue) => issue.path.join('.'))
    )
    assert.equal(issuePaths.has('dynamic_routing_setting.max_samples'), true)
    assert.equal(
      issuePaths.has('dynamic_routing_setting.max_age_seconds'),
      true
    )
    assert.equal(
      issuePaths.has('dynamic_routing_setting.cooldown_seconds'),
      true
    )
    assert.equal(
      issuePaths.has('dynamic_routing_setting.hard_failure_cooldown_seconds'),
      true
    )
  })

  test('rejects durations that overflow the backend time representation', () => {
    const values = buildValidValues()
    values.dynamic_routing_setting.max_age_seconds =
      MAX_DYNAMIC_ROUTING_DURATION_SECONDS + 1
    const result = dynamicRoutingFormSchema.safeParse(values)

    assert.equal(result.success, false)
    if (result.success) return
    assert.deepEqual(result.error.issues[0]?.path, [
      'dynamic_routing_setting',
      'max_age_seconds',
    ])
  })

  test('keeps the hard-failure threshold within the retained sample count', () => {
    const values = buildValidValues()
    values.dynamic_routing_setting.hard_failure_threshold = 61
    const result = dynamicRoutingFormSchema.safeParse(values)

    assert.equal(result.success, false)
    if (result.success) return
    assert.deepEqual(result.error.issues[0]?.path, [
      'dynamic_routing_setting',
      'hard_failure_threshold',
    ])
  })
})
