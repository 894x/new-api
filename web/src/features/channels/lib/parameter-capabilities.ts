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
import type { ParameterCapability, ParameterCapabilityConfig } from '../types'

export const PARAMETER_CAPABILITY_CATALOG = [
  { path: 'temperature', category: 'Sampling', kind: 'number' },
  { path: 'top_p', category: 'Sampling', kind: 'number' },
  { path: 'top_k', category: 'Sampling', kind: 'number' },
  { path: 'max_tokens', category: 'Output', kind: 'number' },
  { path: 'max_completion_tokens', category: 'Output', kind: 'number' },
  { path: 'max_output_tokens', category: 'Output', kind: 'number' },
  { path: 'n', category: 'Output', kind: 'number' },
  { path: 'stop', category: 'Output', kind: 'enum' },
  { path: 'reasoning_effort', category: 'Reasoning', kind: 'enum' },
  { path: 'thinking.type', category: 'Reasoning', kind: 'enum' },
  { path: 'tools', category: 'Tools', kind: 'presence' },
  { path: 'tool_choice', category: 'Tools', kind: 'enum' },
  { path: 'parallel_tool_calls', category: 'Tools', kind: 'presence' },
  { path: 'response_format.type', category: 'Output', kind: 'enum' },
  { path: 'size', category: 'Multimodal', kind: 'enum' },
  { path: 'quality', category: 'Multimodal', kind: 'enum' },
] as const

export type ParameterCapabilityCatalogItem =
  (typeof PARAMETER_CAPABILITY_CATALOG)[number]

export interface ResolvedParameterCapability {
  capability: ParameterCapability
  source: string
}

export interface CapabilityEvaluation {
  parameter: string
  status: 'compatible' | 'rejected' | 'dropped' | 'clamped'
  reason:
    | 'compatible'
    | 'unsupported'
    | 'number_required'
    | 'minimum'
    | 'maximum'
    | 'allowed_values'
  constraint?: number | string
  from?: unknown
  to?: unknown
}

export interface CapabilityEvaluationResult {
  compatible: boolean
  request: Record<string, unknown>
  evaluations: CapabilityEvaluation[]
}

export interface ParameterCapabilityConfigError {
  code:
    | 'selector_required'
    | 'invalid_path'
    | 'inverted_range'
    | 'clamp_without_boundary'
    | 'unsafe_billing_action'
  scope: string
  path?: string
}

const PARAMETER_PATH_PATTERN =
  /^[A-Za-z_][A-Za-z0-9_-]*(?:\.[A-Za-z_][A-Za-z0-9_-]*)*$/
const BILLING_SENSITIVE_PARAMETER_PATHS = new Set([
  'max_tokens',
  'max_completion_tokens',
  'max_output_tokens',
  'maxTokens',
  'maxCompletionTokens',
  'maxOutputTokens',
  'max_tokens_to_sample',
  'maxTokensToSample',
  'generation_config.max_output_tokens',
  'generationConfig.maxOutputTokens',
  'inferenceConfig.maxTokens',
  'n',
  'seconds',
  'duration',
])

export function isBillingSensitiveParameter(path: string): boolean {
  return BILLING_SENSITIVE_PARAMETER_PATHS.has(path)
}

export function createEmptyParameterCapabilityConfig(): ParameterCapabilityConfig {
  return { defaults: {}, rules: [] }
}

export type ParameterCapabilityConfigParseResult =
  | { success: true; config: ParameterCapabilityConfig }
  | { success: false; error: 'invalid_json' | 'invalid_schema' }

export function parseParameterCapabilityConfigStrict(
  value: string
): ParameterCapabilityConfigParseResult {
  const trimmed = value.trim()
  if (!trimmed) {
    return { success: true, config: createEmptyParameterCapabilityConfig() }
  }

  let parsed: unknown
  try {
    parsed = JSON.parse(trimmed)
  } catch {
    return { success: false, error: 'invalid_json' }
  }

  if (!isPlainRecord(parsed) || !hasOnlyKeys(parsed, ['defaults', 'rules'])) {
    return { success: false, error: 'invalid_schema' }
  }

  const defaults = parsed.defaults ?? {}
  const rules = parsed.rules ?? []
  if (
    !isParameterCapabilityMap(defaults) ||
    !Array.isArray(rules) ||
    !rules.every(isModelParameterCapabilityRule)
  ) {
    return { success: false, error: 'invalid_schema' }
  }

  return {
    success: true,
    config: {
      defaults,
      rules: rules.map((rule) => ({
        ...(rule.name === undefined ? {} : { name: rule.name }),
        selector: rule.selector,
        parameters: rule.parameters ?? {},
      })),
    },
  }
}

function isPlainRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === 'object' && value !== null && !Array.isArray(value)
}

