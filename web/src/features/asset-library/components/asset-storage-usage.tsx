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
import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'
import { api } from '@/lib/api'

import { assetLibraryQueryKeys } from '../lib'
import { AssetQuotaDialog } from './asset-quota-dialog'

type StorageUsage = {
  enabled: boolean
  quota_mb: number
  used_bytes: number
  remaining_bytes: number
  bytes_per_mb: number
  quota_override_mb: number | null
  default_quota_mb: number
  can_edit: boolean
}

export function AssetStorageUsage(props: {
  targetUserId?: number
  currentAdminId?: number
}) {
  const { t, i18n } = useTranslation()
  const [editing, setEditing] = useState(false)
  const managedUserId = props.targetUserId ?? props.currentAdminId
  const path = managedUserId
    ? `/api/asset-library/admin/users/${managedUserId}/storage`
    : '/api/asset-library/storage'
  const usage = useQuery({
    queryKey: [
      ...assetLibraryQueryKeys.assets(props.targetUserId),
      'storage-usage',
    ],
    queryFn: async () => {
      const response = await api.get<{
        success: boolean
        message?: string
        data: StorageUsage
      }>(path)
      if (!response.data.success) {
        throw new Error(response.data.message || 'Failed to load storage usage')
      }
      return response.data.data
    },
    staleTime: 10_000,
  })
  if (usage.isError) {
    return (
      <p role='status' className='text-destructive text-sm'>
        {t('Failed to load storage usage')}
      </p>
    )
  }
  if (!usage.data?.enabled) return null
  const format = new Intl.NumberFormat(i18n.language, {
    maximumFractionDigits: 2,
  })
  return (
    <div className='flex flex-wrap items-center gap-2'>
      <p className='text-muted-foreground text-sm' role='status'>
        {t(
          'Asset storage: {{used}} / {{quota}} MB · {{remaining}} MB remaining',
          {
            used: format.format(
              usage.data.used_bytes / usage.data.bytes_per_mb
            ),
            quota: format.format(usage.data.quota_mb),
            remaining: format.format(
              usage.data.remaining_bytes / usage.data.bytes_per_mb
            ),
          }
        )}
      </p>
      {usage.data.can_edit ? (
        <Button size='sm' variant='outline' onClick={() => setEditing(true)}>
          {t('Set user quota')}
        </Button>
      ) : null}
      {editing && usage.data.can_edit ? (
        <AssetQuotaDialog
          path={path}
          quotaOverrideMB={usage.data.quota_override_mb}
          defaultQuotaMB={usage.data.default_quota_mb}
          onClose={() => setEditing(false)}
        />
      ) : null}
    </div>
  )
}
