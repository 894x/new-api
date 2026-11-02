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

export type MonitorObservation = {
  id: number
  channel_id: number
  model: string
  endpoint_type: string
  created_at: number
  state: string
  latency_ms: number
  ttft_ms: number | null
  tokens_per_second: number | null
  status_code: number
  error_code: string
}
export type MonitorBucket = {
  timestamp: number
  state: string
  count: number
  success_count: number
  latency_ms: number | null
  error_code?: string
}
export type ChannelMonitorSummary = {
  channel_id: number
  enabled: boolean
  registered: boolean
  state: string
  latest: MonitorObservation | null
  success_rate: number | null
  sample_count: number
  buckets: MonitorBucket[]
  truncated: boolean
}

export async function getChannelMonitoring(
  model: string,
  channelIds: number[],
  signal?: AbortSignal
) {
  const batches: number[][] = []
  for (let offset = 0; offset < channelIds.length; offset += 50) {
    batches.push(channelIds.slice(offset, offset + 50))
  }
  const responses = await Promise.all(
    batches.map(async (ids) => {
      const response = await api.get<{
        success: boolean
        message?: string
        data: ChannelMonitorSummary[]
      }>('/api/channel/monitoring', {
        params: { model, channel_ids: ids.join(',') },
        signal,
      })
      return requireServerSuccess(response.data).data
    })
  )
  return responses.flat()
}
