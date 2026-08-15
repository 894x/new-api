/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.
*/
import type { VideoCapabilityConfig } from '../types'

const RESOLUTION_LABEL_PATTERN = /^[1-9][0-9]{2,3}p$|^[1-9][0-9]*k$/
const RESOLUTION_SIZE_PATTERN = /^([1-9][0-9]{1,4})x([1-9][0-9]{1,4})$/

export interface VideoCapabilityConfigError {
  model: string
  message: string
  values?: Record<string, string | number>
}

export function normalizeVideoResolution(value: string): string | null {
  const normalized = value
    .trim()
    .toLowerCase()
    .replaceAll('*', 'x')
    .replaceAll(' ', '')
  if (!normalized || normalized.length > 32) return null
  if (RESOLUTION_LABEL_PATTERN.test(normalized)) return normalized

  const match = normalized.match(RESOLUTION_SIZE_PATTERN)
  if (!match) return null
  const width = Number(match[1])
  const height = Number(match[2])
  const longEdge = Math.max(width, height)
  const shortEdge = Math.min(width, height)
  if (longEdge === 640 && shortEdge === 360) return '360p'
  if (longEdge === 854 && shortEdge === 480) return '480p'
  if (longEdge === 960 && shortEdge === 540) return '540p'
  if (longEdge === 1280 && shortEdge === 720) return '720p'
  if (longEdge === 1920 && shortEdge === 1080) return '1080p'
  if (longEdge === 3840 && shortEdge === 2160) return '4k'
  return `${longEdge}x${shortEdge}`
}

export function parseVideoCapabilityConfig(
  value: string
): VideoCapabilityConfig {
  if (!value.trim()) return { models: {} }
  try {
    const parsed = JSON.parse(value) as VideoCapabilityConfig
    return parsed && typeof parsed === 'object' && !Array.isArray(parsed)
      ? { models: parsed.models || {} }
      : { models: {} }
  } catch {
    return { models: {} }
  }
}

export function validateVideoCapabilityConfig(
  config: VideoCapabilityConfig
): VideoCapabilityConfigError[] {
  const errors: VideoCapabilityConfigError[] = []
  const entries = Object.entries(config.models || {})
  if (entries.length > 512) {
    errors.push({ model: '', message: 'Too many video model rules.' })
  }
  for (const [model, capability] of entries) {
    const resolutions = capability.resolutions || []
    if (!model.trim()) {
      errors.push({ model, message: 'Model name is required.' })
    }
    if (resolutions.length === 0) {
      errors.push({ model, message: 'Add at least one resolution.' })
      continue
    }
    if (resolutions.length > 32) {
      errors.push({
        model,
        message: 'A model can have at most 32 resolutions.',
      })
      continue
    }
    const seen = new Set<string>()
    for (const value of resolutions) {
      const resolution = normalizeVideoResolution(value)
      if (!resolution) {
        errors.push({
          model,
          message: 'Invalid resolution: {{resolution}}',
          values: { resolution: value },
        })
        continue
      }
      if (seen.has(resolution)) {
        errors.push({
          model,
          message: 'Duplicate resolution: {{resolution}}',
          values: { resolution },
        })
      }
      seen.add(resolution)
    }
  }
  return errors
}

export function stringifyVideoCapabilityConfig(
  config: VideoCapabilityConfig
): string {
  const models = Object.fromEntries(
    Object.entries(config.models || {}).map(([model, capability]) => [
      model.trim(),
      {
        resolutions: (capability.resolutions || []).map(
          (value) => normalizeVideoResolution(value) || value.trim()
        ),
      },
    ])
  )
  return Object.keys(models).length > 0
    ? JSON.stringify({ models }, null, 2)
    : ''
}
