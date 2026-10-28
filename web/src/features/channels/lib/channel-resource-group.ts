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
import type { TFunction } from 'i18next'
import { z } from 'zod'

export function createChannelResourceGroupSchema(
  t: TFunction
): z.ZodObject<{ name: z.ZodString }> {
  return z.object({
    name: z
      .string()
      .trim()
      .refine(
        (value) =>
          value.startsWith('资源-') &&
          value.slice(3).trim().length > 0 &&
          [...value].length <= 64 &&
          !/[,\p{Cc}]/u.test(value),
        t(
          'Use the 资源- prefix and a name of up to 64 characters without commas or control characters.'
        )
      ),
  })
}

export type ChannelResourceGroupValues = z.infer<
  ReturnType<typeof createChannelResourceGroupSchema>
>
