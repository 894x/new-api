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

import { buildLogExportRequest, getLogExportFields } from '../log-export'

describe('usage log export', () => {
  test('never exposes upstream fields in the downstream field catalog', () => {
    const downstreamKeys = new Set(
      getLogExportFields('downstream').map((field) => field.key)
    )

    assert.equal(downstreamKeys.has('channel_id'), false)
    assert.equal(downstreamKeys.has('channel_name'), false)
    assert.equal(downstreamKeys.has('upstream_request_id'), false)
    assert.equal(downstreamKeys.has('upstream_model_name'), false)
    assert.equal(downstreamKeys.has('error_message'), false)
  })

  test('uses active reconciliation filters without table pagination or log type', () => {
    const request = buildLogExportRequest(
      'upstream',
      ['request_id', 'channel_id'],
      {
        p: 3,
        page_size: 100,
        type: 5,
        start_timestamp: 100,
        end_timestamp: 200,
        model_name: 'gpt-x',
        channel: 10,
      }
    )

    assert.deepEqual(request, {
      view: 'upstream',
      fields: ['request_id', 'channel_id'],
      start_timestamp: 100,
      end_timestamp: 200,
      model_name: 'gpt-x',
      channel: 10,
    })
  })

  test('removes upstream-only filters from downstream requests', () => {
    const request = buildLogExportRequest('downstream', ['request_id'], {
      channel: 10,
      upstream_request_id: 'provider-request',
      request_id: 'client-request',
    })

    assert.deepEqual(request, {
      view: 'downstream',
      fields: ['request_id'],
      request_id: 'client-request',
    })
  })
})
