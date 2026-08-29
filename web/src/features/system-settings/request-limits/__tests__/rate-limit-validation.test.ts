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
import assert from 'node:assert/strict'

import { describe, test } from 'vitest'

import { isValidRateLimitJSON } from '../rate-limit-validation'

describe('group rate limit JSON validation', () => {
  test('accepts legacy limits and limits with TPM', () => {
    assert.equal(isValidRateLimitJSON('{"default":[200,100]}'), true)
    assert.equal(isValidRateLimitJSON('{"vip":[0,1000,60000]}'), true)
  })

  test('rejects tuple lengths other than the legacy and TPM forms', () => {
    for (const value of [
      '{"default":[200]}',
      '{"default":[200,100,60000,1]}',
    ]) {
      assert.equal(isValidRateLimitJSON(value), false)
    }
  })

  test('rejects invalid TPM values', () => {
    for (const value of [
      '{"default":[200,100,-1]}',
      '{"default":[200,100,1.5]}',
      '{"default":[200,100,2147483648]}',
    ]) {
      assert.equal(isValidRateLimitJSON(value), false)
    }
  })
})
