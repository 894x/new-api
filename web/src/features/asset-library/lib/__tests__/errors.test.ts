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

import { getAssetLibraryErrorMessage } from '../errors'

describe('asset library error messages', () => {
  test('uses the official error envelope from an Axios HTTP failure', () => {
    const error = Object.assign(new Error('Request failed with status 409'), {
      response: {
        data: {
          ResponseMetadata: {
            Error: {
              Code: 'AssetGroupNotEmpty',
              Message: 'delete all assets in the group first',
            },
          },
        },
      },
    })

    assert.equal(
      getAssetLibraryErrorMessage(error, 'Fallback'),
      'AssetGroupNotEmpty: delete all assets in the group first'
    )
  })
})
