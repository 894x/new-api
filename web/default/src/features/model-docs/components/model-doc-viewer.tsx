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
import { FileTextIcon } from 'lucide-react'
import { useCallback, useEffect, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { Skeleton } from '@/components/ui/skeleton'
import { useTheme } from '@/context/theme-provider'

import type { ModelDocument } from '../types'

const DOCUMENT_THEME_PROPERTIES = [
  '--background',
  '--foreground',
  '--card',
  '--card-foreground',
  '--muted',
  '--muted-foreground',
  '--border',
  '--primary',
  '--primary-foreground',
] as const

type ModelDocViewerProps = {
  document?: ModelDocument
}

export function ModelDocViewer(props: ModelDocViewerProps) {
  const { t } = useTranslation()
  const { resolvedTheme } = useTheme()
  const iframeRef = useRef<HTMLIFrameElement>(null)
  const viewerRef = useRef<HTMLElement>(null)
  const resizeObserverRef = useRef<ResizeObserver | null>(null)
  const [loadedSlug, setLoadedSlug] = useState<string | null>(null)
  const [frameHeight, setFrameHeight] = useState(520)

  const resetViewerScroll = useCallback(() => {
    if (!viewerRef.current) return
    viewerRef.current.scrollTop = 0
    window.requestAnimationFrame(() => {
      window.requestAnimationFrame(() => {
        if (viewerRef.current) {
          viewerRef.current.scrollTop = 0
        }
      })
    })
  }, [])

  const syncFrameHeight = useCallback(() => {
    const frameDocument = iframeRef.current?.contentDocument
    if (!frameDocument) return

    const nextHeight = Math.max(
      frameDocument.documentElement.scrollHeight,
      frameDocument.body?.scrollHeight ?? 0,
      520
    )
    setFrameHeight(nextHeight)
  }, [])

  const syncDocumentTheme = useCallback(() => {
    const documentRoot = iframeRef.current?.contentDocument?.documentElement
    if (!documentRoot) return

    const siteStyles = window.getComputedStyle(window.document.documentElement)
    for (const property of DOCUMENT_THEME_PROPERTIES) {
      documentRoot.style.setProperty(
        property,
        siteStyles.getPropertyValue(property)
      )
    }
    documentRoot.dataset.theme = resolvedTheme
    documentRoot.style.overflow = 'hidden'
    if (iframeRef.current?.contentDocument?.body) {
      iframeRef.current.contentDocument.body.style.overflow = 'hidden'
    }
    syncFrameHeight()
  }, [resolvedTheme, syncFrameHeight])

  useEffect(() => {
    syncDocumentTheme()
  }, [syncDocumentTheme])

  useEffect(() => {
    resetViewerScroll()
    const resetTimer = window.setTimeout(resetViewerScroll, 250)
    return () => window.clearTimeout(resetTimer)
  }, [frameHeight, props.document?.slug, resetViewerScroll])

  useEffect(
    () => () => {
      resizeObserverRef.current?.disconnect()
    },
    []
  )

  if (!props.document) {
    return (
      <section className='bg-background flex min-h-[520px] items-center justify-center p-8 text-center'>
        <div>
          <FileTextIcon
            aria-hidden='true'
            className='text-muted-foreground/50 mx-auto size-10'
          />
          <h2 className='mt-4 font-semibold'>
            {t('No model documents found')}
          </h2>
          <p className='text-muted-foreground mt-1 text-sm'>
            {t('Try another provider, category, or search term.')}
          </p>
        </div>
      </section>
    )
  }

  const isLoading = loadedSlug !== props.document.slug

  return (
    <section
      ref={viewerRef}
      tabIndex={0}
      aria-label={`${t('Model documentation')}: ${props.document.title}`}
      className='bg-background focus-visible:ring-ring/50 min-w-0 overflow-y-auto overscroll-contain outline-none [overflow-anchor:none] focus-visible:ring-3 lg:h-full'
    >
      <div className='relative min-h-[520px] w-full'>
        {isLoading && (
          <div className='bg-background absolute inset-0 z-10 space-y-5 p-8'>
            <Skeleton className='h-5 w-32' />
            <Skeleton className='h-10 w-2/3' />
            <Skeleton className='h-4 w-1/2' />
            <Skeleton className='mt-8 h-12 w-full' />
            <Skeleton className='h-64 w-full' />
          </div>
        )}
        <iframe
          ref={iframeRef}
          src={`/api/model-docs/${props.document.slug}`}
          title={props.document.title}
          height={frameHeight}
          className='block w-full border-0'
          sandbox='allow-same-origin'
          scrolling='no'
          onLoad={() => {
            syncDocumentTheme()
            iframeRef.current?.contentWindow?.scrollTo(0, 0)
            resetViewerScroll()

            resizeObserverRef.current?.disconnect()
            const frameDocument = iframeRef.current?.contentDocument
            if (frameDocument) {
              resizeObserverRef.current = new ResizeObserver(syncFrameHeight)
              resizeObserverRef.current.observe(frameDocument.documentElement)
              if (frameDocument.body) {
                resizeObserverRef.current.observe(frameDocument.body)
              }
            }
            syncFrameHeight()
            setLoadedSlug(props.document?.slug ?? null)
          }}
        />
      </div>
    </section>
  )
}
