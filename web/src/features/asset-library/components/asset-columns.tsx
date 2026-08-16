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
import type { ColumnDef } from '@tanstack/react-table'
import { useTranslation } from 'react-i18next'

import { StatusBadge, type StatusVariant } from '@/components/status-badge'
import { TableId } from '@/components/table-id'
import { TruncatedText } from '@/components/truncated-text'
import dayjs from '@/lib/dayjs'

import type { Asset, AssetGroup } from '../types'
import { AssetRowActions } from './asset-row-actions'
import { AssetThumbnail } from './asset-thumbnail'
import { ReplicationBadge } from './replication-badge'

function getAssetStatusVariant(status?: string): StatusVariant {
  if (status === 'Active') return 'success'
  if (status === 'Failed') return 'danger'
  if (status === 'Processing') return 'warning'
  return 'neutral'
}

function formatAssetDate(value?: string): string {
  if (!value) return '-'
  const parsed = dayjs(value)
  return parsed.isValid() ? parsed.format('YYYY-MM-DD HH:mm:ss') : value
}

export function useAssetColumns(
  groupsById: Map<string, AssetGroup>
): ColumnDef<Asset>[] {
  const { t } = useTranslation()

  return [
    {
      id: 'preview',
      header: t('Preview'),
      cell: ({ row }) => <AssetThumbnail asset={row.original} />,
      enableSorting: false,
      enableHiding: false,
      size: 72,
      meta: { mobileHidden: true },
    },
    {
      accessorKey: 'Name',
      header: t('Name'),
      cell: ({ row }) => (
        <div className='min-w-0 space-y-0.5'>
          <TruncatedText
            text={row.original.Name || t('Untitled asset')}
            className='font-medium'
          />
          <TableId
            value={row.original.Id}
            className='max-w-56 truncate text-xs'
          />
        </div>
      ),
      size: 240,
      meta: { mobileTitle: true },
    },
    {
      accessorKey: 'AssetType',
      header: t('Type'),
      cell: ({ row }) => (
        <StatusBadge
          label={t(row.original.AssetType)}
          variant='info'
          copyable={false}
          className='-ml-1.5'
        />
      ),
      enableSorting: false,
      size: 110,
      meta: { mobileBadge: true },
    },
    {
      accessorKey: 'Status',
      header: t('Status'),
      cell: ({ row }) => (
        <StatusBadge
          label={t(row.original.Status || 'Unknown')}
          variant={getAssetStatusVariant(row.original.Status)}
          copyable={false}
          className='-ml-1.5'
        />
      ),
      enableSorting: false,
      size: 120,
    },
    {
      accessorKey: 'GroupId',
      header: t('Asset Group'),
      cell: ({ row }) => {
        const group = groupsById.get(row.original.GroupId)
        return (
          <div className='min-w-0'>
            <TruncatedText text={group?.Name || row.original.GroupId} />
            {group && (
              <TableId value={row.original.GroupId} className='block text-xs' />
            )}
          </div>
        )
      },
      enableSorting: false,
      size: 220,
    },
    {
      id: 'replication',
      header: t('Channel availability'),
      cell: ({ row }) => (
        <ReplicationBadge replication={row.original.Replication} />
      ),
      enableSorting: false,
      size: 190,
    },
    {
      accessorKey: 'CreateTime',
      header: t('Created'),
      cell: ({ row }) => (
        <span className='text-muted-foreground whitespace-nowrap'>
          {formatAssetDate(row.original.CreateTime)}
        </span>
      ),
      size: 180,
      meta: { mobileHidden: true },
    },
    {
      id: 'actions',
      header: t('Actions'),
      cell: ({ row }) => <AssetRowActions row={row} />,
      enableSorting: false,
      enableHiding: false,
      size: 64,
      meta: { pinned: 'right' as const },
    },
  ]
}

export { formatAssetDate, getAssetStatusVariant }
