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

import { describe, expect, it } from 'vitest'

import { buildRequestTimingSegments } from './request-timing'

describe('buildRequestTimingSegments', () => {
  it('turns ordered request milestones into exact phase durations', () => {
    const result = buildRequestTimingSegments({
      request_received_at_ms: 1_000,
      request_body_read_at_ms: 1_020,
      upstream_request_started_at_ms: 1_070,
      upstream_request_written_at_ms: 1_100,
      upstream_response_headers_at_ms: 1_400,
      first_response_at_ms: 1_700,
      request_completed_at_ms: 2_500,
    })

    expect(result.totalDurationMs).toBe(1_500)
    expect(result.segments.map((segment) => segment.key)).toEqual([
      'client_upload',
      'gateway_processing',
      'upstream_upload',
      'upstream_wait',
      'first_response',
      'streaming',
    ])
    expect(result.segments.map((segment) => segment.durationMs)).toEqual([
      20, 50, 30, 300, 300, 800,
    ])
    expect(result.longestSegmentKey).toBe('streaming')
  })

  it('omits intervals whose boundary was not observed', () => {
    const result = buildRequestTimingSegments({
      request_received_at_ms: 1_000,
      upstream_request_started_at_ms: 1_070,
      upstream_request_written_at_ms: 1_100,
      upstream_response_headers_at_ms: 1_400,
      request_completed_at_ms: 2_500,
    })

    expect(result.segments.map((segment) => segment.key)).toEqual([
      'upstream_upload',
      'upstream_wait',
    ])
    expect(result.totalDurationMs).toBe(1_500)
  })

  it('rejects reversed timestamps instead of rendering negative time', () => {
    const result = buildRequestTimingSegments({
      request_received_at_ms: 2_000,
      request_body_read_at_ms: 1_000,
    })

    expect(result.segments).toEqual([])
    expect(result.totalDurationMs).toBe(0)
    expect(result.longestSegmentKey).toBeNull()
  })
})
