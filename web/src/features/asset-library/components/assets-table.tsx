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
import {
  useQueries,
  useQuery,
  useQueryClient,
  type Query,
} from '@tanstack/react-query'
import { getRouteApi } from '@tanstack/react-router'
import { useEffect, useMemo, useRef } from 'react'
import { useTranslation } from 'react-i18next'

import { DataTablePage, useDataTable } from '@/components/data-table'
import { useMediaQuery } from '@/hooks'
import { useTableUrlState } from '@/hooks/use-table-url-state'

import { getAsset, listAllAssetGroups, listAssets } from '../api'
import {
  assetLibraryQueryKeys,
  isAssetProcessing,
  updateAssetStatusCaches,
} from '../lib'
import type {
  Asset,
  AssetGroup,
  AssetType,
  ListAssetLibraryRequest,
} from '../types'
import { AssetCard } from './asset-card'
import { useAssetColumns } from './asset-columns'

const route = getRouteApi('/_authenticated/asset-library/')
const ASSET_STATUS_REFRESH_INTERVAL_MS = 4_000
const ASSET_STATUS_ERROR_RETRY_INTERVAL_MS = 15_000
const ASSET_STATUS_FAST_REFRESH_LIMIT = 15
const ASSET_STATUS_MAX_REFRESH_COUNT = 30
const ASSET_STATUS_MAX_ERROR_COUNT = 5

type AssetStatusRefreshBudget = {
  successfulRefreshes: number
  consecutiveFailures: number
  exhausted: boolean
}

