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
import { channelsQueryKeys } from '@/features/channels/lib/channel-actions'
import { api } from '@/lib/api'

import type { MatrixData, MatrixSearch } from './types'

export const matrixQueryKeys = {
  all: [...channelsQueryKeys.modelRoutingOverrides(), 'matrix'] as const,
  list: (search: MatrixSearch) => [...matrixQueryKeys.all, search] as const,
}

export async function getModelChannelMatrix(
  search: MatrixSearch
): Promise<MatrixData> {
  const response = await api.get<{ success: boolean; data?: MatrixData }>(
    '/api/channel/model-matrix',
    { params: { ...search, model_page_size: 25, channel_page_size: 10 } }
  )
  if (!response.data.success || !response.data.data) {
    throw new Error('Failed to load model-channel matrix.')
  }
  return response.data.data
}
