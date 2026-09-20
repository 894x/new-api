import { describe, expect, it } from 'vitest'

import type { ParameterCapabilityConfig } from '../../types'
import {
  evaluateParameterCapabilities,
  parseParameterCapabilityConfigStrict,
  resolveParameterCapabilities,
  stringifyParameterCapabilityConfig,
  validateParameterCapabilityConfig,
} from '../parameter-capabilities'

const path = 'messages.*.content.*.video_url'

describe('video delivery capabilities', () => {
  it('round-trips formats and deep-merges explicit false without losing inherited byte limits', () => {
    const config: ParameterCapabilityConfig = {
      defaults: {
        [path]: {
          media: {
            kind: 'video',
            formats: {
              url: { supported: true, max_media_bytes: 500 },
              base64: { supported: true, max_media_bytes: 20 },
            },
            conversions: { url_to_base64: true, base64_to_url: true },
          },
        },
      },
      rules: [
        {
          selector: { type: 'exact', value: 'kimi-k3' },
          parameters: {
            [path]: {
              media: {
                formats: { base64: { supported: false } },
                conversions: { base64_to_url: false },
              },
            },
          },
        },
      ],
    }
    const parsed = parseParameterCapabilityConfigStrict(
      stringifyParameterCapabilityConfig(config)
    )
    expect(parsed.success).toBe(true)
    if (!parsed.success) throw new Error('invalid configuration')
    expect(validateParameterCapabilityConfig(parsed.config)).toEqual([])
    expect(
      resolveParameterCapabilities(parsed.config, 'kimi-k3')[path].capability
        .media
    ).toEqual({
      kind: 'video',
      formats: {
        url: { supported: true, max_media_bytes: 500 },
        base64: { supported: false, max_media_bytes: 20 },
      },
      conversions: { url_to_base64: true, base64_to_url: false },
    })
    expect(config.defaults?.[path].media?.formats?.base64?.supported).toBe(true)
  })

  it.each([0, -1, 1.5, 2 ** 40 + 1])(
    'rejects invalid byte limit %s',
    (limit) => {
      const draft = {
        defaults: {
          [path]: { media: { formats: { url: { max_media_bytes: limit } } } },
        },
      }
      expect(
        parseParameterCapabilityConfigStrict(JSON.stringify(draft)).success
      ).toBe(false)
      expect(validateParameterCapabilityConfig(draft)).not.toEqual([])
    }
  )

  it('does not claim server-dependent media admission succeeded or expose Base64 in evaluation details', () => {
    const config: ParameterCapabilityConfig = {
      defaults: { [path]: { media: { kind: 'video' } } },
    }
    const input = {
      messages: [
        { content: [{ video_url: { url: 'data:video/mp4;base64,AAAA' } }] },
      ],
    }
    const result = evaluateParameterCapabilities(config, 'kimi-k3', input)
    expect(result.evaluations).toEqual([
      {
        parameter: 'messages.0.content.0.video_url',
        status: 'pending',
        reason: 'media_validation_required',
      },
    ])
    expect(result.request).toEqual(input)
  })
})
