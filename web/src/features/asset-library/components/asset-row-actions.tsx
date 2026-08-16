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
import { Eye, MoreHorizontal, Pencil, Trash2 } from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuShortcut,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'

import type { Asset } from '../types'
import { useAssetLibrary } from './asset-library-provider'

export function AssetRowActions(props: { row: Row<Asset> }) {
  const { t } = useTranslation()
  const { openAssetDialog } = useAssetLibrary()
  const asset = props.row.original

  return (
    <DropdownMenu>
      <DropdownMenuTrigger
        render={
          <Button
            variant='ghost'
            size='icon-sm'
            aria-label={t('Open asset actions')}
          />
        }
      >
        <MoreHorizontal className='size-4' />
      </DropdownMenuTrigger>
      <DropdownMenuContent align='end' className='w-40'>
        <DropdownMenuItem
          onClick={() => openAssetDialog('preview-asset', asset)}
        >
          {t('Preview')}
          <DropdownMenuShortcut>
            <Eye className='size-4' />
          </DropdownMenuShortcut>
        </DropdownMenuItem>
        <DropdownMenuItem
          onClick={() => openAssetDialog('update-asset', asset)}
        >
          {t('Edit')}
          <DropdownMenuShortcut>
            <Pencil className='size-4' />
          </DropdownMenuShortcut>
        </DropdownMenuItem>
        <DropdownMenuItem
          className='text-destructive focus:text-destructive'
          onClick={() => openAssetDialog('delete-asset', asset)}
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
