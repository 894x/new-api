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
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { ConfirmDialog } from '@/components/confirm-dialog'

import { deleteAsset, deleteAssetGroup } from '../api'
import { assetLibraryQueryKeys, getAssetLibraryErrorMessage } from '../lib'
import type { Asset, AssetGroup } from '../types'

export function AssetDeleteDialog(props: {
  open: boolean
  onOpenChange: (open: boolean) => void
  asset?: Asset | null
}) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const mutation = useMutation({
    mutationFn: () => deleteAsset(props.asset?.Id || ''),
    onSuccess: async () => {
      await queryClient.invalidateQueries({
        queryKey: assetLibraryQueryKeys.assets(),
      })
      toast.success(t('Asset deleted'))
      props.onOpenChange(false)
    },
    onError: (error) =>
      toast.error(
        getAssetLibraryErrorMessage(error, t('Failed to delete asset'))
      ),
  })

  return (
    <ConfirmDialog
      open={props.open}
      onOpenChange={props.onOpenChange}
      title={t('Delete asset?')}
      desc={t(
        'The asset "{{name}}" will be removed from every synchronized channel. This action cannot be undone.',
        { name: props.asset?.Name || props.asset?.Id || '' }
      )}
      confirmText={t('Delete')}
      destructive
      disabled={!props.asset}
      isLoading={mutation.isPending}
      handleConfirm={() => mutation.mutate()}
    />
  )
}

export function GroupDeleteDialog(props: {
  open: boolean
  onOpenChange: (open: boolean) => void
  group?: AssetGroup | null
}) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const mutation = useMutation({
    mutationFn: () => deleteAssetGroup(props.group?.Id || ''),
    onSuccess: async () => {
      await queryClient.invalidateQueries({
        queryKey: assetLibraryQueryKeys.all,
      })
      toast.success(t('Asset group deleted'))
      props.onOpenChange(false)
    },
    onError: (error) =>
      toast.error(
        getAssetLibraryErrorMessage(error, t('Failed to delete asset group'))
      ),
  })

  return (
    <ConfirmDialog
      open={props.open}
      onOpenChange={props.onOpenChange}
      title={t('Delete asset group?')}
      desc={t(
        'The asset group "{{name}}" will be removed from every synchronized channel. Delete its assets first. This action cannot be undone.',
        { name: props.group?.Name || '' }
      )}
      confirmText={t('Delete')}
      destructive
      disabled={!props.group}
      isLoading={mutation.isPending}
      handleConfirm={() => mutation.mutate()}
    />
  )
}
