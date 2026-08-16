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
import { FolderOpen } from 'lucide-react'
import { memo } from 'react'
import { useTranslation } from 'react-i18next'

import { StatusBadge } from '@/components/status-badge'
import { TableId } from '@/components/table-id'

import type { AssetGroup } from '../types'
import { GroupRowActions } from './group-row-actions'
import { ReplicationBadge } from './replication-badge'

function GroupCardComponent(props: { row: Row<AssetGroup> }) {
  const { t } = useTranslation()
  const group = props.row.original

  return (
    <div className='flex min-w-0 flex-col gap-3'>
      <div className='flex items-start gap-3'>
        <span className='bg-muted/50 flex size-11 shrink-0 items-center justify-center rounded-lg border'>
          <FolderOpen
            className='text-muted-foreground size-5'
            aria-hidden='true'
          />
        </span>
        <div className='min-w-0 flex-1'>
          <p className='truncate font-medium'>{group.Name}</p>
          <TableId
            value={group.Id}
            className='block max-w-full truncate text-xs'
          />
        </div>
        <GroupRowActions row={props.row} />
      </div>

      <p className='text-muted-foreground line-clamp-2 min-h-10 text-sm'>
        {group.Description || t('No description')}
      </p>

      <div className='flex items-center justify-between gap-2'>
        <StatusBadge label={group.GroupType} variant='info' copyable={false} />
        <ReplicationBadge replication={group.Replication} />
      </div>
    </div>
  )
}

export const GroupCard = memo(GroupCardComponent)
