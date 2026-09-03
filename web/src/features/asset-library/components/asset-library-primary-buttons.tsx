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
import { FolderPlus, Loader2, Plus, RefreshCw } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { Button } from '@/components/ui/button'

import {
  assetLibraryQueryKeys,
  getAssetLibraryErrorMessage,
  refreshAssetLibraryStatuses,
} from '../lib'
import { useAssetLibrary } from './asset-library-provider'

export function AssetLibraryPrimaryButtons() {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const { openAssetDialog, openGroupDialog, isReadOnly } = useAssetLibrary()
  const isAnyStatusRefreshing =
    useIsMutating({
      mutationKey: assetLibraryQueryKeys.statusRefreshes(),
    }) > 0
  const refreshMutation = useMutation({
    mutationKey: assetLibraryQueryKeys.statusRefreshLibrary(),
    mutationFn: () => refreshAssetLibraryStatuses(queryClient),
    onSuccess: (result) => {
      if (result.failed === 0) {
        toast.success(t('Asset library refreshed.'))
        return
      }
      toast.warning(
        t(
          'Asset library refresh completed: {{succeeded}} succeeded, {{failed}} failed.',
          result
        )
      )
    },
    onError: (error) =>
      toast.error(
        getAssetLibraryErrorMessage(error, t('Failed to refresh asset library'))
      ),
  })

  if (isReadOnly) return null

  return (
    <div className='flex flex-wrap gap-2'>
      <Button
        size='sm'
        variant='outline'
        disabled={refreshMutation.isPending || isAnyStatusRefreshing}
        onClick={() => refreshMutation.mutate()}
      >
        {refreshMutation.isPending ? (
          <Loader2 className='animate-spin' />
        ) : (
          <RefreshCw />
        )}
        {t('Refresh asset library')}
      </Button>
      <Button
        size='sm'
        variant='outline'
        onClick={() => openGroupDialog('create-group')}
      >
        <FolderPlus />
        {t('Create asset group')}
      </Button>
      <Button size='sm' onClick={() => openAssetDialog('create-asset')}>
        <Plus />
        {t('Add asset')}
      </Button>
    </div>
  )
}
