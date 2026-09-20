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
  p50_ms: number
  p90_ms: number
  p99_ms: number
  sample_count: number
}

export type PerformanceAnalyticsSummary = {
  request_count: number
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
  model_name: string
  effective_start_timestamp: number
  effective_end_timestamp: number
  summary: PerformanceAnalyticsSummary
  series: PerformanceAnalyticsPoint[]
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
  percentile: 'P50' | 'P90' | 'P99'
  value: number
}

export type PerformanceMetric = 'rpm' | 'tpm' | 'cache_hit_rate'

export type PerformanceMetricChartDatum = {
  ts: number
  time: number
  label: string
  value: number
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
  success_rate: number
  active_concurrency: ChannelAnalyticsConcurrency
  latency: ChannelAnalyticsLatency
}

export type ChannelAnalyticsData = {
  scanned_logs: number
  truncated: boolean
  transport_groups: ChannelTransportGroup[]
  channel_id: number
  effective_start_timestamp: number
  effective_end_timestamp: number
  summary: ChannelAnalyticsPoint
  series: ChannelAnalyticsPoint[]
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
