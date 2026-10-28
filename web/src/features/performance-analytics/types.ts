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
export type PerformancePercentiles = {
  avg_ms?: number | null
  p50_ms: number
  p90_ms: number
  p95_ms?: number
  max_ms?: number | null
  p99_ms: number
  sample_count: number
}

export type PerformanceValuePercentiles = {
  p50: number
  p90: number
  p95: number
  p99: number
  max: number | null
  sample_count: number
}

export type PerformanceAnalyticsSummary = {
  metric_percentiles?: Partial<
    Record<PerformanceMetric, PerformanceValuePercentiles>
  >
  request_body_samples?: number
  request_body_bytes?: number
  request_body_max_bytes?: number
  request_body_avg_bytes?: number | null
  cache_read_samples?: number
  cache_read_tokens?: number
  cache_write_samples?: number
  cache_write_tokens?: number
  cache_write_5m_tokens?: number
  cache_write_1h_tokens?: number
  request_count: number
  error_count: number
  success_rate: number
  rpm: number
  tpm: number
  cache_hit_rate: number
  ttft: PerformancePercentiles
  tpot: PerformancePercentiles
}

export type PerformanceAnalyticsPoint = PerformanceAnalyticsSummary & {
  ts: number
}

export type PerformanceAnalyticsData = {
  source?: 'usage_logs'
  errors_unavailable?: boolean
  model_name: string
  scanned_logs: number
  truncated: boolean
  error_groups: PerformanceAnalyticsErrorGroup[]
  effective_start_timestamp: number
  effective_end_timestamp: number
  summary: PerformanceAnalyticsSummary
  series: PerformanceAnalyticsPoint[]
}

export type PerformanceAnalyticsErrorGroup = {
  error_code: string
  error_type: string
  status_code: number
  count: number
}

export type PerformanceAnalyticsUserOption = {
  id: number
  username: string
}

export type PerformanceAnalyticsTokenOption = {
  id: number
  user_id: number
  name: string
}

export type PerformanceAnalyticsOptions = {
  channels?: Array<{ id: number; name: string }>
  models: string[]
  users: PerformanceAnalyticsUserOption[]
  tokens: PerformanceAnalyticsTokenOption[]
}

export type PerformanceAnalyticsResponse<T> = {
  success: boolean
  message?: string
  data: T
}

export type PerformanceChartDatum = {
  ts: number
  time: number
  label: string
  percentile: 'AVG' | 'P50' | 'P90' | 'P95' | 'P99' | 'MAX'
  value: number
}

export type PerformanceMetric =
  | 'rpm'
  | 'tpm'
  | 'cache_hit_rate'
  | 'error_count'
  | 'request_body_avg_bytes'
  | 'cache_write_tokens'

export type PerformanceMetricChartDatum = Omit<
  PerformanceChartDatum,
  'percentile'
> & {
  percentile?: PerformanceChartDatum['percentile']
}

export type ChannelAnalyticsPercentiles = {
  p50_ms: number
  p95_ms: number
  p99_ms: number
  sample_count: number
}

export type ChannelAnalyticsLatency = {
  acquire_ms: ChannelAnalyticsPercentiles
  write_ms: ChannelAnalyticsPercentiles
  total_ms: ChannelAnalyticsPercentiles
  body_read_ms: ChannelAnalyticsPercentiles
  upstream_queue_ms: ChannelAnalyticsPercentiles
  upload_ms: ChannelAnalyticsPercentiles
  provider_wait_ms: ChannelAnalyticsPercentiles
  headers_to_first_response_ms: ChannelAnalyticsPercentiles
  downstream_ms: ChannelAnalyticsPercentiles
}

export type ChannelAnalyticsConcurrency = {
  average: number
  maximum: number
}

export type ChannelAnalyticsPoint = {
  ts: number
  request_count: number
  error_count: number
  success_rate: number
  active_concurrency: ChannelAnalyticsConcurrency
  latency: ChannelAnalyticsLatency
}

export type ChannelAnalyticsData = {
  scanned_logs: number
  truncated: boolean
  error_groups: ChannelErrorGroup[]
  transport_groups: ChannelTransportGroup[]
  channel_id: number
  effective_start_timestamp: number
  effective_end_timestamp: number
  summary: ChannelAnalyticsPoint
  series: ChannelAnalyticsPoint[]
}

export type ChannelErrorGroup = {
  error_code: string
  error_type: string
  status_code: number
  count: number
}

export type ChannelTransportGroup = {
  channel_id: number
  protocol: string
  body_class: string
  reason: string
  threshold_bytes: number
  count: number
  errors: number
  canceled: number
  timeouts: number
  reused: number
  acquire_ms: ChannelAnalyticsPercentiles
  write_ms: ChannelAnalyticsPercentiles
}

export type HTTPTransportMetric = {
  pool_id: number
  protocol: string
  shard: number
  shards: number
  active_requests: number
  connections: number
  last_protocol?: string
  active_by_channel?: Record<string, number>
}