function hasOnlyKeys(
  value: Record<string, unknown>,
  allowedKeys: readonly string[]
): boolean {
  const allowed = new Set(allowedKeys)
  return Object.keys(value).every((key) => allowed.has(key))
}

function isParameterCapabilityMap(
  value: unknown
): value is Record<string, ParameterCapability> {
  return (
    isPlainRecord(value) && Object.values(value).every(isParameterCapability)
  )
}

function isParameterCapability(value: unknown): value is ParameterCapability {
  if (
    !isPlainRecord(value) ||
    !hasOnlyKeys(value, [
      'supported',
      'min',
      'max',
      'allowed_values',
      'on_violation',
    ])
  ) {
    return false
  }

  return (
    (value.supported === undefined || typeof value.supported === 'boolean') &&
    (value.min === undefined ||
      (typeof value.min === 'number' && Number.isFinite(value.min))) &&
    (value.max === undefined ||
      (typeof value.max === 'number' && Number.isFinite(value.max))) &&
    (value.allowed_values === undefined ||
      (Array.isArray(value.allowed_values) &&
        value.allowed_values.every((item) => typeof item === 'string'))) &&
    (value.on_violation === undefined ||
      value.on_violation === 'reject' ||
      value.on_violation === 'drop' ||
      value.on_violation === 'clamp')
  )
}

function isModelParameterCapabilityRule(value: unknown): value is {
  name?: string
  selector: { type: 'pattern' | 'exact'; value: string }
  parameters?: Record<string, ParameterCapability>
} {
  if (
    !isPlainRecord(value) ||
    !hasOnlyKeys(value, ['name', 'selector', 'parameters']) ||
    (value.name !== undefined && typeof value.name !== 'string') ||
    !isPlainRecord(value.selector) ||
    !hasOnlyKeys(value.selector, ['type', 'value']) ||
    (value.selector.type !== 'pattern' && value.selector.type !== 'exact') ||
    typeof value.selector.value !== 'string'
  ) {
    return false
  }

  return (
    value.parameters === undefined || isParameterCapabilityMap(value.parameters)
  )
}

export function parseParameterCapabilityConfig(
  value: string
): ParameterCapabilityConfig {
  if (!value.trim()) return createEmptyParameterCapabilityConfig()
  try {
    const parsed = JSON.parse(value) as ParameterCapabilityConfig
    return {
      defaults: parsed.defaults || {},
      rules: Array.isArray(parsed.rules) ? parsed.rules : [],
    }
  } catch {
    return createEmptyParameterCapabilityConfig()
  }
}

export function stringifyParameterCapabilityConfig(
  config: ParameterCapabilityConfig
): string {
  const defaults = config.defaults || {}
  const rules = (config.rules || []).filter(
    (rule) =>
      rule.selector.value.trim() &&
      Object.keys(rule.parameters || {}).length > 0
  )
  if (Object.keys(defaults).length === 0 && rules.length === 0) return ''
  return JSON.stringify({ defaults, rules }, null, 2)
}

export function resolveParameterCapabilities(
  config: ParameterCapabilityConfig,
  model: string
): Record<string, ResolvedParameterCapability> {
  const result: Record<string, ResolvedParameterCapability> = {}
  mergeCapabilities(result, config.defaults || {}, 'Channel default')

  for (const rule of config.rules || []) {
    if (
      rule.selector.type === 'pattern' &&
      modelMatchesPattern(model, rule.selector.value)
    ) {
      mergeCapabilities(
        result,
        rule.parameters || {},
        rule.name || rule.selector.value
      )
    }
  }
  for (const rule of config.rules || []) {
    if (
      rule.selector.type === 'exact' &&
      model === rule.selector.value.trim()
    ) {
      mergeCapabilities(
        result,
        rule.parameters || {},
        rule.name || rule.selector.value
      )
    }
  }
  return result
}

export function validateParameterCapabilityConfig(
  config: ParameterCapabilityConfig
): ParameterCapabilityConfigError[] {
  const errors: ParameterCapabilityConfigError[] = []
  validateCapabilityMap(config.defaults || {}, 'Channel default', errors)
  for (const [index, rule] of (config.rules || []).entries()) {
    const label =
      rule.name?.trim() || rule.selector.value.trim() || `#${index + 1}`
    if (!rule.selector.value.trim()) {
      errors.push({ code: 'selector_required', scope: label })
    }
    validateCapabilityMap(rule.parameters || {}, label, errors)
  }
  return errors
}

