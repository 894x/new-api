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

import type { TFunction } from 'i18next'

import {
  DASHBOARD_SECTION_IDS,
  getDashboardSectionNavItems,
} from '../section-registry'

const translate = ((key: string) => key) as TFunction

describe('dashboard performance analytics navigation', () => {
  test('shows performance analytics to ordinary users while keeping user analytics hidden', () => {
    const items = getDashboardSectionNavItems(translate, { isAdmin: false })

    assert.ok(DASHBOARD_SECTION_IDS.includes('performance'))
    assert.ok(items.some((item) => item.title === 'Performance Analytics'))
    assert.ok(!items.some((item) => item.title === 'User Analytics'))
  })

  test('shows both performance and user analytics to administrators', () => {
    const items = getDashboardSectionNavItems(translate, { isAdmin: true })

    assert.ok(items.some((item) => item.title === 'Performance Analytics'))
    assert.ok(items.some((item) => item.title === 'User Analytics'))
  })
})
