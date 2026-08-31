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
import type { UpdateOptionRequest, UpdateOptionResponse } from '../../types'

type SaveChangedOptionFieldsArgs<T extends object> = {
  normalized: T
  baseline: T
  apiKeyMap: Partial<Record<keyof T, string>>
  mutateAsync: (request: UpdateOptionRequest) => Promise<UpdateOptionResponse>
  onFieldSaved: (nextBaseline: T) => void
}

type SaveChangedOptionFieldsResult<T> = {
  baseline: T
  allSucceeded: boolean
  hadChanges: boolean
}

export async function saveChangedOptionFields<T extends object>(
  args: SaveChangedOptionFieldsArgs<T>
): Promise<SaveChangedOptionFieldsResult<T>> {
  let nextBaseline = args.baseline
  const changedFields = (Object.keys(args.normalized) as Array<keyof T>).filter(
    (key) => args.normalized[key] !== args.baseline[key]
  )

  for (const key of changedFields) {
    let response: UpdateOptionResponse
    try {
      response = await args.mutateAsync({
        key: args.apiKeyMap[key] ?? String(key),
        value: args.normalized[key] as string | number | boolean,
      })
    } catch {
      return {
        baseline: nextBaseline,
        allSucceeded: false,
        hadChanges: true,
      }
    }
    if (!response.success) {
      return {
        baseline: nextBaseline,
        allSucceeded: false,
        hadChanges: changedFields.length > 0,
      }
    }

    nextBaseline = {
      ...nextBaseline,
      [key]: args.normalized[key],
    }
    args.onFieldSaved(nextBaseline)
  }

  return {
    baseline: nextBaseline,
    allSucceeded: true,
    hadChanges: changedFields.length > 0,
  }
}
