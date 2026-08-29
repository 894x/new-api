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

import * as filterModule from '../lib/filters'

type BuildPerformanceAnalyticsParams = (
  filters: {
    model: string
    startTimestamp: number
    endTimestamp: number
    userId?: number
    tokenId?: number
  },
  isAdmin: boolean
) => Record<string, string | number>

function getBuilder(): BuildPerformanceAnalyticsParams {
  const candidate = (filterModule as unknown as Record<string, unknown>)
    .buildPerformanceAnalyticsParams
  assert.equal(typeof candidate, 'function')
  return candidate as BuildPerformanceAnalyticsParams
}

describe('performance analytics query scoping', () => {
  test('ordinary users never send a requested user id', () => {
    const buildParams = getBuilder()

    assert.deepEqual(
      buildParams(
        {
          model: 'gpt-test',
          startTimestamp: 100,
          endTimestamp: 200,
          userId: 99,
          tokenId: 11,
        },
        false
      ),
      {
        model: 'gpt-test',
        start_timestamp: 100,
        end_timestamp: 200,
        token_id: 11,
      }
    )
  })

  test('administrators send a key only inside its selected user scope', () => {
    const buildParams = getBuilder()
    const filters = {
      model: 'gpt-test',
      startTimestamp: 100,
      endTimestamp: 200,
      tokenId: 22,
    }

    assert.deepEqual(buildParams(filters, true), {
      model: 'gpt-test',
      start_timestamp: 100,
      end_timestamp: 200,
    })
    assert.deepEqual(buildParams({ ...filters, userId: 2 }, true), {
      model: 'gpt-test',
      start_timestamp: 100,
      end_timestamp: 200,
      user_id: 2,
      token_id: 22,
    })
  })
})
