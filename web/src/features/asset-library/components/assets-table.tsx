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
import { useQuery } from '@tanstack/react-query'
import { getRouteApi } from '@tanstack/react-router'
import { useMemo } from 'react'
import { useTranslation } from 'react-i18next'

import { DataTablePage, useDataTable } from '@/components/data-table'
import { useMediaQuery } from '@/hooks'
import { useTableUrlState } from '@/hooks/use-table-url-state'

import { listAllAssetGroups, listAssets } from '../api'
import { assetLibraryQueryKeys } from '../lib'
import type { AssetGroup, AssetType, ListAssetLibraryRequest } from '../types'
import { AssetCard } from './asset-card'
import { useAssetColumns } from './asset-columns'

const route = getRouteApi('/_authenticated/asset-library/')

export function AssetsTable() {
  const { t } = useTranslation()
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
