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
import { z } from 'zod'

// ============================================================================
// Model Types
// ============================================================================

export interface ModelChannelCapabilityGroup {
  group: string
  enabled: boolean
}

export interface ModelParameterCapability {
  transform?:
    | 'none'
    | 'image_url_to_base64'
    | 'audio_url_to_base64'
    | 'video_url_to_base64'
  supported?: boolean
  min?: number
  max?: number
  allowed_values?: string[]
  on_violation?: 'reject' | 'drop' | 'clamp'
}

export interface ModelChannelParameterOverrideCondition {
  order: number
  path: string
  mode: string
  value: unknown
  invert: boolean
  pass_missing_key: boolean
}

export interface ModelChannelParameterOverrideOperation {
  order: number
  description?: string
  path?: string
  mode: string
  value?: unknown
  value_configured?: boolean
  keep_origin: boolean
  from?: string
  to?: string
  conditions: ModelChannelParameterOverrideCondition[]
  logic: string
}

export interface ModelChannelCapability {
  channel_id: number
  channel_name: string
  channel_type: number
  channel_status: number
  model: string
  default_priority: number
  default_weight: number
  priority_override: number | null
  weight_override: number | null
  effective_priority: number
  effective_weight: number
  groups: ModelChannelCapabilityGroup[]
  upstream_model: string
  model_mapped: boolean
  endpoint_types: string[]
  parameter_capabilities_configured: boolean
  parameter_capabilities?: Record<string, ModelParameterCapability>
  parameter_override_configured: boolean
  parameter_override_mode: 'none' | 'legacy' | 'operations' | 'mixed'
  parameter_override_legacy?: Record<string, unknown>
  parameter_override_operations: ModelChannelParameterOverrideOperation[]
  configuration_error?: string
}

export interface ModelChannelCapabilities {
  model: string
  channels: ModelChannelCapability[]
}

export interface ModelChannelCapabilitiesResponse {
  success: boolean
  message?: string
  data?: ModelChannelCapabilities
}

/**
 * Bound channel information
 */
export interface BoundChannel {
  name: string
  type: number
}

/**
 * Model entity from API
 */
export type ModelSquareState = 'visible' | 'unavailable' | 'hidden' | 'partial'

export interface Model {
  square_state?: ModelSquareState
  has_metadata?: boolean
  configured_channel_count?: number
  id: number
  model_name: string
  description?: string
  icon?: string
  tags?: string
  vendor_id?: number
  endpoints?: string
  supported_endpoints?: string[]
  status: number
  doc_enabled: number
  doc_available?: boolean
  sync_official: number
  created_time: number
  updated_time: number
  name_rule: number
  // Runtime fields
  bound_channels?: BoundChannel[]
  enable_groups?: string[]
  quota_types?: number[]
  matched_models?: string[]
  matched_count?: number
}

/**
 * Vendor entity from API
 */
export interface Vendor {
  model_count?: number
  version?: string
  id: number
  name: string
  description?: string
  icon?: string
  status: number
  created_time: number
  updated_time: number
}

/**
 * Prefill group entity
 */
export interface PrefillGroup {
  id: number
  name: string
  type: 'model' | 'tag' | 'endpoint'
  items: string | string[]
  description?: string
}

// ============================================================================
// API Request/Response Types
// ============================================================================

/**
 * Get models list parameters
 */
export interface GetModelsParams {
  square_state?: ModelSquareState
  include_channel_models?: boolean
  p?: number
  page_size?: number
  vendor?: string // vendor ID to filter by
  status?: string // filter by status
  sync_official?: string // filter by sync_official status
}

/**
 * Search models parameters
 */
export interface SearchModelsParams {
  square_state?: ModelSquareState
  include_channel_models?: boolean
  keyword?: string
  vendor?: string // vendor ID to filter by
  status?: string // filter by status
  sync_official?: string // filter by sync_official status
  p?: number
  page_size?: number
}

/**
 * Get models response
 */
export interface GetModelsResponse {
  success: boolean
  message?: string
  data?: {
    items: Model[]
    total: number
    page: number
    page_size: number
    vendor_counts?: Record<string, number>
  }
}

/**
 * Get model detail response
 */
export interface GetModelResponse {
  success: boolean
  message?: string
  data?: Model
}

export interface ModelDocumentEditor {
  model_id: number
  model: string
  interface_key: string
  interface_name: string
  source: 'builtin' | 'custom' | 'empty'
  effective_source: 'builtin' | 'custom' | 'none'
  has_builtin: boolean
  has_custom: boolean
  published: boolean
  slug: string
  title: string
  vendor: string
  category: string
  summary: string
  html: string
  updated_time: number
}

export interface ModelDocumentEditorPayload {
  interface_name: string
  slug: string
  title: string
  vendor: string
  category: string
  summary: string
  html: string
}

export interface ModelDocumentEditorResponse {
  success: boolean
  message?: string
  data?: ModelDocumentEditorCollection
}

export interface ModelDocumentEditorCollection {
  model_id: number
  model: string
  variants: ModelDocumentEditor[]
}

/**
 * Get vendors response
 */
export interface GetVendorsResponse {
  success: boolean
  message?: string
  data?: {
    items: Vendor[]
    total: number
    page: number
    page_size: number
  }
}

/**
 * Get vendor response
 */
export interface GetVendorResponse {
  success: boolean
  message?: string
  data?: Vendor
}

/**
 * Sync diff data
 */
export type MetadataSyncField =
  | 'description'
  | 'icon'
  | 'tags'
  | 'vendor'
  | 'endpoints'
  | 'name_rule'
  | 'status'
