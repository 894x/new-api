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
import type {
  ModelRoutingOverride,
  ModelRoutingOverridePatch,
  ModelRoutingOverridesResponse,
} from '../types'

export const MAX_MODEL_ROUTING_WEIGHT = 2_147_483_637

export type ModelRoutingOverrideDraft = Pick<
  Record<keyof ModelRoutingOverridePatch, string>,
  'priority_override' | 'weight_override'
>

export type ModelRoutingOverrideDraftField = keyof ModelRoutingOverrideDraft

export type ModelRoutingOverrideDraftState = {
  drafts: Record<string, ModelRoutingOverrideDraft>
  dirtyFields: ReadonlySet<string>
  serverDrafts: Record<string, ModelRoutingOverrideDraft>
}

export type ModelRoutingOverrideDraftError = {
  key: string
  field: keyof ModelRoutingOverrideDraft
}

export type ModelRoutingOverrideSerialization = {
  overrides: ModelRoutingOverridePatch[]
  errors: ModelRoutingOverrideDraftError[]
}

export function getModelRoutingOverrideKey(
  channelId: number,
  model: string
): string {
  return `${channelId}\u0000${model}`
}

export function getModelRoutingOverrideDirtyFieldKey(
  rowKey: string,
  field: ModelRoutingOverrideDraftField
): string {
  return `${rowKey}:${field}`
}

export function parseRoutingOverrideInput(
  rawValue: string
): number | null | undefined {
  const trimmedValue = rawValue.trim()
  if (trimmedValue === '') return null

  const parsedValue = Number(trimmedValue)
  if (!Number.isSafeInteger(parsedValue)) return undefined
  return parsedValue
}

export function createModelRoutingOverrideDrafts(
  rows: ModelRoutingOverride[]
): Record<string, ModelRoutingOverrideDraft> {
  return Object.fromEntries(
    rows.map((row) => [
      getModelRoutingOverrideKey(row.channel_id, row.model),
      {
        priority_override:
          row.priority_override === null ? '' : String(row.priority_override),
        weight_override:
          row.weight_override === null ? '' : String(row.weight_override),
      },
    ])
  )
}

export function createModelRoutingOverrideDraftState(
  rows: ModelRoutingOverride[]
): ModelRoutingOverrideDraftState {
  const serverDrafts = createModelRoutingOverrideDrafts(rows)
  return {
    drafts: serverDrafts,
    dirtyFields: new Set(),
    serverDrafts,
  }
}

export function mergeModelRoutingOverrideDraftState(
  rows: ModelRoutingOverride[],
  currentState: ModelRoutingOverrideDraftState
): ModelRoutingOverrideDraftState {
  const serverDrafts = createModelRoutingOverrideDrafts(rows)
  const mergedDrafts: Record<string, ModelRoutingOverrideDraft> = {}
  const mergedDirtyFields = new Set<string>()

  for (const [key, serverDraft] of Object.entries(serverDrafts)) {
    const currentDraft = currentState.drafts[key]
    const mergedDraft = { ...serverDraft }

    for (const field of ['priority_override', 'weight_override'] as const) {
      const dirtyFieldKey = getModelRoutingOverrideDirtyFieldKey(key, field)
      if (
        !currentDraft ||
        !currentState.dirtyFields.has(dirtyFieldKey) ||
        currentDraft[field] === serverDraft[field]
      ) {
        continue
      }
      mergedDraft[field] = currentDraft[field]
      mergedDirtyFields.add(dirtyFieldKey)
    }

    mergedDrafts[key] = mergedDraft
  }

  return {
    drafts: mergedDrafts,
    dirtyFields: mergedDirtyFields,
    serverDrafts,
  }
}

export function updateModelRoutingOverrideDraftField(
  currentState: ModelRoutingOverrideDraftState,
  row: ModelRoutingOverride,
  field: ModelRoutingOverrideDraftField,
  value: string
): ModelRoutingOverrideDraftState {
  const key = getModelRoutingOverrideKey(row.channel_id, row.model)
  const serverDraft = currentState.serverDrafts[key]
  if (!serverDraft) return currentState

  const nextDrafts = {
    ...currentState.drafts,
    [key]: {
      ...(currentState.drafts[key] ?? serverDraft),
      [field]: value,
    },
  }
  const nextDirtyFields = new Set(currentState.dirtyFields)
  const dirtyFieldKey = getModelRoutingOverrideDirtyFieldKey(key, field)
  if (value === serverDraft[field]) {
    nextDirtyFields.delete(dirtyFieldKey)
  } else {
    nextDirtyFields.add(dirtyFieldKey)
  }

  return {
    drafts: nextDrafts,
    dirtyFields: nextDirtyFields,
    serverDrafts: currentState.serverDrafts,
  }
}

export function resetModelRoutingOverrideDraft(
  currentState: ModelRoutingOverrideDraftState,
  row: ModelRoutingOverride
): ModelRoutingOverrideDraftState {
  const priorityResetState = updateModelRoutingOverrideDraftField(
    currentState,
    row,
    'priority_override',
    ''
  )
  return updateModelRoutingOverrideDraftField(
    priorityResetState,
    row,
    'weight_override',
    ''
  )
}

export function ensureSuccessfulModelRoutingOverridesResponse(
  response: ModelRoutingOverridesResponse
): ModelRoutingOverridesResponse {
  if (!response.success) {
    throw new Error(
      response.message || 'Failed to load model routing overrides.'
    )
  }
  return response
}

export function collectChangedModelRoutingOverrides(
  rows: ModelRoutingOverride[],
  drafts: Record<string, ModelRoutingOverrideDraft>
): ModelRoutingOverrideSerialization {
  const changedOverrides: ModelRoutingOverridePatch[] = []
  const errors: ModelRoutingOverrideDraftError[] = []

  for (const row of rows) {
    const key = getModelRoutingOverrideKey(row.channel_id, row.model)
    const draft = drafts[key]
    if (!draft) continue
    const priorityOverride = parseRoutingOverrideInput(draft.priority_override)
    const weightOverride = parseRoutingOverrideInput(draft.weight_override)
    if (priorityOverride === undefined) {
      errors.push({ key, field: 'priority_override' })
    }
    if (
      weightOverride === undefined ||
      (weightOverride ?? 0) < 0 ||
      (weightOverride ?? 0) > MAX_MODEL_ROUTING_WEIGHT
    ) {
      errors.push({ key, field: 'weight_override' })
    }
    if (
      priorityOverride === undefined ||
      weightOverride === undefined ||
      (weightOverride ?? 0) < 0 ||
      (weightOverride ?? 0) > MAX_MODEL_ROUTING_WEIGHT
    ) {
      continue
    }
    if (
      priorityOverride === row.priority_override &&
      weightOverride === row.weight_override
    ) {
      continue
    }

    changedOverrides.push({
      channel_id: row.channel_id,
      model: row.model,
      priority_override: priorityOverride,
      weight_override: weightOverride,
    })
  }

  return { overrides: changedOverrides, errors }
}
