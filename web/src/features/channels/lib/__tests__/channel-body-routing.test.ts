import { describe, expect, it } from 'vitest'
import type { Channel } from '../../types'
import {
  CHANNEL_FORM_DEFAULT_VALUES,
  buildSettingJSON,
  channelFormSchema,
  transformChannelToFormDefaults,
} from '../channel-form'

const values = {
  ...CHANNEL_FORM_DEFAULT_VALUES,
  name: 'Routing channel',
  key: 'test-key',
  models: 'test-model',
}
const channel: Channel = {
  settings: '{}',
  id: 15,
  type: 1,
  key: '',
  status: 1,
  name: 'Routing channel',
  created_time: 0,
  test_time: 0,
  response_time: 0,
  other: '',
  balance: 0,
  balance_updated_time: 0,
  models: 'test-model',
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
}

describe('large-body routing settings', () => {
  it('is disabled for legacy channels and omits new fields by default', () => {
    expect(JSON.parse(buildSettingJSON(values))).not.toHaveProperty(
      'http1_large_body_enabled'
    )
    expect(
      transformChannelToFormDefaults({ ...channel, setting: '{}' })
        .http1_large_body_enabled
    ).toBe(false)
  })
  it('saves byte thresholds and restores the KiB editor without changing shards', () => {
    const setting = buildSettingJSON({
      ...values,
      http2_connection_shards: 6,
      http1_large_body_enabled: true,
      http1_large_body_threshold_kib: 2048,
    })
    expect(JSON.parse(setting)).toMatchObject({
      http2_connection_shards: 6,
      http1_large_body_enabled: true,
      http1_large_body_threshold_bytes: 2097152,
    })
    const reopened = transformChannelToFormDefaults({ ...channel, setting })
    expect(reopened).toMatchObject({
      http2_connection_shards: 6,
      http1_large_body_enabled: true,
      http1_large_body_threshold_kib: 2048,
    })
    expect(
      JSON.parse(
        buildSettingJSON({ ...reopened, http1_large_body_enabled: false })
      )
    ).not.toHaveProperty('http1_large_body_enabled')
  })
  it.each([undefined, 0, -1, 1.5, 1048577])(
    'rejects invalid enabled threshold %s',
    (threshold) => {
      const result = channelFormSchema.safeParse({
        ...values,
        http1_large_body_enabled: true,
        http1_large_body_threshold_kib: threshold,
      })
      expect(result.success).toBe(false)
      if (!result.success) {
        expect(
          result.error.issues.some(
            (issue) => issue.path[0] === 'http1_large_body_threshold_kib'
          )
        ).toBe(true)
      }
    }
  )
  it('does not serialize hybrid routing when HTTP/1.1 is forced', () => {
    const parsed = JSON.parse(
      buildSettingJSON({
        ...values,
        http_protocol: 'http1',
        http1_large_body_enabled: true,
        http1_large_body_threshold_kib: 1024,
      })
    )
    expect(parsed.http_protocol).toBe('http1')
    expect(parsed).not.toHaveProperty('http1_large_body_enabled')
  })
})
