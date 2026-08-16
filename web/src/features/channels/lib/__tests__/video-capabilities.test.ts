/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.
*/
import { describe, expect, it } from 'vitest'

import {
  normalizeVideoResolution,
  parseVideoCapabilityConfig,
  stringifyVideoCapabilityConfig,
  validateVideoCapabilityConfig,
} from '../video-capabilities'

describe('video capabilities', () => {
  it('normalizes labels and dimensions using the backend contract', () => {
    expect(normalizeVideoResolution('1080P')).toBe('1080p')
    expect(normalizeVideoResolution('640x360')).toBe('360p')
    expect(normalizeVideoResolution('854x480')).toBe('480p')
    expect(normalizeVideoResolution('960x540')).toBe('540p')
    expect(normalizeVideoResolution('1920 x 1080')).toBe('1080p')
    expect(normalizeVideoResolution('1080*1920')).toBe('1080p')
    expect(normalizeVideoResolution('3840x2160')).toBe('4k')
    expect(normalizeVideoResolution('1792x1024')).toBe('1792x1024')
    expect(normalizeVideoResolution('high')).toBeNull()
  })

  it('rejects empty and duplicate normalized allowlists', () => {
    expect(
      validateVideoCapabilityConfig({
        models: { 'video-model': { resolutions: [] } },
      })
    ).not.toHaveLength(0)
    expect(
      validateVideoCapabilityConfig({
        models: {
          'video-model': { resolutions: ['1080P', '1920x1080'] },
        },
      })
    ).not.toHaveLength(0)
  })

  it('round trips a per-model resolution allowlist', () => {
    const value = stringifyVideoCapabilityConfig({
      models: { 'video-model': { resolutions: ['720p', '1080p'] } },
    })

    expect(parseVideoCapabilityConfig(value)).toEqual({
      models: { 'video-model': { resolutions: ['720p', '1080p'] } },
    })
  })
})
