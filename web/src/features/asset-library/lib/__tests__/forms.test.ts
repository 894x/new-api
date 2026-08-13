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

import {
  assetFormSchema,
  channelAssetConfigDestinationChanged,
  channelAssetConfigFormSchema,
  getChannelAssetConfigPayload,
} from '../forms'

describe('asset library forms', () => {
  test('accepts only public HTTP or HTTPS asset URLs', () => {
    const base = {
      name: '',
      groupId: 'group-1',
      assetType: 'Image' as const,
    }

    assert.equal(
      assetFormSchema.safeParse({ ...base, url: 'https://cdn.example/a.png' })
        .success,
      true
    )
    assert.equal(
      assetFormSchema.safeParse({ ...base, url: 'file:///tmp/a.png' }).success,
      false
    )
    assert.equal(
      assetFormSchema.safeParse({ ...base, url: 'blob:test' }).success,
      false
    )
  })

  test('allows stored AK and SK to remain blank when editing', () => {
    const result = channelAssetConfigFormSchema.safeParse({
      enabled: true,
      baseUrl: 'https://ark.cn-beijing.volcengineapi.com',
      authType: 'aksk',
      accessKey: '',
      secretKey: '',
      apiKey: '',
      region: 'cn-beijing',
      projectName: 'default',
      hasAccessKey: true,
      hasSecretKey: true,
      hasApiKey: false,
    })

    assert.equal(result.success, true)
  })

  test('omits blank credentials so the backend preserves stored secrets', () => {
    assert.deepEqual(
      getChannelAssetConfigPayload({
        enabled: true,
        baseUrl: ' https://assets.example.com/ ',
        authType: 'bearer',
        accessKey: '',
        secretKey: '',
        apiKey: '',
        region: ' cn-beijing ',
        projectName: ' default ',
        hasAccessKey: false,
        hasSecretKey: false,
        hasApiKey: true,
      }),
      {
        enabled: true,
        base_url: 'https://assets.example.com/',
        auth_type: 'bearer',
        region: 'cn-beijing',
        project_name: 'default',
      }
    )
  })

  test('requires fresh credentials when the upstream destination changes', () => {
    const config = {
      channel_id: 3,
      enabled: true,
      base_url: 'https://old.example.com',
      auth_type: 'bearer' as const,
      region: 'cn-beijing',
      project_name: 'default',
      has_access_key: false,
      has_secret_key: false,
      has_api_key: true,
      replica_count: 0,
    }

    assert.equal(
      channelAssetConfigDestinationChanged(
        { baseUrl: 'https://old.example.com/', authType: 'bearer' },
        config
      ),
      false
    )
    assert.equal(
      channelAssetConfigDestinationChanged(
        { baseUrl: 'https://new.example.com', authType: 'bearer' },
        config
      ),
      true
    )
    assert.equal(
      channelAssetConfigDestinationChanged(
        { baseUrl: 'https://old.example.com', authType: 'aksk' },
        config
      ),
      true
    )
  })
})
