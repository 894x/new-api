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
import type { GetLogsParams, LogExportRequest, LogExportView } from '../types'

export type { LogExportView } from '../types'

export interface LogExportField {
  key: string
  label: string
  upstreamOnly?: boolean
}

const LOG_EXPORT_FIELDS: LogExportField[] = [
  { key: 'created_at', label: 'Time' },
  { key: 'request_id', label: 'Request ID' },
  {
    key: 'upstream_request_id',
    label: 'Upstream Request ID',
    upstreamOnly: true,
  },
  { key: 'status', label: 'Status' },
  { key: 'user_id', label: 'User ID' },
  { key: 'username', label: 'Username' },
  { key: 'token_id', label: 'Token ID' },
  { key: 'token_name', label: 'Token Name' },
  { key: 'group', label: 'Group' },
  {
    key: 'channel_id',
    label: 'Channel ID',
    upstreamOnly: true,
  },
  {
    key: 'channel_name',
    label: 'Channel Name',
    upstreamOnly: true,
  },
  { key: 'model_name', label: 'Model Name' },
  {
    key: 'upstream_model_name',
    label: 'Upstream Model Name',
    upstreamOnly: true,
  },
  { key: 'input_tokens', label: 'Input Tokens' },
  {
    key: 'cached_input_tokens',
    label: 'Cached Input Tokens',
  },
  { key: 'output_tokens', label: 'Output Tokens' },
  { key: 'model_price', label: 'Model Price' },
  { key: 'model_ratio', label: 'Model Ratio' },
  {
    key: 'completion_ratio',
    label: 'Completion Ratio',
  },
  { key: 'cache_ratio', label: 'Cache Ratio' },
  { key: 'group_ratio', label: 'Group Ratio' },
  { key: 'quota_per_unit', label: 'Quota per USD' },
  {
    key: 'original_amount_usd',
    label: 'Original Amount (USD)',
  },
  {
    key: 'error_message',
    label: 'Error Message',
    upstreamOnly: true,
  },
]

const DEFAULT_LOG_EXPORT_FIELDS: Record<LogExportView, string[]> = {
  upstream: [
    'created_at',
    'upstream_request_id',
    'status',
    'channel_name',
    'model_name',
    'upstream_model_name',
    'input_tokens',
    'cached_input_tokens',
    'output_tokens',
    'model_price',
    'completion_ratio',
    'cache_ratio',
    'error_message',
  ],
  downstream: [
    'created_at',
    'request_id',
    'status',
    'user_id',
    'username',
    'token_name',
    'group',
    'model_name',
    'input_tokens',
    'cached_input_tokens',
    'output_tokens',
    'model_price',
    'completion_ratio',
    'cache_ratio',
    'group_ratio',
  ],
}

export function getLogExportFields(view: LogExportView): LogExportField[] {
  if (view === 'upstream') return LOG_EXPORT_FIELDS
  return LOG_EXPORT_FIELDS.filter((field) => !field.upstreamOnly)
}

export function getDefaultLogExportFields(view: LogExportView): string[] {
  return [...DEFAULT_LOG_EXPORT_FIELDS[view]]
}

export function buildLogExportRequest(
  view: LogExportView,
  fields: string[],
  params: GetLogsParams
): LogExportRequest {
  const { p: _page, page_size: _pageSize, type: _type, ...filters } = params
  if (view === 'downstream') {
    const {
      channel: _channel,
      upstream_request_id: _upstreamRequestId,
      ...downstreamFilters
    } = filters
    return { view, fields, ...downstreamFilters }
  }
  return { view, fields, ...filters }
}