export function AssetsTable() {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const statusRefreshBudgets = useRef(
    new Map<string, AssetStatusRefreshBudget>()
  )
  const isMobile = useMediaQuery('(max-width: 640px)')
  const search = route.useSearch()
  const navigate = route.useNavigate()
  const {
    globalFilter,
    onGlobalFilterChange,
    columnFilters,
    onColumnFiltersChange,
    pagination,
    onPaginationChange,
    ensurePageInRange,
  } = useTableUrlState({
    search,
    navigate,
    pagination: { defaultPage: 1, defaultPageSize: isMobile ? 10 : 20 },
    globalFilter: { enabled: true, key: 'filter' },
    columnFilters: [
      { columnId: 'assetType', searchKey: 'assetType', type: 'array' },
      { columnId: 'groupId', searchKey: 'groupId', type: 'array' },
    ],
  })

  const assetType = ((columnFilters.find((filter) => filter.id === 'assetType')
    ?.value as string[] | undefined) ?? [])[0] as AssetType | undefined
  const groupId = ((columnFilters.find((filter) => filter.id === 'groupId')
    ?.value as string[] | undefined) ?? [])[0]

  const { data: groups = [] } = useQuery({
    queryKey: assetLibraryQueryKeys.groupOptions(),
    queryFn: listAllAssetGroups,
  })
  const groupsById = useMemo(
    () => new Map(groups.map((group) => [group.Id, group] as const)),
    [groups]
  )

  const request = useMemo<ListAssetLibraryRequest>(
    () => ({
      Filter: {
        Name: globalFilter || undefined,
        AssetType: assetType,
        GroupIds: groupId ? [groupId] : undefined,
      },
      PageNumber: pagination.pageIndex + 1,
      PageSize: pagination.pageSize,
      SortBy: 'CreateTime',
      SortOrder: 'Desc',
    }),
    [
      assetType,
      globalFilter,
      groupId,
      pagination.pageIndex,
      pagination.pageSize,
    ]
  )

  const { data, isLoading, isFetching } = useQuery({
    queryKey: assetLibraryQueryKeys.assetList(request),
    queryFn: () => listAssets(request),
    placeholderData: (previousData) => previousData,
  })
  useEffect(() => {
    const visibleProcessingAssetIds = new Set(
      data?.Items.filter(isAssetProcessing).map((asset) => asset.Id) ?? []
    )
    for (const assetId of statusRefreshBudgets.current.keys()) {
      if (!visibleProcessingAssetIds.has(assetId)) {
        statusRefreshBudgets.current.delete(assetId)
      }
    }
  }, [data?.Items])
  useQueries({
    queries: (data?.Items ?? []).filter(isAssetProcessing).map((asset) => {
      const budget = statusRefreshBudgets.current.get(asset.Id)
      return {
        queryKey: assetLibraryQueryKeys.asset(asset.Id),
        queryFn: async () => {
          const currentBudget = statusRefreshBudgets.current.get(asset.Id) ?? {
            successfulRefreshes: 0,
            consecutiveFailures: 0,
            exhausted: false,
          }
          try {
            const refreshedAsset = await getAsset(asset.Id)
            if (isAssetProcessing(refreshedAsset)) {
              currentBudget.successfulRefreshes += 1
              currentBudget.consecutiveFailures = 0
              currentBudget.exhausted =
                currentBudget.successfulRefreshes >=
                ASSET_STATUS_MAX_REFRESH_COUNT
              statusRefreshBudgets.current.set(asset.Id, currentBudget)
            } else {
              statusRefreshBudgets.current.delete(asset.Id)
            }
            updateAssetStatusCaches(queryClient, refreshedAsset)
            return refreshedAsset
          } catch (error) {
            currentBudget.consecutiveFailures += 1
            currentBudget.exhausted =
              currentBudget.consecutiveFailures >= ASSET_STATUS_MAX_ERROR_COUNT
            statusRefreshBudgets.current.set(asset.Id, currentBudget)
            throw error
          }
        },
        enabled: !budget?.exhausted,
        placeholderData: asset,
        refetchInterval: (query: Query<Asset>) => {
          const currentBudget = statusRefreshBudgets.current.get(asset.Id)
          if (currentBudget?.exhausted) return false
          const currentAsset = query.state.data
          if (currentAsset && !isAssetProcessing(currentAsset)) return false
          return (currentBudget?.consecutiveFailures ?? 0) > 0 ||
            (currentBudget?.successfulRefreshes ?? 0) >=
              ASSET_STATUS_FAST_REFRESH_LIMIT
            ? ASSET_STATUS_ERROR_RETRY_INTERVAL_MS
            : ASSET_STATUS_REFRESH_INTERVAL_MS
        },
        refetchIntervalInBackground: false,
        refetchOnWindowFocus: () =>
          !statusRefreshBudgets.current.get(asset.Id)?.exhausted,
        retry: false,
        staleTime: 0,
      }
    }),
  })
  const includeReplication = Boolean(
    data?.Items.some((asset) => asset.Replication)
  )
  const columns = useAssetColumns(groupsById, includeReplication)
  const { table } = useDataTable({
    data: data?.Items ?? [],
    columns,
    totalCount: data?.TotalCount ?? 0,
    columnFilters,
    pagination,
    globalFilter,
    onColumnFiltersChange,
    onPaginationChange,
    onGlobalFilterChange,
    getRowId: (asset) => asset.Id,
    manualPagination: true,
    manualFiltering: true,
    ensurePageInRange,
  })

  const groupOptions = groups.map((group: AssetGroup) => ({
    label: group.Name,
    value: group.Id,
  }))

  return (
    <DataTablePage
      table={table}
      columns={columns}
      isLoading={isLoading}
      isFetching={isFetching}
      emptyTitle={t('No assets found')}
      emptyDescription={t(
        'Add a public image, video, or audio URL to start your asset library.'
      )}
      enableCardView
      viewModeStorageKey='asset-library:assets:view-mode'
      renderCard={(row) => (
        <AssetCard row={row} group={groupsById.get(row.original.GroupId)} />
      )}
      cardGridClassName='grid gap-2.5 [grid-template-columns:repeat(auto-fill,minmax(min(100%,15rem),1fr))] sm:gap-3'
      toolbarProps={{
        searchPlaceholder: t('Filter assets by name...'),
        searchDebounceMs: 400,
        filters: [
          {
            columnId: 'assetType',
            title: t('Type'),
            singleSelect: true,
            options: [
              { label: t('Image'), value: 'Image' },
              { label: t('Video'), value: 'Video' },
              { label: t('Audio'), value: 'Audio' },
            ],
          },
          {
            columnId: 'groupId',
            title: t('Asset Group'),
            singleSelect: true,
            options: groupOptions,
          },
        ],
      }}
      applyHeaderSize
      skeletonKeyPrefix='asset-library-asset'
    />
  )
}
