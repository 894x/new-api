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
  channelFormSchema,
  transformChannelToFormDefaults,
  transformFormDataToCreatePayload,
} from '../channel-form'

describe('DoubaoVideo upstream API settings', () => {
  it('persists custom submit and fetch paths for DoubaoVideo channels', () => {
    const payload = transformFormDataToCreatePayload({
      ...CHANNEL_FORM_DEFAULT_VALUES,
      type: 54,
      name: 'Doubao video',
      key: 'sk-test',
      models: 'doubao-seedance-2-0-260128',
      settings: JSON.stringify({ disable_task_polling_sleep: true }),
      disable_task_polling_sleep: true,
      doubao_video_api_mode: 'custom',
      doubao_video_submit_path: '/custom/video/tasks',
      doubao_video_fetch_path: '/custom/video/tasks/{id}',
    })

    const settings = JSON.parse(payload.channel.settings || '{}')
    expect(settings).toMatchObject({
      disable_task_polling_sleep: true,
      doubao_video_api_mode: 'custom',
      doubao_video_submit_path: '/custom/video/tasks',
      doubao_video_fetch_path: '/custom/video/tasks/{id}',
    })
  })

  it('loads persisted custom paths into the channel form', () => {
    const channel = {
      id: 54,
      type: 54,
      key: '',
      status: 1,
      name: 'Doubao video',
      created_time: 0,
      test_time: 0,
      response_time: 0,
      other: '',
      balance: 0,
      balance_updated_time: 0,
      models: 'doubao-seedance-2-0-260128',
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
        doubao_video_api_mode: 'custom',
        doubao_video_submit_path: '/custom/video/tasks',
        doubao_video_fetch_path: '/custom/video/tasks/{id}',
      }),
    } satisfies Channel

    const defaults = transformChannelToFormDefaults(channel)

    expect(defaults.doubao_video_api_mode).toBe('custom')
    expect(defaults.doubao_video_submit_path).toBe('/custom/video/tasks')
    expect(defaults.doubao_video_fetch_path).toBe('/custom/video/tasks/{id}')
  })

  it('removes DoubaoVideo-only settings from other channel types', () => {
    const payload = transformFormDataToCreatePayload({
      ...CHANNEL_FORM_DEFAULT_VALUES,
      type: 1,
      name: 'OpenAI',
      key: 'sk-test',
      models: 'gpt-5',
      settings: JSON.stringify({
        doubao_video_api_mode: 'custom',
        doubao_video_submit_path: '/custom/video/tasks',
        doubao_video_fetch_path: '/custom/video/tasks/{id}',
      }),
      doubao_video_api_mode: 'custom',
      doubao_video_submit_path: '/custom/video/tasks',
      doubao_video_fetch_path: '/custom/video/tasks/{id}',
    })

    const settings = JSON.parse(payload.channel.settings || '{}')
    expect(settings.doubao_video_api_mode).toBeUndefined()
    expect(settings.doubao_video_submit_path).toBeUndefined()
    expect(settings.doubao_video_fetch_path).toBeUndefined()
  })

  it('removes stale custom paths when a built-in API mode is selected', () => {
    const payload = transformFormDataToCreatePayload({
      ...CHANNEL_FORM_DEFAULT_VALUES,
      type: 54,
      name: 'Doubao video',
      key: 'sk-test',
      models: 'doubao-seedance-2-0-260128',
      settings: JSON.stringify({
        doubao_video_api_mode: 'custom',
        doubao_video_submit_path: '/custom/video/tasks',
        doubao_video_fetch_path: '/custom/video/tasks/{id}',
      }),
      doubao_video_api_mode: 'video_generations',
      doubao_video_submit_path: '/custom/video/tasks',
      doubao_video_fetch_path: '/custom/video/tasks/{id}',
    })

    const settings = JSON.parse(payload.channel.settings || '{}')
    expect(settings.doubao_video_api_mode).toBe('video_generations')
    expect(settings.doubao_video_submit_path).toBeUndefined()
    expect(settings.doubao_video_fetch_path).toBeUndefined()
  })

  it('requires valid relative custom paths and an id placeholder', () => {
    const result = channelFormSchema.safeParse({
      ...CHANNEL_FORM_DEFAULT_VALUES,
      type: 54,
      doubao_video_api_mode: 'custom',
      doubao_video_submit_path: 'https://video.example/tasks',
      doubao_video_fetch_path: '/custom/video/tasks/result',
    })

    expect(result.success).toBe(false)
    if (result.success) return
    expect(result.error.issues.map((issue) => issue.path.join('.'))).toEqual(
      expect.arrayContaining([
        'doubao_video_submit_path',
        'doubao_video_fetch_path',
      ])
    )
  })
})
