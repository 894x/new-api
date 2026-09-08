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
import { useEffect, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { api } from '@/lib/api'
import { cn } from '@/lib/utils'

import type { Asset } from '../types'
import { useAssetLibrary } from './asset-library-provider'

// HTTP sources need an authenticated same-origin fetch: HTTPS pages cannot
// reliably embed them directly. Keep the original source URL for copy/export.
export function AssetImage(props: {
  asset: Asset
  className?: string
  lazy?: boolean
}) {
  const { t } = useTranslation()
  const { targetUserId } = useAssetLibrary()
  const proxy = /^http:\/\//i.test(props.asset.URL ?? '')
  const placeholder = useRef<HTMLSpanElement>(null)
  const [visible, setVisible] = useState(
    !props.lazy || typeof IntersectionObserver === 'undefined'
  )
  const [failedSource, setFailedSource] = useState('')
  const [preview, setPreview] = useState<{ blob: Blob; url: string } | null>(
    null
  )
  const query = useQuery({
    queryKey: [
      'asset-image-preview',
      targetUserId ?? 'self',
      props.asset.Id,
      props.asset.URL,
    ],
    queryFn: async ({ signal }) => {
      const prefix = targetUserId
        ? `/api/asset-library/admin/users/${targetUserId}/assets`
        : '/api/asset-library/assets'
      const response = await api.get<Blob>(
        `${prefix}/${encodeURIComponent(props.asset.Id)}/preview`,
        {
          responseType: 'blob',
          signal,
          skipErrorHandler: true,
        }
      )
      return response.data
    },
    enabled: proxy && (visible || !props.lazy),
    staleTime: 60_000,
    gcTime: 0,
    retry: false,
  })
  useEffect(() => {
    if (!proxy || visible || !props.lazy || !placeholder.current) return
    const observer = new IntersectionObserver((entries) => {
      if (entries.some((entry) => entry.isIntersecting)) {
        setVisible(true)
        observer.disconnect()
      }
    })
    observer.observe(placeholder.current)
    return () => observer.disconnect()
  }, [proxy, visible, props.lazy])
  useEffect(() => {
    if (!proxy || !query.data) return
    const url = URL.createObjectURL(query.data)
    setPreview({ blob: query.data, url })
    return () => URL.revokeObjectURL(url)
  }, [proxy, query.data])
  let src = props.asset.URL
  if (proxy) src = preview?.blob === query.data ? (preview?.url ?? '') : ''
  if (query.isError || (src && failedSource === src)) {
    return (
      <span
        role='status'
        className={cn(
          'text-muted-foreground flex items-center justify-center rounded-md border p-2 text-center text-xs whitespace-normal',
          props.className
        )}
      >
        {t('Failed to load asset preview.')}
      </span>
    )
  }
  if (!src) {
    return (
      <span
        ref={placeholder}
        role='status'
        className={cn(
          'bg-muted text-muted-foreground flex items-center justify-center rounded-md border text-xs',
          props.className
        )}
      >
        {t('Loading...')}
      </span>
    )
  }
  return (
    <img
      src={src}
      alt={props.asset.Name || t('Asset preview')}
      loading={props.lazy ? 'lazy' : undefined}
      onError={() => setFailedSource(src)}
      className={props.className}
    />
  )
}
