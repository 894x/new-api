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
  useIsMutating,
  useMutation,
  useQueryClient,
} from '@tanstack/react-query'
import type { Row } from '@tanstack/react-table'
import {
  Eye,
  Loader2,
  MoreHorizontal,
  Pencil,
  RefreshCw,
  Trash2,
} from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { Button } from '@/components/ui/button'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuShortcut,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'

import {
  assetLibraryQueryKeys,
  getAssetLibraryErrorMessage,
  refreshAssetStatus,
} from '../lib'
import type { Asset } from '../types'
import { useAssetLibrary } from './asset-library-provider'

export function AssetRowActions(props: { row: Row<Asset> }) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const { openAssetDialog, isReadOnly } = useAssetLibrary()
  const asset = props.row.original
  const isLibraryRefreshing =
    useIsMutating({
      mutationKey: assetLibraryQueryKeys.statusRefreshLibrary(),
    }) > 0
  const isAssetRefreshing =
    useIsMutating({
      mutationKey: assetLibraryQueryKeys.statusRefreshAsset(asset.Id),
    }) > 0
  const refreshMutation = useMutation({
    mutationKey: assetLibraryQueryKeys.statusRefreshAsset(asset.Id),
    mutationFn: () => refreshAssetStatus(queryClient, asset.Id),
    onSuccess: () => toast.success(t('Asset status refreshed.')),
    onError: (error) =>
      toast.error(
        getAssetLibraryErrorMessage(error, t('Failed to refresh asset'))
      ),
  })

  return (
    <DropdownMenu>
      <DropdownMenuTrigger
        render={
          <Button
            variant='ghost'
            size='icon-sm'
            aria-label={t('Open asset actions')}
          />
        }
      >
        <MoreHorizontal className='size-4' />
      </DropdownMenuTrigger>
      <DropdownMenuContent align='end' className='w-40'>
        {!isReadOnly ? (
          <DropdownMenuItem
            disabled={isAssetRefreshing || isLibraryRefreshing}
            onClick={() => refreshMutation.mutate()}
          >
            {t('Refresh asset')}
            <DropdownMenuShortcut>
              {isAssetRefreshing ? (
                <Loader2 className='size-4 animate-spin' />
              ) : (
                <RefreshCw className='size-4' />
              )}
            </DropdownMenuShortcut>
          </DropdownMenuItem>
        ) : null}
        <DropdownMenuItem
          onClick={() => openAssetDialog('preview-asset', asset)}
        >
          {t('Preview')}
          <DropdownMenuShortcut>
            <Eye className='size-4' />
          </DropdownMenuShortcut>
        </DropdownMenuItem>
        {!isReadOnly ? (
          <>
            <DropdownMenuItem
              onClick={() => openAssetDialog('update-asset', asset)}
            >
              {t('Edit')}
              <DropdownMenuShortcut>
                <Pencil className='size-4' />
              </DropdownMenuShortcut>
            </DropdownMenuItem>
            <DropdownMenuItem
              className='text-destructive focus:text-destructive'
              onClick={() => openAssetDialog('delete-asset', asset)}
            >
              {t('Delete')}
              <DropdownMenuShortcut>
                <Trash2 className='size-4' />
              </DropdownMenuShortcut>
            </DropdownMenuItem>
          </>
        ) : null}
      </DropdownMenuContent>
    </DropdownMenu>
  )
}
