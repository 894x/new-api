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
import { describe, expect, it } from 'vitest'

import {
  formatErrorResponseReplacementRules,
  parseErrorResponseReplacementRules,
} from '../error-response-replacement-rules'

describe('error response replacement rules', () => {
  it('normalizes valid rule text and preserves rule order', () => {
    const rules = parseErrorResponseReplacementRules(`[
      {"status_code":500,"match":" Moonshot AI ","replacement":" Retry later. "},
      {"status_code":429,"match":"rate limit","replacement":"Slow down."}
    ]`)

    expect(rules).toEqual([
      {
        status_code: 500,
        match: 'Moonshot AI',
        replacement: 'Retry later.',
      },
      {
        status_code: 429,
        match: 'rate limit',
        replacement: 'Slow down.',
      },
    ])
    expect(formatErrorResponseReplacementRules(rules)).toContain(
      '"status_code": 500'
    )
  })

  it('treats a blank editor as an empty rule list', () => {
    expect(parseErrorResponseReplacementRules('   ')).toEqual([])
  })

  it.each([
    'null',
    '[{"status_code":200,"match":"error","replacement":"retry"}]',
    '[{"status_code":500,"match":"","replacement":"retry"}]',
    '[{"status_code":500,"match":"error","replacement":"retry","extra":true}]',
  ])('rejects invalid configuration: %s', (value) => {
    expect(() => parseErrorResponseReplacementRules(value)).toThrow()
  })

  it('allows an empty replacement to remove matched text', () => {
    expect(
      parseErrorResponseReplacementRules(
        '[{"status_code":500,"match":"Moonshot AI","replacement":""}]'
      )
    ).toEqual([{ status_code: 500, match: 'Moonshot AI', replacement: '' }])
  })
})
