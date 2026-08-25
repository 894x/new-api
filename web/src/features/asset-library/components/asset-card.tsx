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
import type { Row } from '@tanstack/react-table'
import { memo } from 'react'
import { useTranslation } from 'react-i18next'

import { StatusBadge } from '@/components/status-badge'
import { TableId } from '@/components/table-id'

import type { Asset, AssetGroup } from '../types'
import { getAssetStatusVariant } from './asset-columns'
import { AssetRowActions } from './asset-row-actions'
import { AssetThumbnail } from './asset-thumbnail'
import { ReplicationBadge } from './replication-badge'

function AssetCardComponent(props: { row: Row<Asset>; group?: AssetGroup }) {
  const { t } = useTranslation()
  const asset = props.row.original

  if (!asset.Replication) {
    return (
      <div className='flex min-w-0 flex-col gap-3'>
        <AssetThumbnail
          asset={asset}
          className='aspect-video h-auto w-full rounded-lg [&_svg]:size-12'
        />

        <div className='flex min-w-0 items-start gap-2'>
          <div className='min-w-0 flex-1'>
            <p
              className='truncate font-medium'
              title={asset.Name || t('Untitled asset')}
            >
              {asset.Name || t('Untitled asset')}
            </p>
            <p
              className='text-muted-foreground truncate text-xs'
              title={props.group?.Name || asset.GroupId}
            >
              {props.group?.Name || asset.GroupId}
            </p>
          </div>
          <AssetRowActions row={props.row} />
        </div>

        <div className='flex flex-wrap gap-1'>
          <StatusBadge
            label={t(asset.AssetType)}
            variant='info'
            copyable={false}
          />
          <StatusBadge
            label={t(asset.Status || 'Unknown')}
            variant={getAssetStatusVariant(asset.Status)}
            copyable={false}
          />
        </div>
      </div>
    )
  }

  return (
    <div className='flex min-w-0 flex-col gap-3'>
      <div className='flex items-start gap-3'>
        <AssetThumbnail asset={asset} className='size-16' />
        <div className='min-w-0 flex-1'>
          <p className='truncate font-medium'>
            {asset.Name || t('Untitled asset')}
          </p>
          <TableId
            value={asset.Id}
            className='block max-w-full truncate text-xs'
          />
          <div className='mt-2 flex flex-wrap gap-1'>
            <StatusBadge
              label={t(asset.AssetType)}
              variant='info'
              copyable={false}
            />
            <StatusBadge
              label={t(asset.Status || 'Unknown')}
              variant={getAssetStatusVariant(asset.Status)}
              copyable={false}
            />
          </div>
        </div>
        <AssetRowActions row={props.row} />
      </div>

      <div className='grid grid-cols-2 gap-3 text-sm'>
        <div className='min-w-0'>
          <p className='text-muted-foreground text-xs'>{t('Asset Group')}</p>
          <p className='truncate'>{props.group?.Name || asset.GroupId}</p>
        </div>
        <div className='min-w-0'>
          <p className='text-muted-foreground text-xs'>
            {t('Channel availability')}
          </p>
          <ReplicationBadge replication={asset.Replication} />
        </div>
      </div>
    </div>
  )
}

export const AssetCard = memo(AssetCardComponent)
