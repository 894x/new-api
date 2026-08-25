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
import { ExternalLink, Loader2, RefreshCw } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { Dialog } from '@/components/dialog'
import { StatusBadge } from '@/components/status-badge'
import { Button } from '@/components/ui/button'
import { Skeleton } from '@/components/ui/skeleton'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import {
  ADMIN_PERMISSION_ACTIONS,
  ADMIN_PERMISSION_RESOURCES,
  hasPermission,
} from '@/lib/admin-permissions'
import { useAuthStore } from '@/stores/auth-store'

import {
  getAdminAssetReplicaDetails,
  getAsset,
  syncAdminAssetReplicas,
} from '../api'
import { assetLibraryQueryKeys, getAssetLibraryErrorMessage } from '../lib'
import type { Asset } from '../types'
import { AdminReplicaDetails } from './admin-replica-details'
import { ReplicationBadge } from './replication-badge'

function AssetMediaPreview(props: { asset: Asset }) {
  const { t } = useTranslation()
  const { asset } = props
  if (!asset.URL) {
    return (
      <div className='bg-muted/30 flex min-h-56 items-center justify-center rounded-lg border'>
        <p className='text-muted-foreground px-6 text-center text-sm'>
          {t(
            'A preview URL is not available yet. Try again after processing finishes.'
          )}
        </p>
      </div>
    )
  }

  if (asset.AssetType === 'Image') {
    return (
      <div className='bg-muted/30 flex min-h-56 items-center justify-center rounded-lg border p-2'>
        <img
          src={asset.URL}
          alt={asset.Name || t('Asset preview')}
          className='max-h-[60vh] max-w-full rounded object-contain'
        />
      </div>
    )
  }
  if (asset.AssetType === 'Video') {
    return (
      <video
        src={asset.URL}
        controls
        playsInline
        preload='metadata'
        className='bg-muted/30 max-h-[60vh] w-full rounded-lg border'
      >
        {t('Your browser does not support video playback.')}
      </video>
    )
  }
  if (asset.AssetType === 'Audio') {
    return (
      <div className='bg-muted/30 rounded-lg border p-5'>
        <audio src={asset.URL} controls preload='metadata' className='w-full'>
          {t('Your browser does not support audio playback.')}
        </audio>
      </div>
    )
  }

  return null
}

