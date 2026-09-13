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
import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { api } from '@/lib/api'

import type { Asset } from '../types'
import { useAssetLibrary } from './asset-library-provider'

export function AssetStoredMedia({ asset }: { asset: Asset }) {
  const { t } = useTranslation()
  const { targetUserId } = useAssetLibrary()
  const [preview, setPreview] = useState<{ blob: Blob; url: string } | null>(
    null
  )
  const media = useQuery({
    queryKey: ['asset-stored-media', targetUserId ?? 'self', asset.Id],
    queryFn: async ({ signal }) => {
      const prefix = targetUserId
        ? `/api/asset-library/admin/users/${targetUserId}/assets`
        : '/api/asset-library/assets'
      const response = await api.get<Blob>(
        `${prefix}/${encodeURIComponent(asset.Id)}/content`,
        { responseType: 'blob', signal, skipErrorHandler: true }
      )
      return response.data
    },
    gcTime: 0,
    retry: false,
  })
  useEffect(() => {
    if (!media.data) return
    const url = URL.createObjectURL(media.data)
    setPreview({ blob: media.data, url })
    return () => URL.revokeObjectURL(url)
  }, [media.data])
  if (media.isError)
    {return <p role='status'>{t('Failed to load asset preview.')}</p>}
  const src = preview?.blob === media.data ? preview?.url : undefined
  if (!src) return <p role='status'>{t('Loading...')}</p>
  if (asset.AssetType === 'Audio')
    {return (
      <audio src={src} controls className='w-full'>
        {t('Your browser does not support audio playback.')}
      </audio>
    )}
  return (
    <video
      src={src}
      controls
      playsInline
      className='max-h-[60vh] w-full rounded-lg'
    >
      {t('Your browser does not support video playback.')}
    </video>
  )
}
