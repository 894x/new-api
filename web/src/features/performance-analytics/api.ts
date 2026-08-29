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
import type { PerformanceAnalyticsQueryParams } from '@/features/dashboard/lib/filters'
import { api } from '@/lib/api'

import type {
  PerformanceAnalyticsData,
  PerformanceAnalyticsOptions,
  PerformanceAnalyticsResponse,
} from './types'

function performanceAnalyticsPath(isAdmin: boolean, suffix = ''): string {
  return `/api/perf-analytics/${isAdmin ? 'admin' : 'self'}${suffix}`
}

export async function getPerformanceAnalytics(
  params: PerformanceAnalyticsQueryParams,
  isAdmin: boolean
): Promise<PerformanceAnalyticsResponse<PerformanceAnalyticsData>> {
  const response = await api.get(performanceAnalyticsPath(isAdmin), { params })
  return response.data
}

export async function getPerformanceAnalyticsOptions(
  isAdmin: boolean,
  userId?: number
): Promise<PerformanceAnalyticsResponse<PerformanceAnalyticsOptions>> {
  const response = await api.get(
    performanceAnalyticsPath(isAdmin, '/options'),
    {
      params: isAdmin && userId ? { user_id: userId } : undefined,
    }
  )
  return response.data
}
