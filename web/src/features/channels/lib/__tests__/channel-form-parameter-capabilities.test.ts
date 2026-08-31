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

import type { Channel } from '../../types'
import {
  CHANNEL_FORM_DEFAULT_VALUES,
  transformChannelToFormDefaults,
  transformFormDataToCreatePayload,
} from '../channel-form'

const parameterCapabilities = {
  defaults: {
    temperature: { min: 0, max: 1, on_violation: 'reject' },
  },
}

describe('channel form parameter capability persistence', () => {
  it('stores capability configuration in channel settings without discarding existing settings', () => {
    const payload = transformFormDataToCreatePayload({
      ...CHANNEL_FORM_DEFAULT_VALUES,
      name: 'Capability channel',
      key: 'test-key',
      models: 'model-a',
      settings: JSON.stringify({ disable_store: true }),
      disable_store: true,
      parameter_capabilities: JSON.stringify(parameterCapabilities),
    })

    const settings = JSON.parse(payload.channel.settings || '{}')
    expect(settings.disable_store).toBe(true)
    expect(settings.parameter_capabilities).toEqual(parameterCapabilities)
  })

  it('removes obsolete video capability settings when saving a channel', () => {
    const payload = transformFormDataToCreatePayload({
      ...CHANNEL_FORM_DEFAULT_VALUES,
      name: 'Migrated capability channel',
      key: 'test-key',
      models: 'video-model',
      disable_store: true,
      settings: JSON.stringify({
        disable_store: true,
        video_capabilities: {
          models: { 'video-model': { resolutions: ['1080p'] } },
        },
      }),
    })

    const settings = JSON.parse(payload.channel.settings || '{}')
    expect(settings.disable_store).toBe(true)
    expect(settings).not.toHaveProperty('video_capabilities')
  })

  it('loads persisted capability configuration into the visual editor field', () => {
    const channel: Channel = {
      id: 1,
      type: 1,
      key: '',
      status: 1,
      name: 'Capability channel',
      created_time: 0,
      test_time: 0,
      response_time: 0,
      other: '',
      balance: 0,
      balance_updated_time: 0,
      models: 'model-a',
      group: 'default',
      used_quota: 0,
      priority: 0,
      auto_ban: 1,
      other_info: '',
      remark: '',
      max_input_tokens: 0,
      channel_info: {
        is_multi_key: false,
        multi_key_size: 0,
        multi_key_polling_index: 0,
        multi_key_mode: 'random',
      },
      settings: JSON.stringify({
        parameter_capabilities: parameterCapabilities,
      }),
    }

    const defaults = transformChannelToFormDefaults(channel)

    expect(JSON.parse(defaults.parameter_capabilities || '{}')).toEqual(
      parameterCapabilities
    )
  })
})
