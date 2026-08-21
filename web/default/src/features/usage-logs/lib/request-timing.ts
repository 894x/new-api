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

import type { RequestTimingInfo } from '../types'

export type RequestTimingSegmentKey =
  | 'client_upload'
  | 'gateway_processing'
  | 'upstream_upload'
  | 'upstream_wait'
  | 'first_response'
  | 'streaming'

export interface RequestTimingSegment {
  key: RequestTimingSegmentKey
  labelKey: string
  durationMs: number
}

interface RequestTimingResult {
  segments: RequestTimingSegment[]
  totalDurationMs: number
  longestSegmentKey: RequestTimingSegmentKey | null
}

const intervalDefinitions: Array<{
  key: RequestTimingSegmentKey
  labelKey: string
  start: keyof RequestTimingInfo
  end: keyof RequestTimingInfo
}> = [
  {
    key: 'client_upload',
    labelKey: 'Request Intake',
    start: 'request_received_at_ms',
    end: 'request_body_read_at_ms',
  },
  {
    key: 'gateway_processing',
    labelKey: 'Gateway Processing',
    start: 'request_body_read_at_ms',
    end: 'upstream_request_started_at_ms',
  },
  {
    key: 'upstream_upload',
    labelKey: 'Upstream Upload',
    start: 'upstream_request_started_at_ms',
    end: 'upstream_request_written_at_ms',
  },
  {
    key: 'upstream_wait',
    labelKey: 'Upstream Wait',
    start: 'upstream_request_written_at_ms',
    end: 'upstream_response_headers_at_ms',
  },
  {
    key: 'first_response',
    labelKey: 'First Response',
    start: 'upstream_response_headers_at_ms',
    end: 'first_response_at_ms',
  },
  {
    key: 'streaming',
    labelKey: 'Streaming',
    start: 'first_response_at_ms',
    end: 'request_completed_at_ms',
  },
]

function isTimestamp(value: number | undefined): value is number {
  return value != null && Number.isFinite(value) && value > 0
}

export function buildRequestTimingSegments(
  timing: RequestTimingInfo
): RequestTimingResult {
  const segments = intervalDefinitions.flatMap((definition) => {
    const start = timing[definition.start]
    const end = timing[definition.end]
    if (!isTimestamp(start) || !isTimestamp(end) || end < start) return []
    return [
      {
        key: definition.key,
        labelKey: definition.labelKey,
        durationMs: end - start,
      },
    ]
  })

  const receivedAt = timing.request_received_at_ms
  const completedAt = timing.request_completed_at_ms
  const totalDurationMs =
    isTimestamp(receivedAt) &&
    isTimestamp(completedAt) &&
    completedAt >= receivedAt
      ? completedAt - receivedAt
      : 0

  let longestSegmentKey: RequestTimingSegmentKey | null = null
  let longestDurationMs = -1
  for (const segment of segments) {
    if (segment.durationMs > longestDurationMs) {
      longestDurationMs = segment.durationMs
      longestSegmentKey = segment.key
    }
  }

  return { segments, totalDurationMs, longestSegmentKey }
}
