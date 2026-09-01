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
import { FileAudio, FileImage, FileVideo } from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'
import { cn } from '@/lib/utils'

import type { Asset } from '../types'
import { useAssetLibrary } from './asset-library-provider'

export function AssetThumbnail(props: {
  asset: Asset
  className?: string
  buttonClassName?: string
}) {
  const { t } = useTranslation()
  const { openAssetDialog } = useAssetLibrary()
  const iconClassName = 'size-6 text-muted-foreground'
  if (props.asset.AssetType === 'Image' && props.asset.URL) {
    return (
      <Button
        type='button'
        variant='ghost'
        className={cn('h-auto cursor-zoom-in p-0', props.buttonClassName)}
        aria-label={`${t('Preview')}: ${props.asset.Name || t('Untitled asset')}`}
        title={t('Preview')}
        onClick={() => openAssetDialog('preview-asset', props.asset)}
      >
        <img
          src={props.asset.URL}
          alt={props.asset.Name || t('Asset preview')}
          loading='lazy'
          className={cn(
            'size-12 rounded-md border object-cover transition-opacity group-hover/button:opacity-90',
            props.className
          )}
        />
      </Button>
    )
  }

  let icon = <FileImage className={iconClassName} aria-hidden='true' />
  if (props.asset.AssetType === 'Video') {
    icon = <FileVideo className={iconClassName} aria-hidden='true' />
  } else if (props.asset.AssetType === 'Audio') {
    icon = <FileAudio className={iconClassName} aria-hidden='true' />
  }

  return (
    <span
      className={cn(
        'bg-muted/50 flex size-12 shrink-0 items-center justify-center rounded-md border',
        props.className
      )}
      aria-label={t('{{type}} asset', { type: t(props.asset.AssetType) })}
    >
      {icon}
    </span>
  )
}
