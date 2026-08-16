/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.
*/
import { describe, expect, it } from 'vitest'

import type { Channel } from '../../types'
import {
  CHANNEL_FORM_DEFAULT_VALUES,
  transformChannelToFormDefaults,
  transformFormDataToCreatePayload,
} from '../channel-form'

const videoCapabilities = {
  models: {
    'video-model': { resolutions: ['720p', '1080p'] },
  },
}

describe('channel form video capability persistence', () => {
  it('stores video capabilities without discarding existing settings', () => {
    const payload = transformFormDataToCreatePayload({
      ...CHANNEL_FORM_DEFAULT_VALUES,
      name: 'Video channel',
      key: 'test-key',
      models: 'video-model',
      settings: JSON.stringify({ disable_store: true }),
      disable_store: true,
      video_capabilities: JSON.stringify(videoCapabilities),
    })

    const settings = JSON.parse(payload.channel.settings || '{}')
    expect(settings.disable_store).toBe(true)
    expect(settings.video_capabilities).toEqual(videoCapabilities)
  })

  it('loads persisted video capabilities into the editor field', () => {
    const channel = {
      id: 1,
      type: 1,
      key: '',
      status: 1,
      name: 'Video channel',
      created_time: 0,
      test_time: 0,
      response_time: 0,
      other: '',
      balance: 0,
      balance_updated_time: 0,
      models: 'video-model',
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
      settings: JSON.stringify({ video_capabilities: videoCapabilities }),
    } satisfies Channel

    const defaults = transformChannelToFormDefaults(channel)
    expect(JSON.parse(defaults.video_capabilities || '{}')).toEqual(
      videoCapabilities
    )
  })
})
