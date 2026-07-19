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
  formatDisplayPriceFromUSD,
  formatUSDPriceFromDisplay,
} from './pricing-format'

describe('model pricing display currency conversion', () => {
  it('converts stored USD prices to CNY and back', () => {
    expect(formatDisplayPriceFromUSD('3', 7.3)).toBe('21.9')
    expect(formatUSDPriceFromDisplay('21.9', 7.3)).toBe('3')
  })

  it('preserves zero-priced models', () => {
    expect(formatDisplayPriceFromUSD(0, 7.3)).toBe('0')
    expect(formatUSDPriceFromDisplay(0, 7.3)).toBe('0')
  })

  it.each([null, undefined, '', false, 'not-a-price'])(
    'rejects invalid price value %j',
    (value) => {
      expect(formatDisplayPriceFromUSD(value, 7.3)).toBe('')
      expect(formatUSDPriceFromDisplay(value, 7.3)).toBe('')
    }
  )

  it.each([0, -1, Number.NaN, Number.POSITIVE_INFINITY])(
    'rejects invalid exchange rate %j',
    (exchangeRate) => {
      expect(formatDisplayPriceFromUSD('3', exchangeRate)).toBe('')
      expect(formatUSDPriceFromDisplay('21.9', exchangeRate)).toBe('')
    }
  )

  it('keeps small decimal prices stable through a round trip', () => {
    const displayPrice = formatDisplayPriceFromUSD('0.00000125', 7.3)
    expect(formatUSDPriceFromDisplay(displayPrice, 7.3)).toBe('0.00000125')
  })
})
