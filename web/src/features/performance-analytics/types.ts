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
