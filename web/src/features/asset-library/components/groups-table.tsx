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

import { listAssetGroups } from '../api'
import { assetLibraryQueryKeys } from '../lib'
import type { ListAssetLibraryRequest } from '../types'
import { useAssetLibrary } from './asset-library-provider'
import { GroupCard } from './group-card'
import { useAssetGroupColumns } from './group-columns'

const route = getRouteApi('/_authenticated/asset-library/')

export function GroupsTable() {
  const { t } = useTranslation()
  const isMobile = useMediaQuery('(max-width: 640px)')
  const search = route.useSearch()
  const navigate = route.useNavigate()
  const { targetUserId } = useAssetLibrary()
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
  })

  const request = useMemo<ListAssetLibraryRequest>(
    () => ({
      Filter: { Name: globalFilter || undefined },
      PageNumber: pagination.pageIndex + 1,
      PageSize: pagination.pageSize,
      SortBy: 'CreateTime',
      SortOrder: 'Desc',
    }),
    [globalFilter, pagination.pageIndex, pagination.pageSize]
  )
  const { data, isLoading, isFetching } = useQuery({
    queryKey: assetLibraryQueryKeys.groupList(request, targetUserId),
    queryFn: () => listAssetGroups(request, targetUserId),
    placeholderData: (previousData, previousQuery) =>
      previousQuery?.queryKey[2] === (targetUserId ?? 'self')
        ? previousData
        : undefined,
  })
  const includeReplication = Boolean(
    data?.Items.some((group) => group.Replication)
  )
  const columns = useAssetGroupColumns(includeReplication)
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
    getRowId: (group) => group.Id,
    manualPagination: true,
    manualFiltering: true,
    ensurePageInRange,
  })

  return (
    <DataTablePage
      table={table}
      columns={columns}
      isLoading={isLoading}
      isFetching={isFetching}
      emptyTitle={t('No asset groups found')}
      emptyDescription={t(
        'Create an asset group before adding images, videos, or audio.'
      )}
      enableCardView
      viewModeStorageKey='asset-library:groups:view-mode'
      renderCard={(row) => <GroupCard row={row} />}
      cardGridClassName='grid gap-2.5 [grid-template-columns:repeat(auto-fill,minmax(min(100%,15rem),1fr))] sm:gap-3'
      toolbarProps={{
        searchPlaceholder: t('Filter asset groups by name...'),
        searchDebounceMs: 400,
      }}
      applyHeaderSize
      skeletonKeyPrefix='asset-library-group'
    />
  )
}
