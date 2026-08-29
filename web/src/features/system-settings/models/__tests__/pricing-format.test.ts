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

import { describe, it } from 'vitest'

import {
  formatDisplayPriceFromUSD,
  formatUSDPriceFromDisplay,
} from '../pricing-format'

describe('model pricing display currency conversion', () => {
  it('converts stored USD prices to CNY and back', () => {
    assert.equal(formatDisplayPriceFromUSD('3', 7.3), '21.9')
    assert.equal(formatUSDPriceFromDisplay('21.9', 7.3), '3')
  })

  it('preserves zero-priced models', () => {
    assert.equal(formatDisplayPriceFromUSD(0, 7.3), '0')
    assert.equal(formatUSDPriceFromDisplay(0, 7.3), '0')
  })

  for (const value of [null, undefined, '', false, 'not-a-price']) {
    it(`rejects invalid price value ${String(value)}`, () => {
      assert.equal(formatDisplayPriceFromUSD(value, 7.3), '')
      assert.equal(formatUSDPriceFromDisplay(value, 7.3), '')
    })
  }

  for (const exchangeRate of [0, -1, Number.NaN, Number.POSITIVE_INFINITY]) {
    it(`rejects invalid exchange rate ${String(exchangeRate)}`, () => {
      assert.equal(formatDisplayPriceFromUSD('3', exchangeRate), '')
      assert.equal(formatUSDPriceFromDisplay('21.9', exchangeRate), '')
    })
  }

  it('keeps small decimal prices stable through a round trip', () => {
    const displayPrice = formatDisplayPriceFromUSD('0.00000125', 7.3)
    assert.equal(formatUSDPriceFromDisplay(displayPrice, 7.3), '0.00000125')
  })

  it('removes floating-point drift from saved CNY model prices', () => {
    assert.equal(formatDisplayPriceFromUSD('1.095890410958', 7.3), '8')
    assert.equal(formatDisplayPriceFromUSD('3.424657534244', 7.3), '25')
  })
})