export function evaluateParameterCapabilities(
  config: ParameterCapabilityConfig,
  model: string,
  input: Record<string, unknown>
): CapabilityEvaluationResult {
  const request = structuredClone(input)
  const evaluations: CapabilityEvaluation[] = []
  const resolved = resolveParameterCapabilities(config, model)

  for (const parameter of Object.keys(resolved).sort()) {
    const current = getPathValue(request, parameter)
    if (!current.exists) continue
    const capability = resolved[parameter].capability
    const action = capability.on_violation || 'reject'
    let reason: CapabilityEvaluation['reason'] | '' = ''
    let constraint: number | string | undefined
    let clampValue: number | undefined

    if (capability.supported === false) {
      reason = 'unsupported'
    } else if (capability.min !== undefined || capability.max !== undefined) {
      if (typeof current.value !== 'number') {
        reason = 'number_required'
      } else if (
        capability.min !== undefined &&
        current.value < capability.min
      ) {
        reason = 'minimum'
        constraint = capability.min
        clampValue = capability.min
      } else if (
        capability.max !== undefined &&
        current.value > capability.max
      ) {
        reason = 'maximum'
        constraint = capability.max
        clampValue = capability.max
      }
    }

    if (
      !reason &&
      capability.allowed_values?.length &&
      !capability.allowed_values.includes(String(current.value))
    ) {
      reason = 'allowed_values'
      constraint = capability.allowed_values.join(', ')
    }

    if (!reason) {
      evaluations.push({
        parameter,
        status: 'compatible',
        reason: 'compatible',
        from: current.value,
      })
      continue
    }

    if (action === 'drop') {
      deletePathValue(request, parameter)
      evaluations.push({
        parameter,
        status: 'dropped',
        reason,
        constraint,
        from: current.value,
      })
      continue
    }
    if (action === 'clamp' && clampValue !== undefined) {
      setPathValue(request, parameter, clampValue)
      evaluations.push({
        parameter,
        status: 'clamped',
        reason,
        constraint,
        from: current.value,
        to: clampValue,
      })
      continue
    }
    evaluations.push({
      parameter,
      status: 'rejected',
      reason,
      constraint,
      from: current.value,
    })
  }

  return {
    compatible: !evaluations.some((item) => item.status === 'rejected'),
    request,
    evaluations,
  }
}

function mergeCapabilities(
  target: Record<string, ResolvedParameterCapability>,
  source: Record<string, ParameterCapability>,
  sourceLabel: string
): void {
  for (const [path, override] of Object.entries(source)) {
    const current = target[path]?.capability || {}
    target[path] = {
      capability: {
        ...current,
        ...override,
        allowed_values:
          override.allowed_values?.length === 0
            ? current.allowed_values
            : override.allowed_values,
      },
      source: sourceLabel,
    }
  }
}

function modelMatchesPattern(model: string, pattern: string): boolean {
  const escaped = pattern
    .trim()
    .replaceAll(/[.+^${}()|[\]\\]/g, '\\$&')
    .replaceAll('*', '.*')
    .replaceAll('?', '.')
  try {
    return new RegExp(`^${escaped}$`).test(model)
  } catch {
    return false
  }
}

function validateCapabilityMap(
  parameters: Record<string, ParameterCapability>,
  scope: string,
  errors: ParameterCapabilityConfigError[]
): void {
  for (const [path, capability] of Object.entries(parameters)) {
    if (!PARAMETER_PATH_PATTERN.test(path)) {
      errors.push({ code: 'invalid_path', scope, path })
    }
    if (
      capability.min !== undefined &&
      capability.max !== undefined &&
      capability.min > capability.max
    ) {
      errors.push({ code: 'inverted_range', scope, path })
    }
    if (
      capability.on_violation === 'clamp' &&
      capability.min === undefined &&
      capability.max === undefined
    ) {
      errors.push({ code: 'clamp_without_boundary', scope, path })
    }
    if (
      isBillingSensitiveParameter(path) &&
      capability.on_violation !== undefined &&
      capability.on_violation !== 'reject'
    ) {
      errors.push({ code: 'unsafe_billing_action', scope, path })
    }
  }
}

function getPathValue(
  object: Record<string, unknown>,
  path: string
): { exists: boolean; value?: unknown } {
  const parts = path.split('.')
  let current: unknown = object
  for (const part of parts) {
    if (!isRecord(current) || !(part in current)) return { exists: false }
    current = current[part]
  }
  return { exists: true, value: current }
}

function setPathValue(
  object: Record<string, unknown>,
  path: string,
  value: unknown
): void {
  const parts = path.split('.')
  let current = object
  for (let index = 0; index < parts.length - 1; index += 1) {
    const part = parts[index]
    if (!isRecord(current[part])) current[part] = {}
    current = current[part] as Record<string, unknown>
  }
  current[parts.at(-1) as string] = value
}

function deletePathValue(object: Record<string, unknown>, path: string): void {
  const parts = path.split('.')
  let current = object
  for (let index = 0; index < parts.length - 1; index += 1) {
    const next = current[parts[index]]
    if (!isRecord(next)) return
    current = next
  }
  delete current[parts.at(-1) as string]
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === 'object' && value !== null && !Array.isArray(value)
}
