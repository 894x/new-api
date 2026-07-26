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
import i18next from 'i18next'
import { useEffect, useState } from 'react'
import { toast } from 'sonner'

import { isHttpUrl } from '@/lib/content-format'

import { getHomePageContent } from '../api'
import {
  HOME_PAGE_TEMPLATES,
  type HomePageContentResult,
  type HomePageTemplate,
} from '../types'

const CONTENT_STORAGE_KEY = 'home_page_content'
const TEMPLATE_STORAGE_KEY = 'home_page_template'

export function resolveHomePageTemplate(
  value: string | null | undefined,
  content: string
): HomePageTemplate {
  if (value && HOME_PAGE_TEMPLATES.includes(value as HomePageTemplate)) {
    return value as HomePageTemplate
  }
  return content ? 'custom' : 'system'
}

/**
 * Hook to load and manage custom home page content
 * Supports both Markdown/HTML content and iframe URLs
 */
export function useHomePageContent(): HomePageContentResult {
  const [content, setContent] = useState<string>('')
  const [template, setTemplate] = useState<HomePageTemplate>('system')
  const [isLoaded, setIsLoaded] = useState(false)

  useEffect(() => {
    let mounted = true

    const loadContent = async () => {
      // Load from localStorage first for immediate display
      const cached = localStorage.getItem(CONTENT_STORAGE_KEY) || ''
      const cachedTemplate = localStorage.getItem(TEMPLATE_STORAGE_KEY)
      if (cached && mounted) {
        setContent(cached)
      }
      if (mounted) {
        setTemplate(resolveHomePageTemplate(cachedTemplate, cached))
      }

      try {
        const response = await getHomePageContent()
        const { success, data, template: configuredTemplate } = response

        if (!mounted) return

        if (success) {
          const nextContent = data || ''
          const nextTemplate = resolveHomePageTemplate(
            configuredTemplate,
            nextContent
          )
          setContent(nextContent)
          setTemplate(nextTemplate)
          localStorage.setItem(TEMPLATE_STORAGE_KEY, nextTemplate)
          if (nextContent) {
            localStorage.setItem(CONTENT_STORAGE_KEY, nextContent)
          } else {
            localStorage.removeItem(CONTENT_STORAGE_KEY)
          }
        } else {
          setContent('')
          setTemplate('system')
          localStorage.removeItem(CONTENT_STORAGE_KEY)
          localStorage.removeItem(TEMPLATE_STORAGE_KEY)
        }
      } catch (error) {
        if (!mounted) return
        // eslint-disable-next-line no-console
        console.error('Failed to load home page content:', error)
        toast.error(i18next.t('Failed to load home page content'))
      } finally {
        if (mounted) {
          setIsLoaded(true)
        }
      }
    }

    loadContent()

    return () => {
      mounted = false
    }
  }, [])

  const isUrl = isHttpUrl(content)

  return { content, isLoaded, isUrl, template }
}
