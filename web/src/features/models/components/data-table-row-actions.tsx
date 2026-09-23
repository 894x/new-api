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
import { useQueryClient } from '@tanstack/react-query'
import type { Row } from '@tanstack/react-table'
import { Eye, EyeOff, FileCode2, Network, Pencil, Trash2 } from 'lucide-react'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import { DataTableRowActionMenu } from '@/components/data-table/core/row-action-menu'
import { Button } from '@/components/ui/button'
import {
  DropdownMenuItem,
  DropdownMenuShortcut,
} from '@/components/ui/dropdown-menu'
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from '@/components/ui/tooltip'
import {
  ADMIN_PERMISSION_ACTIONS,
  ADMIN_PERMISSION_RESOURCES,
  hasPermission,
} from '@/lib/admin-permissions'
import { useAuthStore } from '@/stores/auth-store'

import { handleToggleModelStatus, isModelEnabled } from '../lib'
import type { Model } from '../types'
import { ModelDeleteDialog } from './dialogs/model-delete-dialog'
import { useModels } from './models-provider'

interface DataTableRowActionsProps {
  row: Row<Model>
}

export function DataTableRowActions({ row }: DataTableRowActionsProps) {
  const { t } = useTranslation()
  const model = row.original
  const { setOpen, setCurrentRow } = useModels()
  const queryClient = useQueryClient()
  const user = useAuthStore((state) => state.auth.user)
  const [deleteConfirmOpen, setDeleteConfirmOpen] = useState(false)

  const isEnabled = isModelEnabled(model)
  const canReadChannels = hasPermission(
    user,
    ADMIN_PERMISSION_RESOURCES.CHANNEL,
    ADMIN_PERMISSION_ACTIONS.READ
  )

  const handleEdit = () => {
    setCurrentRow(model)
    setOpen('update-model')
  }

  const handleToggleStatus = () => {
    handleToggleModelStatus(model.id, model.status, queryClient)
  }

  const handleEditDocument = () => {
    setCurrentRow(model)
    setOpen('edit-document')
  }

  const handleViewChannelCapabilities = () => {
    setCurrentRow(model)
    setOpen('view-channel-capabilities')
  }

  const toggleLabel = isEnabled
    ? t('Hide from model square')
    : t('Show in model square')

  return (
    <div className='-ml-1.5 flex items-center gap-1'>
      <Tooltip>
        <TooltipTrigger
          render={
            <Button
              variant='ghost'
              size='icon-sm'
              onClick={handleEdit}
              aria-label={t('Edit')}
            />
          }
        >
          <Pencil />
        </TooltipTrigger>
        <TooltipContent>{t('Edit')}</TooltipContent>
      </Tooltip>

      {model.name_rule === 0 && canReadChannels && (
        <Tooltip>
          <TooltipTrigger
            render={
              <Button
                variant='ghost'
                size='icon-sm'
                onClick={handleViewChannelCapabilities}
                aria-label={t('View channel capabilities')}
              />
            }
          >
            <Network />
          </TooltipTrigger>
          <TooltipContent>{t('View channel capabilities')}</TooltipContent>
        </Tooltip>
      )}

      <Tooltip>
        <TooltipTrigger
          render={
            <Button
              variant='ghost'
              size='icon-sm'
              onClick={handleEditDocument}
              aria-label={t('Edit model document')}
            />
          }
        >
          <FileCode2 />
        </TooltipTrigger>
        <TooltipContent>{t('Edit model document')}</TooltipContent>
      </Tooltip>

      <Tooltip>
        <TooltipTrigger
          render={
            <Button
              variant='ghost'
              size='icon-sm'
              onClick={handleToggleStatus}
              aria-label={toggleLabel}
              className={
                isEnabled
                  ? 'text-destructive hover:text-destructive'
                  : 'text-success hover:text-success'
              }
            />
          }
        >
          {isEnabled ? <EyeOff /> : <Eye />}
        </TooltipTrigger>
        <TooltipContent>{toggleLabel}</TooltipContent>
      </Tooltip>

      <DataTableRowActionMenu ariaLabel={t('Open menu')}>
        <DropdownMenuItem onClick={handleToggleStatus}>
          {toggleLabel}
          <DropdownMenuShortcut>
            {isEnabled ? <EyeOff size={16} /> : <Eye size={16} />}
          </DropdownMenuShortcut>
        </DropdownMenuItem>
        <DropdownMenuItem
          onSelect={(e) => {
            e.preventDefault()
            setDeleteConfirmOpen(true)
          }}
          className='text-destructive focus:text-destructive'
        >
          {t('Delete')}
          <DropdownMenuShortcut>
            <Trash2 size={16} />
          </DropdownMenuShortcut>
        </DropdownMenuItem>
      </DataTableRowActionMenu>

      {deleteConfirmOpen && (
        <ModelDeleteDialog
          models={[model]}
          onClose={() => setDeleteConfirmOpen(false)}
        />
      )}
    </div>
  )
}
