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
import { keepPreviousData, useQuery } from '@tanstack/react-query'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import { Alert, AlertDescription } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import {
  Empty,
  EmptyDescription,
  EmptyHeader,
  EmptyTitle,
} from '@/components/ui/empty'
import { Skeleton } from '@/components/ui/skeleton'
import {
  hasPermission,
  ADMIN_PERMISSION_RESOURCES,
  ADMIN_PERMISSION_ACTIONS,
} from '@/lib/admin-permissions'
import { ROLE } from '@/lib/roles'
import { useAuthStore } from '@/stores/auth-store'

import { getModelChannelMatrix, matrixQueryKeys } from '../api'
import type { MatrixCell, MatrixSearch } from '../types'
import { MatrixCellEditor } from './matrix-cell-editor'
import { MatrixFilters } from './matrix-filters'
import { MatrixPagination } from './matrix-pagination'
import { MatrixTable } from './matrix-table'

export function MatrixWorkspace(props: {
  search: MatrixSearch
  onSearchChange: (search: MatrixSearch) => void
}) {
  const { t } = useTranslation()
  const user = useAuthStore((state) => state.auth.user)
  const canRead =
    (user?.role ?? 0) >= ROLE.ADMIN &&
    hasPermission(
      user,
      ADMIN_PERMISSION_RESOURCES.CHANNEL,
      ADMIN_PERMISSION_ACTIONS.READ
    )
  const canWrite = hasPermission(
    user,
    ADMIN_PERMISSION_RESOURCES.CHANNEL,
    ADMIN_PERMISSION_ACTIONS.WRITE
  )
  const [selection, setSelection] = useState<{
    cell: MatrixCell
    trigger: HTMLElement
  } | null>(null)
  const query = useQuery({
    queryKey: matrixQueryKeys.list(props.search),
    queryFn: () => getModelChannelMatrix(props.search),
    enabled: canRead,
    staleTime: 0,
    refetchOnMount: 'always',
    placeholderData: keepPreviousData,
  })

  if (!canRead) {
    return (
      <Alert>
        <AlertDescription>
          {t("You don't have necessary permission")}
        </AlertDescription>
      </Alert>
    )
  }

  return (
    <div className='flex h-full min-h-0 min-w-0 flex-col gap-3'>
      <p className='text-muted-foreground shrink-0 text-sm'>
        {t('Compare and edit routing limits for each model and channel.')}
      </p>
      <MatrixFilters
        search={props.search}
        groups={query.data?.groups ?? []}
        onSearchChange={props.onSearchChange}
      />
      <div className='flex shrink-0 flex-wrap items-center justify-between gap-2'>
        <p className='text-muted-foreground max-w-4xl text-xs'>
          {t(
            'Limits are shared across users and groups for each channel and public model. Async tasks and Realtime are excluded.'
          )}
        </p>
        <Button
          type='button'
          variant='outline'
          size='sm'
          disabled={query.isFetching}
          onClick={() => void query.refetch()}
        >
          {t('Refresh')}
        </Button>
      </div>
      {query.isFetching && (
        <span className='sr-only' role='status'>
          {t('Loading...')}
        </span>
      )}
      {query.isPending && <Skeleton className='min-h-32 flex-1' />}
      {query.isError && (
        <Alert variant='destructive'>
          <AlertDescription className='flex flex-wrap items-center justify-between gap-2'>
            {t('Failed to load model-channel matrix.')}
            <Button
              type='button'
              variant='outline'
              disabled={query.isFetching}
              onClick={() => void query.refetch()}
            >
              {t('Retry')}
            </Button>
          </AlertDescription>
        </Alert>
      )}
      {!query.isError && query.data && (
        <>
          {query.data.models.length > 0 && query.data.channels.length > 0 ? (
            <MatrixTable
              data={query.data}
              busy={query.isFetching}
              onSelect={(cell, trigger) => setSelection({ cell, trigger })}
            />
          ) : (
            <Empty className='flex-1 border'>
              <EmptyHeader>
                <EmptyTitle>
                  {t('No matching model-channel configurations')}
                </EmptyTitle>
                <EmptyDescription>
                  {t('Change the filters or configure models in Channels.')}
                </EmptyDescription>
              </EmptyHeader>
            </Empty>
          )}
          <div className='flex shrink-0 flex-wrap items-center justify-between gap-3'>
            <MatrixPagination
              axis='models'
              page={query.data.model_page}
              pageSize={query.data.model_page_size}
              total={query.data.model_total}
              disabled={query.isFetching}
              onPageChange={(page) =>
                props.onSearchChange({ ...props.search, model_page: page })
              }
            />
            <MatrixPagination
              axis='channels'
              page={query.data.channel_page}
              pageSize={query.data.channel_page_size}
              total={query.data.channel_total}
              disabled={query.isFetching}
              onPageChange={(page) =>
                props.onSearchChange({ ...props.search, channel_page: page })
              }
            />
          </div>
        </>
      )}
      {selection && (
        <MatrixCellEditor
          cell={selection.cell}
          canWrite={canWrite}
          trigger={selection.trigger}
          onClose={() => setSelection(null)}
        />
      )}
    </div>
  )
}
