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
export interface GrossProfitTotals {
  known_requests: number
  known_revenue_usd: number
  cost_usd: number
  net_revenue_usd: number
  net_cost_usd: number
  output_vat_usd: number
  input_vat_usd: number
  profit_usd: number
  profit_margin: number | null
  revenue_coverage: number | null
  unknown_requests: number
  unknown_revenue_usd: number
}

export interface GrossProfitChannel extends GrossProfitTotals {
  channel_id: number
  channel_name: string
}

export interface GrossProfitUnknown {
  channel_id: number
  channel_name: string
  reason: string
  requests: number
  revenue_usd: number
}

export interface GrossProfitReport {
  timezone: string
  aggregation: {
    backfill_complete: boolean
    pending_events: number
    updated_at: number
  }
  summary: GrossProfitTotals
  channels: GrossProfitChannel[]
  unknown: GrossProfitUnknown[]
  series: (GrossProfitTotals & { timestamp: number; day: string })[]
}
