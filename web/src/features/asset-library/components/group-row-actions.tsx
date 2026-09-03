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
import { Eye, MoreHorizontal, Pencil, RefreshCw, Trash2 } from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuShortcut,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
import {
  ADMIN_PERMISSION_ACTIONS,
  ADMIN_PERMISSION_RESOURCES,
  hasPermission,
} from '@/lib/admin-permissions'
import { useAuthStore } from '@/stores/auth-store'

import type { AssetGroup } from '../types'
import { useAssetLibrary } from './asset-library-provider'

export function GroupRowActions(props: { row: Row<AssetGroup> }) {
  const { t } = useTranslation()
  const { openGroupDialog, isReadOnly } = useAssetLibrary()
  const user = useAuthStore((state) => state.auth.user)
  const canViewUpstreamDetails = hasPermission(
    user,
    ADMIN_PERMISSION_RESOURCES.CHANNEL,
    ADMIN_PERMISSION_ACTIONS.READ
  )
  const canSyncUpstreams = hasPermission(
    user,
    ADMIN_PERMISSION_RESOURCES.CHANNEL,
    ADMIN_PERMISSION_ACTIONS.SENSITIVE_WRITE
  )
  const group = props.row.original

  if (isReadOnly) return null

  return (
    <DropdownMenu>
      <DropdownMenuTrigger
        render={
          <Button
            variant='ghost'
            size='icon-sm'
            aria-label={t('Open asset group actions')}
          />
        }
      >
        <MoreHorizontal className='size-4' />
      </DropdownMenuTrigger>
      <DropdownMenuContent align='end' className='w-52'>
        {canViewUpstreamDetails || canSyncUpstreams ? (
          <DropdownMenuItem
            onClick={() => openGroupDialog('group-details', group)}
          >
            {t(
              canViewUpstreamDetails ? 'View details' : 'Sync group and assets'
            )}
            <DropdownMenuShortcut>
              {canViewUpstreamDetails ? (
                <Eye className='size-4' />
              ) : (
                <RefreshCw className='size-4' />
              )}
            </DropdownMenuShortcut>
          </DropdownMenuItem>
        ) : null}
        <DropdownMenuItem
          onClick={() => openGroupDialog('update-group', group)}
        >
          {t('Edit')}
          <DropdownMenuShortcut>
            <Pencil className='size-4' />
          </DropdownMenuShortcut>
        </DropdownMenuItem>
        <DropdownMenuItem
          className='text-destructive focus:text-destructive'
          onClick={() => openGroupDialog('delete-group', group)}
        >
          {t('Delete')}
          <DropdownMenuShortcut>
            <Trash2 className='size-4' />
          </DropdownMenuShortcut>
        </DropdownMenuItem>
      </DropdownMenuContent>
    </DropdownMenu>
  )
}