export type MetadataSyncValues = {
  description: string
  icon: string
  tags: string
  vendor: string
  endpoints: string
  name_rule: number
  status: number
}
export type MetadataSyncCandidate = {
  model_name: string
  kind:
    | 'create'
    | 'update'
    | 'unchanged'
    | 'blocked'
    | 'missing_upstream'
    | 'missing_vendor'
  scope: 'site' | 'catalog'
  record_version: string
  fields: Array<{
    field: MetadataSyncField
    local: string | number
    upstream: string | number
  }>
  upstream?: MetadataSyncValues
  vendor_to_create?: string
}
export type MetadataSyncSource = {
  locale: SyncLocale
  models_url: string
  vendors_url: string
  version: string
}
export type MetadataSyncPreview = {
  source: MetadataSyncSource
  candidates: MetadataSyncCandidate[]
}
export type MetadataSyncSelection = {
  model_name: string
  record_version: string
  create: boolean
  fields: MetadataSyncField[]
}
export type MetadataSyncRequest = {
  locale: SyncLocale
  source_version: string
  selections: MetadataSyncSelection[]
}
export type MetadataSyncResult = {
  created_models: string[]
  updated_models: MetadataSyncSelection[]
  created_vendors: string[]
}
export interface SyncUpstreamResponse {
  success: boolean
  message?: string
  data?: MetadataSyncResult
}
export interface PreviewUpstreamDiffResponse {
  success: boolean
  message?: string
  data?: MetadataSyncPreview
}

/**
 * Missing models response
 */
export interface MissingModelsResponse {
  success: boolean
  message?: string
  data?: string[]
}

/**
 * Prefill groups response
 */
export interface PrefillGroupsResponse {
  success: boolean
  message?: string
  data?: PrefillGroup[]
}

// ============================================================================
// Form Data Types
// ============================================================================

/**
 * Model form schema
 */
export const modelFormSchema = z.object({
  id: z.number().optional(),
  model_name: z.string().min(1, 'Model name is required'),
  description: z.string().default(''),
  icon: z.string().default(''),
  tags: z.array(z.string()).default([]),
  vendor_id: z.number().optional(),
  endpoints: z.string().default(''),
  name_rule: z.number().min(0).max(3).default(0),
  status: z.boolean().default(true),
  doc_enabled: z.boolean().default(false),
  sync_official: z.boolean().default(true),
})

export type ModelFormValues = z.infer<typeof modelFormSchema>

/**
 * Vendor form schema
 */
export const vendorFormSchema = z.object({
  id: z.number().optional(),
  name: z
    .string()
    .trim()
    .min(1, 'Vendor name is required')
    .max(128, 'Vendor name and icon must not exceed 128 characters.'),
  description: z.string().default(''),
  icon: z
    .string()
    .max(128, 'Vendor name and icon must not exceed 128 characters.')
    .default(''),
  version: z.string().optional(),
})

export type VendorFormValues = z.infer<typeof vendorFormSchema>

/**
 * Prefill group form schema
 */
export const prefillGroupFormSchema = z.object({
  id: z.number().optional(),
  name: z.string().min(1, 'Group name is required'),
  description: z.string().optional(),
  type: z.enum(['model', 'tag', 'endpoint']),
  items: z.union([z.string(), z.array(z.string())]),
})

export type PrefillGroupFormValues = z.infer<typeof prefillGroupFormSchema>

// ============================================================================
// Utility Types
// ============================================================================

/**
 * Name rule type
 */
export type NameRule = 0 | 1 | 2 | 3 // exact, prefix, contains, suffix

/**
 * Model status type
 */
export type ModelStatus = 0 | 1 // disabled, enabled

/**
 * Quota type
 */
export type QuotaType = 0 | 1 // usage-based, per-call

/**
 * Sync locale
 */
export type SyncLocale = 'zh' | 'zh-CN' | 'en' | 'ja'

/**
 * Sync upstream source
 */
export type SyncSource = 'official'

// ============================================================================
// Model Deployments Types
// ============================================================================

/**
 * Model tab type
 */
export type ModelTabCategory = 'metadata' | 'vendors' | 'deployments'

/**
 * Deployment entity from API
 */
export interface Deployment {
  id: string | number
  container_name?: string
  deployment_name?: string
  name?: string
  status?: string
  provider?: string
  /**
   * Human readable string returned by backend, e.g. "2 hour 15 minutes"
   * or "completed".
   */
  time_remaining?: string
  /**
   * Remaining minutes (numeric) returned by backend.
   */
  compute_minutes_remaining?: number
  /**
   * Served minutes (numeric) returned by backend.
   */
  compute_minutes_served?: number
  /**
   * Completed percent (0-100) returned by backend.
   */
  completed_percent?: number
  hardware_info?: string | Record<string, unknown>
  hardware_name?: string
  brand_name?: string
  hardware_quantity?: number
  created_at?: string | number
  updated_at?: string | number
  [key: string]: unknown
}

/**
 * Deployment settings response
 */
export interface DeploymentSettingsResponse {
  success: boolean
  message?: string
  data?: {
    enabled?: boolean
    [key: string]: unknown
  }
}

/**
 * List deployments response
 */
export interface ListDeploymentsResponse {
  success: boolean
  message?: string
  data?: {
    items?: Deployment[]
    total?: number
    page?: number
    page_size?: number
    status_counts?: Record<string, number>
  }
}

/**
 * Deployment logs response
 */
export interface DeploymentLogsResponse {
  success: boolean
  message?: string
  data?: {
    logs?: Array<{
      timestamp?: string
      level?: string
      message?: string
      source?: string
    }>
    cursor?: string
  }
}
