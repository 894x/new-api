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
import { useTranslation } from 'react-i18next'

import { api } from '@/lib/api'

import { assetLibraryQueryKeys } from '../lib'

type StorageUsage = {
  enabled: boolean
  quota_mb: number
  used_bytes: number
  remaining_bytes: number
  bytes_per_mb: number
}

export function AssetStorageUsage() {
  const { t, i18n } = useTranslation()
  const usage = useQuery({
    queryKey: [...assetLibraryQueryKeys.assets(), 'storage-usage'],
    queryFn: async () => {
      const response = await api.get<{
        success: boolean
        message?: string
        data: StorageUsage
      }>('/api/asset-library/storage')
      if (!response.data.success)
        {throw new Error(response.data.message || 'Failed to load storage usage')}
      return response.data.data
    },
    staleTime: 10_000,
  })
  if (usage.isError)
    {return (
      <p role='status' className='text-destructive text-sm'>
        {t('Failed to load storage usage')}
      </p>
    )}
  if (!usage.data?.enabled) return null
  const format = new Intl.NumberFormat(i18n.language, {
    maximumFractionDigits: 2,
  })
  return (
    <p className='text-muted-foreground text-sm' role='status'>
      {t(
        'Asset storage: {{used}} / {{quota}} MB · {{remaining}} MB remaining',
        {
          used: format.format(usage.data.used_bytes / usage.data.bytes_per_mb),
          quota: format.format(usage.data.quota_mb),
          remaining: format.format(
            usage.data.remaining_bytes / usage.data.bytes_per_mb
          ),
        }
      )}
    </p>
  )
}
