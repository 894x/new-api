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
import { describe, test } from 'node:test'

import { calculateGenerationTokensPerSecond } from '../format'

describe('stream generation speed', () => {
  test('excludes time to first token and the first output token', () => {
    const tokensPerSecond = calculateGenerationTokensPerSecond(101, 12, 2000)

    assert.equal(tokensPerSecond, 10)
  })

  test('omits speed when the generation interval cannot be measured', () => {
    const invalidMeasurements = [
      calculateGenerationTokensPerSecond(1, 12, 2000),
      calculateGenerationTokensPerSecond(10, 2, 2000),
      calculateGenerationTokensPerSecond(10, 12),
      calculateGenerationTokensPerSecond(10, 12, 0),
    ]

    assert.deepEqual(invalidMeasurements, [null, null, null, null])
  })
})
