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

import { StatusBadge } from '@/components/status-badge'
import { TableId } from '@/components/table-id'
import { TruncatedText } from '@/components/truncated-text'

import type { AssetGroup } from '../types'
import { formatAssetDate } from './asset-columns'
import { GroupRowActions } from './group-row-actions'
import { ReplicationBadge } from './replication-badge'

export function useAssetGroupColumns(): ColumnDef<AssetGroup>[] {
  const { t } = useTranslation()

  return [
    {
      accessorKey: 'Name',
      header: t('Name'),
      cell: ({ row }) => (
        <div className='min-w-0 space-y-0.5'>
          <TruncatedText text={row.original.Name} className='font-medium' />
          <TableId
            value={row.original.Id}
            className='max-w-64 truncate text-xs'
          />
        </div>
      ),
      size: 260,
      meta: { mobileTitle: true },
    },
    {
      accessorKey: 'GroupType',
      header: t('Type'),
      cell: ({ row }) => (
        <StatusBadge
          label={row.original.GroupType}
          variant='info'
          copyable={false}
          className='-ml-1.5'
        />
      ),
      size: 120,
      meta: { mobileBadge: true },
    },
    {
      accessorKey: 'Description',
      header: t('Description'),
      cell: ({ row }) => (
        <TruncatedText
          text={row.original.Description || '-'}
          maxWidth='max-w-[360px]'
        />
      ),
      enableSorting: false,
      size: 360,
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
      cell: ({ row }) => <GroupRowActions row={row} />,
      enableSorting: false,
      enableHiding: false,
      size: 64,
      meta: { pinned: 'right' as const },
    },
  ]
}
