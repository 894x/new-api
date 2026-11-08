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
import { api } from '@/lib/api'
import { requireServerSuccess } from '@/lib/server-error-message'

export interface EarlyChunkBand {
  index: number
  min: number
  max?: number
  unknown?: boolean
}

export interface EarlyChunkMetrics {
  samples: number
  content_samples: number
  completed: number
  failed: number
  cancelled: number
  average_ms: number | null
  p50_ms: number | null
  p90_ms: number | null
  p95_ms: number | null
  overflow: number
  decisions: number
  sent: number
  met: number
  sent_met: number
  sent_failed: number
  average_gap_ms: number | null
  updated_at_ms: number
}

export interface EarlyChunkCell {
  id: number
  token_band: number
  cache_band: number
  metrics: EarlyChunkMetrics
  prediction: {
    pending: number
    soon: number
    probability: number
    merged_cells: number
    eligible: boolean
    at_ms: number
    estimated_ttft_ms: number | null
    remaining_p50_ms: number | null
    remaining_p90_ms: number | null
    lead_ms: number | null
    candidate_ms: number | null
    reason: string
  }
  histogram: number[]
}

export interface EarlyChunkGrid {
  model: string
  channel_id: number
  channel_name: string
  scope: 'model_channel'
  backend: 'redis' | 'memory'
  token_bands: EarlyChunkBand[]
  cache_bands: EarlyChunkBand[]
  latency_bounds_ms: number[]
  regular_buckets: number
  tracked_buckets: number
  max_buckets: number
  payload_bytes: number
  node_read_errors: number
  node_write_errors: number
  node_evictions: number
  probe_ms: number
  config: {
    mode: string
    lead_ratio: number
    min_lead_ms: number
    max_lead_ms: number
    earliest_ms: number
    min_samples: number
    min_probability: number
    max_age_seconds: number
  }
  summary: EarlyChunkMetrics
  cells: EarlyChunkCell[]
  generated_at_ms: number
}

export async function getEarlyChunkGrid(
  model: string,
  channelId: number,
  probeMS: number,
  signal?: AbortSignal
): Promise<EarlyChunkGrid> {
  const response = await api.get<{
    success: boolean
    message?: string
    data: EarlyChunkGrid
  }>('/api/channel/early-chunk-grid', {
    params: { model, channel_id: channelId, probe_ms: probeMS },
    signal,
  })
  return requireServerSuccess(response.data).data
}
