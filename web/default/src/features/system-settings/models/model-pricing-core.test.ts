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
  buildPreviewRows,
  createInitialLaneState,
  deriveModelRatioFromDisplayPrice,
  EMPTY_LANE_ENABLED,
  EMPTY_LANE_PRICES,
} from './model-pricing-core'

describe('model pricing currency-aware state', () => {
  it('derives the canonical model ratio from the display currency price', () => {
    expect(deriveModelRatioFromDisplayPrice('2', 1)).toBe('1')
    expect(deriveModelRatioFromDisplayPrice('14.6', 7.3)).toBe('1')
    expect(deriveModelRatioFromDisplayPrice('3', 7.3)).toBe('0.205479452055')
    expect(deriveModelRatioFromDisplayPrice('', 7.3)).toBe('')
    expect(deriveModelRatioFromDisplayPrice('3', 0)).toBe('')
  })

  it('converts stored USD token prices to CNY while preserving ratios', () => {
    const state = createInitialLaneState(
      {
        name: 'test-model',
        ratio: '1.5',
        completionRatio: '5',
        audioRatio: '1.27',
        audioCompletionRatio: '4',
      },
      7.3
    )

    expect(state.promptPrice).toBe('21.9')
    expect(state.prices.completion).toBe('109.5')
    expect(state.prices.audioInput).toBe('27.813')
    expect(state.prices.audioOutput).toBe('111.252')
  })

  it('uses the selected currency formatter in fixed-price previews', () => {
    const rows = buildPreviewRows(
      { name: 'test-model', price: '0.073' },
      'per-request',
      '',
      '',
      '',
      EMPTY_LANE_PRICES,
      EMPTY_LANE_ENABLED,
      (key) => key,
      (value) => `¥${value}`
    )

    expect(rows).toEqual([
      { key: 'price', label: 'ModelPrice', value: '¥0.073' },
    ])
  })
})
