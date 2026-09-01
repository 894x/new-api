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
import type { QueryClient } from '@tanstack/react-query'

import { getAsset, listAllAssets } from '../api'
import type { Asset, AssetsPage } from '../types'
import { assetLibraryQueryKeys } from './query-keys'

const ASSET_LIBRARY_REFRESH_CONCURRENCY = 4

export type AssetLibraryRefreshResult = {
  succeeded: number
  failed: number
}

export function isAssetProcessing(asset: Asset): boolean {
  return (
    asset.Status?.trim().toLowerCase() === 'processing' ||
    (asset.Replication?.Processing ?? 0) > 0
  )
}

export function updateAssetStatusCaches(
  queryClient: QueryClient,
  refreshedAsset: Asset
): void {
  queryClient.setQueryData(
    assetLibraryQueryKeys.asset(refreshedAsset.Id),
    refreshedAsset
  )
  queryClient.setQueriesData<AssetsPage>(
    { queryKey: assetLibraryQueryKeys.assetLists() },
    (page) => {
      if (!page?.Items.some((item) => item.Id === refreshedAsset.Id)) {
        return page
      }
      return {
        ...page,
        Items: page.Items.map((item) =>
          item.Id === refreshedAsset.Id ? refreshedAsset : item
        ),
      }
    }
  )
}

export async function refreshAssetStatus(
  queryClient: QueryClient,
  assetId: string
): Promise<Asset> {
  const refreshedAsset = await queryClient.fetchQuery({
    queryKey: assetLibraryQueryKeys.asset(assetId),
    queryFn: () => getAsset(assetId),
    staleTime: 0,
  })
  updateAssetStatusCaches(queryClient, refreshedAsset)
  return refreshedAsset
}

export async function refreshAssetLibraryStatuses(
  queryClient: QueryClient
): Promise<AssetLibraryRefreshResult> {
  const assets = await listAllAssets()
  let nextAssetIndex = 0
  let succeeded = 0
  let failed = 0

  const workerCount = Math.min(ASSET_LIBRARY_REFRESH_CONCURRENCY, assets.length)
  await Promise.all(
    Array.from({ length: workerCount }, async () => {
      while (nextAssetIndex < assets.length) {
        const asset = assets[nextAssetIndex]
        nextAssetIndex += 1
        try {
          await refreshAssetStatus(queryClient, asset.Id)
          succeeded += 1
        } catch {
          failed += 1
        }
      }
    })
  )

  await queryClient.invalidateQueries({
    queryKey: assetLibraryQueryKeys.assetLists(),
  })
  return { succeeded, failed }
}
