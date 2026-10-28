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

import { describe, expect, test, vi } from 'vitest'

import { api } from '@/lib/api'

import { exportLogs } from '../../api'
import {
  buildLogExportRequest,
  getDefaultLogExportFields,
  getLogExportFields,
} from '../log-export'

describe('usage log export', () => {
  test('offers only the personal consumption field allowlist in self mode', () => {
    const fields = [
      'created_at',
      'request_id',
      'consumption_type',
      'token_name',
      'model_name',
      'input_tokens',
      'cached_input_tokens',
      'output_tokens',
      'charged_quota',
      'actual_amount_usd',
    ]
    expect(getLogExportFields('self').map((field) => field.key)).toEqual(fields)
    expect(getDefaultLogExportFields('self')).toEqual(fields)
  })

  test('drops identity, channel, upstream and arbitrary parameters in self requests', () => {
    const request = buildLogExportRequest(
      'self',
      ['request_id', 'channel_id', 'group_ratio', 'charged_quota'],
      {
        p: 3,
        page_size: 100,
        type: 5,
        username: 'another-user',
        channel: 10,
        group: 'internal-group',
        upstream_request_id: 'provider-request',
        user_id: 99,
        arbitrary_filter: 'private',
        start_timestamp: 100,
        end_timestamp: 200,
        model_name: 'gpt-x',
        token_name: 'personal-key',
        request_id: 'client-request',
      } as Parameters<typeof buildLogExportRequest>[2]
    )
    expect(request).toEqual({
      view: 'self',
      fields: ['request_id', 'charged_quota'],
      start_timestamp: 100,
      end_timestamp: 200,
      model_name: 'gpt-x',
      token_name: 'personal-key',
      request_id: 'client-request',
    })
  })

  test.each([
    ['self', '/api/log/self/export'],
    ['upstream', '/api/log/export'],
    ['downstream', '/api/log/export'],
  ] as const)(
    'uses the correct endpoint for %s exports',
    async (view, endpoint) => {
      const request = { view, fields: ['request_id'] }
      const blob = new Blob(['archive'])
      const post = vi.spyOn(api, 'post').mockResolvedValue({
        data: blob,
        headers: { 'content-type': 'application/zip' },
      })
      const result = await exportLogs(request)
      expect(post).toHaveBeenCalledWith(
        endpoint,
        request,
        expect.objectContaining({ responseType: 'blob' })
      )
      expect(result).toEqual({ blob, filename: `usage-${view}-export.zip` })
    }
  )

  test('uses the reconciliation-focused upstream fields by default', () => {
    assert.deepEqual(getDefaultLogExportFields('upstream'), [
      'created_at',
      'upstream_request_id',
      'status',
      'channel_name',
      'model_name',
      'upstream_model_name',
      'input_tokens',
      'cached_input_tokens',
      'output_tokens',
      'model_price',
      'completion_ratio',
      'cache_ratio',
      'actual_amount_usd',
      'error_message',
    ])
  })

  test('uses the customer reconciliation fields by default downstream', () => {
    assert.deepEqual(getDefaultLogExportFields('downstream'), [
      'created_at',
      'request_id',
      'status',
      'user_id',
      'username',
      'token_name',
      'group',
      'model_name',
      'input_tokens',
      'cached_input_tokens',
      'output_tokens',
      'model_price',
      'completion_ratio',
      'cache_ratio',
      'group_ratio',
      'actual_amount_usd',
    ])
  })

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
