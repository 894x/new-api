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
import { z } from 'zod'

import type { ModelRoutingOverride } from '@/features/channels/types'

export const matrixSearchSchema = z.object({
  model: z.string().max(255).catch('').default(''),
  channel: z.string().max(255).catch('').default(''),
  group: z.string().max(255).catch('').default(''),
  status: z.enum(['enabled', 'all']).catch('enabled').default('enabled'),
  model_page: z
    .number()
    .int()
    .min(1)
    .max(Number.MAX_SAFE_INTEGER)
    .catch(1)
    .default(1),
  channel_page: z
    .number()
    .int()
    .min(1)
    .max(Number.MAX_SAFE_INTEGER)
    .catch(1)
    .default(1),
})

export type MatrixSearch = z.infer<typeof matrixSearchSchema>

export interface MatrixChannel {
  id: number
  name: string
  type: number
  status: number
  groups: string[]
}

export interface MatrixCell extends ModelRoutingOverride {
  upstream_model: string
  mapping_error: boolean
}

export interface MatrixData {
  models: string[]
  channels: MatrixChannel[]
  cells: MatrixCell[]
  groups: string[]
  model_total: number
  channel_total: number
  model_page: number
  channel_page: number
  model_page_size: number
  channel_page_size: number
}
