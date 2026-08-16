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

import type { ModelRoutingOverride } from '../../types'
import {
  collectChangedModelRoutingOverrides,
  createModelRoutingOverrideDraftState,
  createModelRoutingOverrideDrafts,
  ensureSuccessfulModelRoutingOverridesResponse,
  getModelRoutingOverrideDirtyFieldKey,
  getModelRoutingOverrideKey,
  mergeModelRoutingOverrideDraftState,
  parseRoutingOverrideInput,
  updateModelRoutingOverrideDraftField,
} from '../model-routing-overrides'

const row: ModelRoutingOverride = {
  channel_id: 7,
  channel_name: 'Primary channel',
  channel_type: 1,
  channel_status: 1,
  model: 'gpt-test',
  default_priority: 10,
  default_weight: 20,
  priority_override: null,
  weight_override: 3,
  effective_priority: 10,
  effective_weight: 3,
}

describe('model routing override serialization', () => {
  it('distinguishes inherited, explicit zero, and non-zero values', () => {
    expect(parseRoutingOverrideInput('')).toBeNull()
    expect(parseRoutingOverrideInput('0')).toBe(0)
    expect(parseRoutingOverrideInput(' -4 ')).toBe(-4)
  })

  it('rejects non-integer input instead of converting it into inheritance', () => {
    expect(parseRoutingOverrideInput('1.5')).toBeUndefined()
    expect(parseRoutingOverrideInput('not-a-number')).toBeUndefined()
  })

  it('emits only changed pairs and preserves explicit zero and null resets', () => {
    const drafts = createModelRoutingOverrideDrafts([row])
    drafts[getModelRoutingOverrideKey(row.channel_id, row.model)] = {
      priority_override: '0',
      weight_override: '',
    }

    expect(collectChangedModelRoutingOverrides([row], drafts)).toEqual({
      overrides: [
        {
          channel_id: 7,
          model: 'gpt-test',
          priority_override: 0,
          weight_override: null,
        },
      ],
      errors: [],
    })
  })

  it('omits an unchanged pair from a patch', () => {
    const drafts = createModelRoutingOverrideDrafts([row])

    expect(collectChangedModelRoutingOverrides([row], drafts)).toEqual({
      overrides: [],
      errors: [],
    })
  })

  it('rejects a negative weight without serializing a partial patch', () => {
    const drafts = createModelRoutingOverrideDrafts([row])
    const key = getModelRoutingOverrideKey(row.channel_id, row.model)
    drafts[key] = {
      priority_override: '0',
      weight_override: '-1',
    }

    expect(collectChangedModelRoutingOverrides([row], drafts)).toEqual({
      overrides: [],
      errors: [{ key, field: 'weight_override' }],
    })
  })

  it('rejects a weight above the backend int32 limit', () => {
    const drafts = createModelRoutingOverrideDrafts([row])
    const key = getModelRoutingOverrideKey(row.channel_id, row.model)
    drafts[key] = {
      priority_override: '',
      weight_override: '2147483638',
    }

    expect(collectChangedModelRoutingOverrides([row], drafts)).toEqual({
      overrides: [],
      errors: [{ key, field: 'weight_override' }],
    })
  })

  it.each([
    {
      dirtyField: 'priority_override' as const,
      expectedDraft: { priority_override: '0', weight_override: '12' },
      expectedPatch: { priority_override: 0, weight_override: 12 },
    },
    {
      dirtyField: 'weight_override' as const,
      expectedDraft: { priority_override: '9', weight_override: '0' },
      expectedPatch: { priority_override: 9, weight_override: 0 },
    },
  ])(
    'keeps dirty $dirtyField while refreshing and patching the other field',
    ({ dirtyField, expectedDraft, expectedPatch }) => {
      const key = getModelRoutingOverrideKey(row.channel_id, row.model)
      let state = createModelRoutingOverrideDraftState([row])
      state = updateModelRoutingOverrideDraftField(state, row, dirtyField, '0')
      const refreshedRows = [
        { ...row, priority_override: 9, weight_override: 12 },
      ]

      state = mergeModelRoutingOverrideDraftState(refreshedRows, state)

      expect(state.drafts[key]).toEqual(expectedDraft)
      expect(state.dirtyFields).toEqual(
        new Set([getModelRoutingOverrideDirtyFieldKey(key, dirtyField)])
      )
      expect(
        collectChangedModelRoutingOverrides(refreshedRows, state.drafts)
      ).toEqual({
        overrides: [
          {
            channel_id: row.channel_id,
            model: row.model,
            ...expectedPatch,
          },
        ],
        errors: [],
      })
    }
  )

  it('clears field dirty state when the draft changes back to the server value', () => {
    const key = getModelRoutingOverrideKey(row.channel_id, row.model)
    let state = createModelRoutingOverrideDraftState([row])
    state = updateModelRoutingOverrideDraftField(
      state,
      row,
      'weight_override',
      '0'
    )
    expect(state.dirtyFields).toEqual(
      new Set([getModelRoutingOverrideDirtyFieldKey(key, 'weight_override')])
    )

    state = updateModelRoutingOverrideDraftField(
      state,
      row,
      'weight_override',
      '3'
    )

    expect(state.drafts[key]?.weight_override).toBe('3')
    expect(state.dirtyFields).toEqual(new Set())
  })

  it('compares later edits with the latest server value after a refetch', () => {
    const key = getModelRoutingOverrideKey(row.channel_id, row.model)
    let state = createModelRoutingOverrideDraftState([row])
    state = updateModelRoutingOverrideDraftField(
      state,
      row,
      'priority_override',
      '0'
    )
    state = mergeModelRoutingOverrideDraftState(
      [{ ...row, priority_override: 9 }],
      state
    )

    state = updateModelRoutingOverrideDraftField(
      state,
      row,
      'priority_override',
      '9'
    )

    expect(state.drafts[key]?.priority_override).toBe('9')
    expect(state.dirtyFields).toEqual(new Set())
  })

  it('throws when an HTTP 200 response reports a business failure', () => {
    expect(() =>
      ensureSuccessfulModelRoutingOverridesResponse({
        success: false,
        message: 'routing lookup failed',
      })
    ).toThrow('routing lookup failed')
  })
})
