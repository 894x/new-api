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
  defaultSelected: boolean
}

const LOG_EXPORT_FIELDS: LogExportField[] = [
  { key: 'created_at', label: 'Time', defaultSelected: true },
  { key: 'request_id', label: 'Request ID', defaultSelected: true },
  {
    key: 'upstream_request_id',
    label: 'Upstream Request ID',
    upstreamOnly: true,
    defaultSelected: true,
  },
  { key: 'status', label: 'Status', defaultSelected: true },
  { key: 'user_id', label: 'User ID', defaultSelected: true },
  { key: 'username', label: 'Username', defaultSelected: true },
  { key: 'token_id', label: 'Token ID', defaultSelected: false },
  { key: 'token_name', label: 'Token Name', defaultSelected: true },
  { key: 'group', label: 'Group', defaultSelected: true },
  {
    key: 'channel_id',
    label: 'Channel ID',
    upstreamOnly: true,
    defaultSelected: true,
  },
  {
    key: 'channel_name',
    label: 'Channel Name',
    upstreamOnly: true,
    defaultSelected: true,
  },
  { key: 'model_name', label: 'Model Name', defaultSelected: true },
  {
    key: 'upstream_model_name',
    label: 'Upstream Model Name',
    upstreamOnly: true,
    defaultSelected: true,
  },
  { key: 'input_tokens', label: 'Input Tokens', defaultSelected: true },
  {
    key: 'cached_input_tokens',
    label: 'Cached Input Tokens',
    defaultSelected: true,
  },
  { key: 'output_tokens', label: 'Output Tokens', defaultSelected: true },
  { key: 'model_price', label: 'Model Price', defaultSelected: true },
  { key: 'model_ratio', label: 'Model Ratio', defaultSelected: true },
  {
    key: 'completion_ratio',
    label: 'Completion Ratio',
    defaultSelected: true,
  },
  { key: 'cache_ratio', label: 'Cache Ratio', defaultSelected: true },
  { key: 'group_ratio', label: 'Group Ratio', defaultSelected: true },
  { key: 'quota_per_unit', label: 'Quota per USD', defaultSelected: true },
  {
    key: 'original_amount_usd',
    label: 'Original Amount (USD)',
    defaultSelected: true,
  },
  {
    key: 'error_message',
    label: 'Error Message',
    upstreamOnly: true,
    defaultSelected: true,
  },
]

export function getLogExportFields(view: LogExportView): LogExportField[] {
  if (view === 'upstream') return LOG_EXPORT_FIELDS
  return LOG_EXPORT_FIELDS.filter((field) => !field.upstreamOnly)
}

export function getDefaultLogExportFields(view: LogExportView): string[] {
  return getLogExportFields(view)
    .filter((field) => field.defaultSelected)
    .map((field) => field.key)
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
