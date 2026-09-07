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
import { describe, it, expect } from 'vitest'

import type { ModelRoutingOverride } from '../../types'
import {
  CHANNEL_FORM_DEFAULT_VALUES,
  channelFormSchema,
  transformChannelToFormDefaults,
  transformFormDataToCreatePayload,
  transformFormDataToUpdatePayload,
} from '../channel-form'
import {
  createModelRoutingOverrideDrafts,
  collectChangedModelRoutingOverrides,
  getModelRoutingOverrideKey,
} from '../model-routing-overrides'

describe('channel capacity configuration', () => {
  it('round-trips defaults and rejects negative and unsafe limits', () => {
    const form = {
      ...CHANNEL_FORM_DEFAULT_VALUES,
      name: 'Capacity channel',
      key: 'fixture',
      models: 'gpt-test',
      group: ['default'],
      rpm: 60,
      tpm: 6000,
    }
    expect(channelFormSchema.safeParse(form).success).toBe(true)
    expect(channelFormSchema.safeParse({ ...form, rpm: -1 }).success).toBe(
      false
    )
    expect(
      channelFormSchema.safeParse({ ...form, tpm: Number.MAX_SAFE_INTEGER + 1 })
        .success
    ).toBe(false)
    const channel = transformFormDataToCreatePayload(form).channel
    expect(channel).toMatchObject({ rpm: 60, tpm: 6000 })
    expect(
      transformChannelToFormDefaults({
        ...channel,
        id: 1,
        channel_info: {
          is_multi_key: false,
          multi_key_size: 1,
          multi_key_mode: 'random',
        },
      } as Parameters<typeof transformChannelToFormDefaults>[0])
    ).toMatchObject({ rpm: 60, tpm: 6000 })
    expect(
      transformFormDataToUpdatePayload({ ...form, rpm: 0, tpm: 0 }, 1)
    ).toMatchObject({ rpm: 0, tpm: 0 })
  })
  it('serializes capacity inheritance, explicit zero, and rejects invalid partial patches', () => {
    const row: ModelRoutingOverride = {
      channel_id: 1,
      channel_name: 'Channel',
      channel_type: 1,
      channel_status: 1,
      model: 'm',
      default_priority: 0,
      default_weight: 0,
      priority_override: null,
      weight_override: null,
      effective_priority: 0,
      effective_weight: 0,
      default_rpm: 60,
      default_tpm: 6000,
      rpm_override: 20,
      tpm_override: null,
      effective_rpm: 20,
      effective_tpm: 6000,
    }
    const drafts = createModelRoutingOverrideDrafts([row])
    const key = getModelRoutingOverrideKey(1, 'm')
    drafts[key].rpm_override = ''
    drafts[key].tpm_override = '0'
    expect(
      collectChangedModelRoutingOverrides([row], drafts).overrides[0]
    ).toMatchObject({ rpm_override: null, tpm_override: 0 })
    drafts[key].tpm_override = '-1'
    expect(collectChangedModelRoutingOverrides([row], drafts)).toEqual({
      overrides: [],
      errors: [{ key, field: 'tpm_override' }],
    })
  })
})
