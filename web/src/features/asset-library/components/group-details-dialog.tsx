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
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Loader2, RefreshCw } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { Dialog } from '@/components/dialog'
import { StatusBadge } from '@/components/status-badge'
import { Button } from '@/components/ui/button'
import { Skeleton } from '@/components/ui/skeleton'
import {
  ADMIN_PERMISSION_ACTIONS,
  ADMIN_PERMISSION_RESOURCES,
  hasPermission,
} from '@/lib/admin-permissions'
import { useAuthStore } from '@/stores/auth-store'

import {
  getAdminAssetGroupReplicaDetails,
  syncAdminAssetGroupReplicas,
} from '../api'
import { assetLibraryQueryKeys, getAssetLibraryErrorMessage } from '../lib'
import type { AssetGroup } from '../types'
import { AdminReplicaDetails } from './admin-replica-details'
import { ReplicationBadge } from './replication-badge'

export function GroupDetailsDialog(props: {
  open: boolean
  onOpenChange: (open: boolean) => void
  group?: AssetGroup | null
}) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const user = useAuthStore((state) => state.auth.user)
  const canView = hasPermission(
    user,
    ADMIN_PERMISSION_RESOURCES.CHANNEL,
    ADMIN_PERMISSION_ACTIONS.READ
  )
  const canSync = hasPermission(
    user,
    ADMIN_PERMISSION_RESOURCES.CHANNEL,
    ADMIN_PERMISSION_ACTIONS.SENSITIVE_WRITE
  )
  const groupId = props.group?.Id || ''
  const replicaQuery = useQuery({
    queryKey: assetLibraryQueryKeys.groupReplicas(groupId),
    queryFn: () => getAdminAssetGroupReplicaDetails(groupId),
    enabled: props.open && !!groupId && canView,
    staleTime: 0,
    gcTime: 0,
  })
  const syncMutation = useMutation({
    mutationFn: (channelId?: number) =>
      syncAdminAssetGroupReplicas(groupId, channelId ? [channelId] : []),
    onSuccess: async (report) => {
      await Promise.all([
        queryClient.invalidateQueries({
          queryKey: assetLibraryQueryKeys.groupReplicas(groupId),
        }),
        queryClient.invalidateQueries({
          queryKey: assetLibraryQueryKeys.groups(),
        }),
        queryClient.invalidateQueries({
          queryKey: assetLibraryQueryKeys.assets(),
        }),
      ])
      if (report.Errors.length > 0) {
        toast.warning(
          t(
            'Synchronization completed with {{count}} synchronization errors.',
            {
              count: report.Errors.length,
            }
          ),
          {
            description: report.Errors.slice(0, 3)
              .map(
                (error) =>
                  `${error.asset_id ? `#${error.channel_id} / ${error.asset_id}` : `#${error.channel_id}`}: ${error.message}`
              )
              .join('; '),
          }
        )
      } else {
        toast.success(t('Asset group synchronization completed.'))
      }
    },
    onError: (error) =>
      toast.error(
        getAssetLibraryErrorMessage(
          error,
          t('Failed to synchronize asset group.')
        )
      ),
  })

  let details = null
  if (!canView) {
    details = (
      <p className='text-muted-foreground rounded-lg border p-6 text-center text-sm'>
        {t(
          'You can synchronize this asset group, but you do not have permission to view upstream details.'
        )}
      </p>
    )
  } else if (replicaQuery.isLoading) {
    details = <Skeleton className='h-56 w-full rounded-lg' />
  } else if (replicaQuery.isError) {
    details = (
      <div className='flex min-h-56 flex-col items-center justify-center gap-3 rounded-lg border'>
        <p className='text-muted-foreground text-sm'>
          {t('Failed to load upstream synchronization details.')}
        </p>
        <Button
          variant='outline'
          onClick={() => replicaQuery.refetch()}
          disabled={replicaQuery.isFetching}
        >
          {replicaQuery.isFetching && <Loader2 className='animate-spin' />}
          {t('Retry')}
        </Button>
      </div>
    )
  } else if (replicaQuery.data) {
    details = (
      <div className='flex flex-col gap-3'>
        <div className='flex flex-wrap items-center gap-2'>
          <ReplicationBadge replication={replicaQuery.data.summary} />
        </div>
        {replicaQuery.data.replicas.length > 0 ? (
          <AdminReplicaDetails
            replicas={replicaQuery.data.replicas}
            resource='group'
            canSync={canSync}
            syncingChannelId={
              syncMutation.isPending
                ? (syncMutation.variables ?? 'all')
                : undefined
            }
            onSync={(channelId) => syncMutation.mutate(channelId)}
          />
        ) : (
          <p className='text-muted-foreground rounded-lg border p-6 text-center text-sm'>
            {t('No upstream channels are configured for the asset library.')}
          </p>
        )}
      </div>
    )
  }

  return (
    <Dialog
      open={props.open}
      onOpenChange={props.onOpenChange}
      title={props.group?.Name || t('Asset group details')}
      description={props.group ? `group://${props.group.Id}` : undefined}
      contentClassName='sm:max-w-6xl'
      contentHeight='auto'
    >
      <div className='flex flex-col gap-4'>
        {props.group ? (
          <div className='flex flex-wrap items-center justify-between gap-2'>
            <div className='flex flex-wrap items-center gap-2'>
              <StatusBadge
                label={props.group.GroupType}
                variant='info'
                copyable={false}
              />
              <span className='text-muted-foreground text-sm'>
                {props.group.Description || t('No description')}
              </span>
            </div>
            {canSync ? (
              <Button
                variant='outline'
                size='sm'
                disabled={
                  syncMutation.isPending ||
                  (canView &&
                    !replicaQuery.data?.replicas.some(
                      (replica) => replica.enabled,
                    ))
                }
                onClick={() => syncMutation.mutate(undefined)}
              >
                {syncMutation.isPending &&
                syncMutation.variables === undefined ? (
                  <Loader2 className='animate-spin' />
                ) : (
                  <RefreshCw />
                )}
                {t('Sync group and assets')}
              </Button>
            ) : null}
          </div>
        ) : null}
        {details}
      </div>
    </Dialog>
  )
}
