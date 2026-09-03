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
import { afterEach, describe, expect, test, vi } from 'vitest'

import { api } from '@/lib/api'

import { listAssets } from './api'

type ApiPost = typeof api.post
const originalPost = api.post

afterEach(() => {
  api.post = originalPost
})

describe('admin asset library queries', () => {
  test('routes a target-user read through the read-only admin endpoint', async () => {
    const post = vi.fn(async () => ({
      data: {
        Result: { TotalCount: 0, Items: [], PageNumber: 1, PageSize: 20 },
      },
    }))
    api.post = post as unknown as ApiPost

    await listAssets({ PageNumber: 1, PageSize: 20 }, 42)

    expect(post).toHaveBeenCalledWith(
      '/api/asset-library/admin/users/42',
      { PageNumber: 1, PageSize: 20 },
      expect.objectContaining({
        params: { Action: 'ListAssets', Version: '2024-01-01' },
      })
    )
  })
})
