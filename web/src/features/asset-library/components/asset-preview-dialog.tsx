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
import { ExternalLink, Loader2 } from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { Dialog } from '@/components/dialog'
import { StatusBadge } from '@/components/status-badge'
import { Button } from '@/components/ui/button'
import { Skeleton } from '@/components/ui/skeleton'

import { getAsset } from '../api'
import { assetLibraryQueryKeys } from '../lib'
import type { Asset } from '../types'
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
  const assetId = props.asset?.Id || ''
  const { data, isLoading, isError, refetch, isFetching } = useQuery({
    queryKey: assetLibraryQueryKeys.asset(assetId),
    queryFn: () => getAsset(assetId),
    enabled: props.open && !!assetId,
    staleTime: 0,
    gcTime: 0,
  })
  const asset = data || props.asset
  let content = null
  if (isLoading && !asset) {
    content = <Skeleton className='h-72 w-full rounded-lg' />
  } else if (isError) {
    content = (
      <div className='flex min-h-56 flex-col items-center justify-center gap-3 rounded-lg border'>
        <p className='text-muted-foreground text-sm'>
          {t('Failed to load asset preview.')}
        </p>
        <Button
          variant='outline'
          onClick={() => refetch()}
          disabled={isFetching}
        >
          {isFetching && <Loader2 className='animate-spin' />}
          {t('Retry')}
        </Button>
      </div>
    )
  } else if (asset) {
    content = (
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

  return (
    <Dialog
      open={props.open}
      onOpenChange={props.onOpenChange}
      title={asset?.Name || t('Asset preview')}
      description={asset ? `asset://${asset.Id}` : undefined}
      contentClassName='sm:max-w-3xl'
      contentHeight='auto'
      footer={
        asset?.URL ? (
          <Button
            variant='outline'
            onClick={() =>
              window.open(asset.URL, '_blank', 'noopener,noreferrer')
            }
          >
            <ExternalLink />
            {t('Preview')}
          </Button>
        ) : undefined
      }
    >
      {content}
    </Dialog>
  )
}
