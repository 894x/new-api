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
import * as z from 'zod'

import type { ErrorResponseReplacementRule } from '../types'

const maxRuleCount = 32
const maxRuleTextLengthBytes = 4096

const boundedMatchText = z
  .string()
  .trim()
  .min(1)
  .refine(
    (value) => new TextEncoder().encode(value).length <= maxRuleTextLengthBytes
  )

const boundedReplacementText = z
  .string()
  .trim()
  .refine(
    (value) => new TextEncoder().encode(value).length <= maxRuleTextLengthBytes
  )

const errorResponseReplacementRuleSchema = z
  .object({
    status_code: z.number().int().min(400).max(599),
    match: boundedMatchText,
    replacement: boundedReplacementText,
  })
  .strict()

const errorResponseReplacementRulesSchema = z
  .array(errorResponseReplacementRuleSchema)
  .max(maxRuleCount)

export const errorResponseReplacementRulesTextSchema = z.string().refine(
  (value) => {
    try {
      parseErrorResponseReplacementRules(value)
      return true
    } catch {
      return false
    }
  },
  {
    message:
      'Enter a JSON array with up to 32 rules. Each rule needs status_code (400-599), match, and replacement.',
  }
)

export function parseErrorResponseReplacementRules(
  value: string
): ErrorResponseReplacementRule[] {
  const source = value.trim() === '' ? '[]' : value
  return errorResponseReplacementRulesSchema.parse(JSON.parse(source))
}

export function formatErrorResponseReplacementRules(
  rules: ErrorResponseReplacementRule[]
): string {
  return JSON.stringify(rules, null, 2)
}
