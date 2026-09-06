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
import { describe, expect, test } from 'vitest'

import { calculatePresetPricing } from './format'

describe('calculatePresetPricing', () => {
  test('treats CNY presets as direct CNY amounts', () => {
    expect(calculatePresetPricing(100, 7.3, 1, 7.3)).toEqual({
      displayValue: 100,
      originalPrice: 100,
      actualPrice: 100,
      savedAmount: 0,
      hasDiscount: false,
    })
  })

  test('keeps USD presets priced through the payment rate', () => {
    expect(calculatePresetPricing(10, 7.3, 1, 1)).toEqual({
      displayValue: 10,
      originalPrice: 73,
      actualPrice: 73,
      savedAmount: 0,
      hasDiscount: false,
    })
  })
})
