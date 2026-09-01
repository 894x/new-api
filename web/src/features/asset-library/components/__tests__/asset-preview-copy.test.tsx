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
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { beforeEach, describe, expect, test, vi } from 'vitest'

import { useAuthStore } from '@/stores/auth-store'

import type { Asset } from '../../types'
import { AssetPreviewDialog } from '../asset-preview-dialog'

const { copyToClipboard } = vi.hoisted(() => ({
  copyToClipboard: vi.fn(async () => true),
}))

vi.mock('@/lib/copy-to-clipboard', () => ({ copyToClipboard }))

const imageAsset: Asset = {
  Id: 'asset-na-preview',
  Name: 'Launch hero',
  URL: 'https://example.com/launch-hero.png',
  GroupId: 'group-na-brand',
  AssetType: 'Image',
  Status: 'Active',
  ProjectName: 'default',
  CreateTime: '2026-09-02T00:00:00Z',
  UpdateTime: '2026-09-02T00:00:00Z',
}

vi.mock('../../api', () => ({
  getAsset: async () => imageAsset,
  getAdminAssetReplicaDetails: vi.fn(),
  syncAdminAssetReplicas: vi.fn(),
}))

describe('AssetPreviewDialog asset URI', () => {
  beforeEach(() => {
    copyToClipboard.mockClear()
    useAuthStore.getState().auth.setUser(null)
  })

  test('copies the asset URI when the identifier is clicked', async () => {
    const user = userEvent.setup()
    const queryClient = new QueryClient({
      defaultOptions: { queries: { retry: false } },
    })

    render(
      <QueryClientProvider client={queryClient}>
        <AssetPreviewDialog open onOpenChange={vi.fn()} asset={imageAsset} />
      </QueryClientProvider>
    )

    await user.click(
      screen.getByRole('button', {
        name: 'Copy to clipboard: asset://asset-na-preview',
      })
    )

    expect(copyToClipboard).toHaveBeenCalledWith('asset://asset-na-preview')
    await waitFor(() =>
      expect(screen.getByRole('button', { name: 'Copied' })).toBeVisible()
    )
  })
})
