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

export type GroupModelAccess = Record<string, string[]>

export interface GroupModelMatrixData {
  groups: string[]
  models: string[]
  available: Record<string, string[]>
  config: GroupModelAccess
  revision: string
}

export const groupModelMatrixKey = ['group-model-matrix'] as const

export async function getGroupModelMatrix(): Promise<GroupModelMatrixData> {
  const response = await api.get<{
    success: boolean
    data: GroupModelMatrixData
  }>('/api/channel/group-model-matrix')
  return requireServerSuccess(response.data).data
}

export async function saveGroupModelMatrix(
  config: GroupModelAccess,
  revision: string
): Promise<Pick<GroupModelMatrixData, 'config' | 'revision'>> {
  const response = await api.put<{
    success: boolean
    data: Pick<GroupModelMatrixData, 'config' | 'revision'>
  }>('/api/channel/group-model-matrix', { config, revision })
  return requireServerSuccess(response.data).data
}
