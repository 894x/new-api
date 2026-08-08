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
import { describe, it } from 'node:test'

import { resolveHomePageTemplate } from './use-home-page-content'

describe('resolveHomePageTemplate', () => {
  const cases = [
    ['system', '', 'system'],
    ['quality', '', 'quality'],
    ['economy', '', 'economy'],
    ['business', '', 'business'],
    ['custom', '# Welcome', 'custom'],
    ['', '', 'system'],
    ['', '# Legacy custom homepage', 'custom'],
    ['unknown', '', 'system'],
    ['unknown', '<h1>Legacy homepage</h1>', 'custom'],
  ]

  for (const [configuredTemplate, content, expected] of cases) {
    it(`resolves configured template ${configuredTemplate} with content ${content}`, () => {
      assert.equal(resolveHomePageTemplate(configuredTemplate, content), expected)
    })
  }
})
