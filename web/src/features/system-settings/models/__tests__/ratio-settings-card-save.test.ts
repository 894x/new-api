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
import { describe, expect, test, vi } from 'vitest'

import { saveChangedOptionFields } from '../lib/save-changed-option-fields'

describe('ratio settings partial-save baseline', () => {
  test('stops on an unsuccessful response and retries only failed or unattempted fields', async () => {
    const baseline = {
      GroupRatio: '{"premium":1}',
      TopupGroupRatio: '{}',
      ModelTieredRatios: '{}',
    }
    const normalized = {
      GroupRatio: '{"premium":0.9}',
      TopupGroupRatio: '{"premium":1.1}',
      ModelTieredRatios: '{"premium":{}}',
    }
    const firstMutate = vi
      .fn()
      .mockResolvedValueOnce({ success: true, message: '' })
      .mockResolvedValueOnce({ success: false, message: 'rejected' })
    const committedBaselines: (typeof baseline)[] = []

    const afterFailure = await saveChangedOptionFields({
      normalized,
      baseline,
      apiKeyMap: {
        ModelTieredRatios: 'group_ratio_setting.model_tiered_ratios',
      },
      mutateAsync: firstMutate,
      onFieldSaved: (nextBaseline) => committedBaselines.push(nextBaseline),
    })

    expect(firstMutate.mock.calls.map(([request]) => request.key)).toEqual([
      'GroupRatio',
      'TopupGroupRatio',
    ])
    expect(afterFailure).toEqual({
      baseline: {
        ...baseline,
        GroupRatio: normalized.GroupRatio,
      },
      allSucceeded: false,
      hadChanges: true,
    })
    expect(committedBaselines).toEqual([afterFailure.baseline])

    const retryMutate = vi
      .fn()
      .mockResolvedValue({ success: true, message: '' })
    const afterRetry = await saveChangedOptionFields({
      normalized,
      baseline: afterFailure.baseline,
      apiKeyMap: {
        ModelTieredRatios: 'group_ratio_setting.model_tiered_ratios',
      },
      mutateAsync: retryMutate,
      onFieldSaved: vi.fn(),
    })

    expect(retryMutate.mock.calls.map(([request]) => request.key)).toEqual([
      'TopupGroupRatio',
      'group_ratio_setting.model_tiered_ratios',
    ])
    expect(afterRetry).toEqual({
      baseline: normalized,
      allSucceeded: true,
      hadChanges: true,
    })
  })
})
