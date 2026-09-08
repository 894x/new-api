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

export interface AssetLibraryTimingStage {
  id?: number
  stage: string
  operation?: string
  asset_id?: string
  channel_id?: number
  started_at_ms: number
  duration_ms: number
  outcome: string
  http_status?: number
  upstream_request_id?: string
  error_code?: string
  error_kind?: string
  bytes?: number
  held_ms?: number
  backend?: string
}

export interface AssetLibraryReadiness {
  asset_id: string
  channel_id: number
  status: string
  error_code?: string
  error_message?: string
  upload_started_at_ms?: number
  submitted_at_ms?: number
  first_active_at_ms?: number
  last_polled_at_ms?: number
  last_processing_at_ms?: number
  poll_count: number
}

export interface AssetLibraryTiming {
  version: number
  request_id: string
  action: string
  asset_id?: string
  started_at_ms: number
  completed_at_ms: number
  duration_ms: number
  outcome: string
  stages: AssetLibraryTimingStage[]
  replicas?: AssetLibraryReadiness[]
  dropped_stages?: number
}