export function AssetPreviewDialog(props: {
  open: boolean
  onOpenChange: (open: boolean) => void
  asset?: Asset | null
}) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const user = useAuthStore((state) => state.auth.user)
  const canViewReplicas = hasPermission(
    user,
    ADMIN_PERMISSION_RESOURCES.CHANNEL,
    ADMIN_PERMISSION_ACTIONS.READ
  )
  const canSyncReplicas = hasPermission(
    user,
    ADMIN_PERMISSION_RESOURCES.CHANNEL,
    ADMIN_PERMISSION_ACTIONS.SENSITIVE_WRITE
  )
  const assetId = props.asset?.Id || ''
  const assetQuery = useQuery({
    queryKey: assetLibraryQueryKeys.asset(assetId),
    queryFn: () => getAsset(assetId),
    enabled: props.open && !!assetId && !canViewReplicas,
    staleTime: 0,
    gcTime: 0,
  })
  const replicaQuery = useQuery({
    queryKey: assetLibraryQueryKeys.assetReplicas(assetId),
    queryFn: () => getAdminAssetReplicaDetails(assetId),
    enabled: props.open && !!assetId && canViewReplicas,
    staleTime: 0,
    gcTime: 0,
  })
  const syncMutation = useMutation({
    mutationFn: (channelId?: number) =>
      syncAdminAssetReplicas(assetId, channelId ? [channelId] : []),
    onSuccess: async (report) => {
      await Promise.all([
        queryClient.invalidateQueries({
          queryKey: assetLibraryQueryKeys.assetReplicas(assetId),
        }),
        queryClient.invalidateQueries({
          queryKey: assetLibraryQueryKeys.assets(),
        }),
      ])
      if (report.Errors.length > 0) {
        toast.warning(
          t('Synchronization completed with {{count}} channel errors.', {
            count: report.Errors.length,
          }),
          {
            description: report.Errors.slice(0, 3)
              .map((error) => `#${error.channel_id}: ${error.message}`)
              .join('; '),
          }
        )
      } else {
        toast.success(t('Asset synchronization completed.'))
      }
    },
    onError: (error) =>
      toast.error(
        getAssetLibraryErrorMessage(error, t('Failed to synchronize asset.'))
      ),
  })
  const asset = replicaQuery.data?.asset || assetQuery.data || props.asset
  let previewContent = null
  if (assetQuery.isLoading && !asset) {
    previewContent = <Skeleton className='h-72 w-full rounded-lg' />
  } else if (assetQuery.isError) {
    previewContent = (
      <div className='flex min-h-56 flex-col items-center justify-center gap-3 rounded-lg border'>
        <p className='text-muted-foreground text-sm'>
          {t('Failed to load asset preview.')}
        </p>
        <Button
          variant='outline'
          onClick={() => assetQuery.refetch()}
          disabled={assetQuery.isFetching}
        >
          {assetQuery.isFetching && <Loader2 className='animate-spin' />}
          {t('Retry')}
        </Button>
      </div>
    )
  } else if (asset) {
    previewContent = (
      <div className='space-y-4'>
        <AssetMediaPreview asset={asset} />
        <div className='flex flex-wrap items-center gap-2'>
          <StatusBadge
            label={t(asset.AssetType)}
            variant='info'
            copyable={false}
          />
          <StatusBadge
            label={t(asset.Status || 'Unknown')}
            variant={asset.Status === 'Active' ? 'success' : 'warning'}
            copyable={false}
          />
          <ReplicationBadge replication={asset.Replication} />
        </div>
        {asset.Error?.Message && (
          <p className='text-destructive border-destructive/30 rounded-lg border p-3 text-sm'>
            {[asset.Error.Code, asset.Error.Message].filter(Boolean).join(': ')}
          </p>
        )}
        {asset.URL && (
          <p className='bg-muted text-muted-foreground rounded-md p-3 font-mono text-xs break-all'>
            {asset.URL}
          </p>
        )}
      </div>
    )
  }

  let upstreamContent = null
  if (replicaQuery.isLoading) {
    upstreamContent = <Skeleton className='h-56 w-full rounded-lg' />
  } else if (replicaQuery.isError) {
    upstreamContent = (
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
    upstreamContent = (
      <div className='flex flex-col gap-3'>
        <div className='flex flex-wrap items-center justify-between gap-2'>
          <ReplicationBadge replication={replicaQuery.data.summary} />
          {canSyncReplicas ? (
            <Button
              variant='outline'
              size='sm'
              disabled={
                syncMutation.isPending ||
                !replicaQuery.data.replicas.some((replica) => replica.enabled)
              }
              onClick={() => syncMutation.mutate(undefined)}
            >
              {syncMutation.isPending &&
              syncMutation.variables === undefined ? (
                <Loader2 className='animate-spin' />
              ) : (
                <RefreshCw />
              )}
              {t('Sync all channels')}
            </Button>
          ) : null}
        </div>
        {replicaQuery.data.replicas.length > 0 ? (
          <AdminReplicaDetails
            replicas={replicaQuery.data.replicas}
            resource='asset'
            canSync={canSyncReplicas}
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

  const content = canViewReplicas ? (
    <Tabs defaultValue='preview'>
      <TabsList>
        <TabsTrigger value='preview'>{t('Preview')}</TabsTrigger>
        <TabsTrigger value='upstreams'>{t('Upstream details')}</TabsTrigger>
      </TabsList>
      <TabsContent value='preview'>{previewContent}</TabsContent>
      <TabsContent value='upstreams'>{upstreamContent}</TabsContent>
    </Tabs>
  ) : (
    previewContent
  )

  const previewButton = asset?.URL ? (
    <Button
      variant='outline'
      onClick={() => window.open(asset.URL, '_blank', 'noopener,noreferrer')}
    >
      <ExternalLink />
      {t('Preview')}
    </Button>
  ) : null
  const syncWithoutDetailsButton =
    canSyncReplicas && !canViewReplicas ? (
      <Button
        variant='outline'
        disabled={syncMutation.isPending}
        onClick={() => syncMutation.mutate(undefined)}
      >
        {syncMutation.isPending ? (
          <Loader2 className='animate-spin' />
        ) : (
          <RefreshCw />
        )}
        {t('Sync all channels')}
      </Button>
    ) : null

  return (
    <Dialog
      open={props.open}
      onOpenChange={props.onOpenChange}
      title={asset?.Name || t('Asset preview')}
      description={asset ? `asset://${asset.Id}` : undefined}
      contentClassName={canViewReplicas ? 'sm:max-w-6xl' : 'sm:max-w-3xl'}
      contentHeight='auto'
      footer={
        syncWithoutDetailsButton || previewButton ? (
          <>
            {syncWithoutDetailsButton}
            {previewButton}
          </>
        ) : undefined
      }
    >
      {content}
    </Dialog>
  )
}
