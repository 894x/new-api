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

import type { AdminPermissionMatrix } from '@/lib/admin-permissions'

// ============================================================================
// User Schema & Types
// ============================================================================

/** User status: 1 = enabled, 2 = disabled, 3+ = other states */
export const userStatusSchema = z.number()
export type UserStatus = z.infer<typeof userStatusSchema>

/** User role: 1 = common user, 10 = admin, 100 = root */
export const userRoleSchema = z.number()
export type UserRole = z.infer<typeof userRoleSchema>

export const logQueryRateLimitPolicySchema = z.object({
  enabled: z.boolean(),
  default_limit: z.number(),
  window_seconds: z.number(),
  max_limit: z.number(),
})

export const userSchema = z.object({
  id: z.number(),
  username: z.string(),
  display_name: z.string(),
  password: z.string().optional(),
  github_id: z.string().optional(),
  oidc_id: z.string().optional(),
  wechat_id: z.string().optional(),
  telegram_id: z.string().optional(),
  email: z.string().optional(),
  quota: z.number(),
  used_quota: z.number(),
  request_count: z.number(),
  group: z.string(),
  aff_code: z.string().optional(),
  aff_count: z.number().optional(),
  aff_quota: z.number().optional(),
  aff_history_quota: z.number().optional(),
  inviter_id: z.number().optional(),
  managed_by_user_id: z.number().nullable().optional(),
  linux_do_id: z.string().optional(),
  status: userStatusSchema,
  role: userRoleSchema,
  created_at: z.number().optional(),
  updated_at: z.number().optional(),
  last_login_at: z.number().optional(),
  DeletedAt: z.any().nullable().optional(),
  remark: z.string().optional(),
  log_query_rate_limit: z.number().nullable().optional(),
  log_query_rate_limit_policy: logQueryRateLimitPolicySchema.optional(),
  admin_permissions: z
    .record(z.string(), z.record(z.string(), z.boolean()))
    .optional(),
})
export type User = z.infer<typeof userSchema>

export const userListSchema = z.array(userSchema)

// ============================================================================
// API Request/Response Types
// ============================================================================

/** Generic API response */
export interface ApiResponse<T = unknown> {
  success: boolean
  message?: string
  data?: T
}

export type UserSortBy =
  | 'id'
  | 'username'
  | 'quota'
  | 'group'
  | 'created_at'
  | 'last_login_at'

export type UserSortOrder = 'asc' | 'desc'

export interface GetUsersParams {
  p?: number
  page_size?: number
  sort_by?: UserSortBy
  sort_order?: UserSortOrder
}

export interface GetUsersResponse {
  success: boolean
  message?: string
  data?: {
    items: User[]
    total: number
    page: number
    page_size: number
  }
}

export interface SearchUsersParams {
  keyword?: string
  group?: string
  role?: string
  status?: string
  p?: number
  page_size?: number
  sort_by?: UserSortBy
  sort_order?: UserSortOrder
}

export interface UserFormData {
  username: string
  display_name: string
  password?: string
  role?: number // Only used when creating user
  quota?: number // Only used when updating user
  group?: string // Only used when updating user
  remark?: string // Only used when updating user
  log_query_rate_limit?: number // 0 inherits the global limit; admin edits only
  admin_permissions?: AdminPermissionMatrix
}

export type ManageUserAction =
  | 'promote'
  | 'demote'
  | 'enable'
  | 'disable'
  | 'delete'
  | 'add_quota'

export type QuotaAdjustMode = 'add' | 'subtract' | 'override'

export interface ManageUserQuotaPayload {
  id: number
  action: 'add_quota'
  mode: QuotaAdjustMode
  value: number
}

export interface UserGroupRateLimit {
  limits: [number, number, number]
  models: Record<
    string,
    {
      rpm?: number
      tpm?: number
    }
  >
}

export interface UserChannelRoutingRow {
  channel_id: number
  channel_name: string
  default_priority: number
  model_priority: number | null
  inherited_priority: number
  priority_override: number | null
  effective_priority: number
  enabled: boolean
  available: boolean
}

export interface UserChannelRoutingConfig {
  models: string[]
  model: string
  revision: string
  channels: UserChannelRoutingRow[]
}

export interface UserChannelRoutingPatch {
  model: string
  revision: string
  overrides: Pick<
    UserChannelRoutingRow,
    'channel_id' | 'priority_override' | 'enabled'
  >[]
}

export interface UserGroupConfig {
  user_id: number
  username: string
  group: string
  dedicated_group_name: string
  is_dedicated_group: boolean
  revision: string
  group_user_count: number
  discounts: Record<string, number>
  rate_limit_enabled: boolean
  global_rate_limit_enabled: boolean
  rate_limit: UserGroupRateLimit
  model_channel_groups: Record<string, string[]>
  available_group_ratios: Record<string, number>
}

export interface UpdateUserGroupConfigPayload {
  group: string
  revision: string
  discounts: Record<string, number>
  rate_limit_enabled: boolean
  rate_limit: UserGroupRateLimit
  model_channel_groups: Record<string, string[]>
}

export interface ChannelPoolPreview {
  candidate_count: number
}

// ============================================================================
// Dialog Types
// ============================================================================

export type UsersDialogType = 'create' | 'update' | 'delete' | 'group-config'
export interface UserCacheHitPolicy {
  enabled: boolean
  min_bps: number
  max_bps: number
}

export interface UserCacheHitPolicyConfig {
  policies: Record<string, UserCacheHitPolicy>
  revision: string
  daily: {
    day: string
    input_tokens: number
    real_cache_tokens: number
    bill_cache_tokens: number
  }
  statistics_error: string
}
