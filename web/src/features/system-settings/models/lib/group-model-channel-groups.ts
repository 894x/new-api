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

const modelName = z
  .string()
  .min(1)
  .refine((value) => value.trim() === value && value !== '__proto__')
const groupName = modelName.refine((value) => value !== 'auto')

export const groupModelChannelGroupsSchema = z.record(
  groupName,
  z.record(
    modelName,
    z
      .array(groupName)
      .refine((groups) => new Set(groups).size === groups.length)
  )
)

export type GroupModelChannelGroups = z.infer<
  typeof groupModelChannelGroupsSchema
>

export function parseGroupModelChannelGroups(
  value: string
): GroupModelChannelGroups | null {
  try {
    const result = groupModelChannelGroupsSchema.safeParse(JSON.parse(value))
    return result.success ? result.data : null
  } catch {
    return null
  }
}
