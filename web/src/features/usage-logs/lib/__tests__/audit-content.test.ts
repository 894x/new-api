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

import { createInstance } from 'i18next'
import { describe, test } from 'vitest'

import en from '@/i18n/locales/en.json'

import { parseAuditChangedFields, renderAuditContent } from '../format'

describe('asset library audit content', () => {
  test('normalizes current and legacy changed field descriptors', () => {
    assert.deepEqual(parseAuditChangedFields(['enabled', 'backend']), [
      'enabled',
      'backend',
    ])
    assert.deepEqual(parseAuditChangedFields('enabled, backend'), [
      'enabled',
      'backend',
    ])
  })

  test('renders each asset library mutation as a human-readable operation', async () => {
    const i18n = createInstance()
    await i18n.init({
      lng: 'en',
      resources: { en },
      interpolation: { escapeValue: false },
    })

    const cases: Array<{
      action: string
      params: Record<string, string | number | boolean | string[]>
      expected: string
    }> = [
      {
        action: 'asset_library.group.create',
        params: { id: 'group-na-1', name: 'Characters' },
        expected: 'Created asset group Characters (ID: group-na-1)',
      },
      {
        action: 'asset_library.group.update',
        params: { id: 'group-na-1', name: 'Heroes' },
        expected: 'Updated asset group Heroes (ID: group-na-1)',
      },
      {
        action: 'asset_library.group.delete',
        params: { id: 'group-na-1', name: 'Heroes' },
        expected: 'Deleted asset group Heroes (ID: group-na-1)',
      },
      {
        action: 'asset_library.asset.create',
        params: {
          id: 'asset-na-1',
          asset_type: 'Image',
          group_id: 'group-na-1',
        },
        expected:
          'Created asset (ID: asset-na-1, type: Image, group: group-na-1)',
      },
      {
        action: 'asset_library.asset.update',
        params: {
          id: 'asset-na-1',
          asset_type: 'Image',
          group_id: 'group-na-1',
        },
        expected:
          'Updated asset (ID: asset-na-1, type: Image, group: group-na-1)',
      },
      {
        action: 'asset_library.asset.delete',
        params: {
          id: 'asset-na-1',
          asset_type: 'Image',
          group_id: 'group-na-1',
        },
        expected:
          'Deleted asset (ID: asset-na-1, type: Image, group: group-na-1)',
      },
      {
        action: 'channel.asset_library.update',
        params: { id: 17 },
        expected: 'Updated asset library config for channel (ID: 17)',
      },
      {
        action: 'channel.asset_library.delete',
        params: { id: 17 },
        expected: 'Deleted asset library config for channel (ID: 17)',
      },
      {
        action: 'channel.asset_library.sync',
        params: { id: 17 },
        expected: 'Synchronized asset library replicas for channel (ID: 17)',
      },
      {
        action: 'asset_library.asset.sync',
        params: { id: 'asset-na-1', error_count: 1 },
        expected: 'Synchronized asset asset-na-1 (errors: 1)',
      },
      {
        action: 'asset_library.group.sync',
        params: { id: 'group-na-1', error_count: 2 },
        expected: 'Synchronized asset group group-na-1 (errors: 2)',
      },
    ]

    for (const testCase of cases) {
      const rendered = renderAuditContent(
        { op: { action: testCase.action, params: testCase.params } },
        i18n.t.bind(i18n)
      )
      assert.equal(rendered, testCase.expected, testCase.action)
    }
  })
})
