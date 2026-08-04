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
import { useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { PublicLayout } from '@/components/layout'
import { PageTransition } from '@/components/page-transition'
import { Button } from '@/components/ui/button'
import { Skeleton } from '@/components/ui/skeleton'

import { getModelDocumentCatalog } from './api'
import { ModelDocSidebar } from './components/model-doc-sidebar'
import { ModelDocViewer } from './components/model-doc-viewer'

const ALL_FILTER_VALUE = 'all'

export function ModelDocs() {
  const { t } = useTranslation()
  const [search, setSearch] = useState('')
  const [vendor, setVendor] = useState(ALL_FILTER_VALUE)
  const [category, setCategory] = useState(ALL_FILTER_VALUE)
  const [selectedSlug, setSelectedSlug] = useState<string | null>(null)

  const catalogQuery = useQuery({
    queryKey: ['model-docs'],
    queryFn: getModelDocumentCatalog,
    staleTime: 5 * 60 * 1000,
  })

  const documents = useMemo(
    () => catalogQuery.data?.data ?? [],
    [catalogQuery.data?.data]
  )
  const vendors = useMemo(
    () =>
      [...new Set(documents.map((document) => document.vendor))].sort((a, b) =>
        a.localeCompare(b)
      ),
    [documents]
  )
  const categories = useMemo(
    () =>
      [...new Set(documents.map((document) => document.category))].sort(
        (a, b) => a.localeCompare(b)
      ),
    [documents]
  )
  const filteredDocuments = useMemo(() => {
    const query = search.trim().toLowerCase()
    return documents.filter((document) => {
      if (vendor !== ALL_FILTER_VALUE && document.vendor !== vendor) {
        return false
      }
      if (category !== ALL_FILTER_VALUE && document.category !== category) {
        return false
      }
      if (!query) return true
      return [
        document.title,
        document.model,
        document.vendor,
        document.summary,
      ].some((value) => value.toLowerCase().includes(query))
    })
  }, [category, documents, search, vendor])

  const selectedDocument =
    filteredDocuments.find((document) => document.slug === selectedSlug) ??
    filteredDocuments[0]

  const clearFilters = () => {
    setSearch('')
    setVendor(ALL_FILTER_VALUE)
    setCategory(ALL_FILTER_VALUE)
  }

  return (
    <PublicLayout showMainContainer={false}>
      <PageTransition className='mx-auto flex w-full max-w-[1600px] flex-col px-4 pt-20 pb-5 sm:px-6 sm:pt-24 lg:fixed lg:inset-0 lg:overflow-hidden lg:px-8'>
        <header className='mb-5 shrink-0'>
          <h1 className='text-2xl font-bold tracking-tight sm:text-3xl'>
            {t('Model documentation')}
          </h1>
          <p className='text-muted-foreground mt-1.5 text-sm'>
            {t(
              'Filter by provider or model category. Every result opens an independent HTML document.'
            )}
          </p>
        </header>

        {catalogQuery.isLoading && (
          <div className='grid min-h-[620px] overflow-hidden rounded-xl border lg:min-h-0 lg:flex-1 lg:grid-cols-[320px_minmax(0,1fr)]'>
            <div className='border-b p-4 lg:border-r lg:border-b-0'>
              <Skeleton className='h-9 w-full' />
              <div className='mt-4 space-y-3'>
                {Array.from({ length: 6 }, (_, index) => (
                  <Skeleton key={index} className='h-20 w-full rounded-lg' />
                ))}
              </div>
            </div>
            <Skeleton className='m-6 h-[560px] rounded-lg' />
          </div>
        )}

        {catalogQuery.isError && (
          <div className='border-destructive/30 bg-destructive/5 rounded-xl border p-8 text-center'>
            <h2 className='font-semibold'>
              {t('Unable to load model documentation')}
            </h2>
            <Button
              className='mt-4'
              variant='outline'
              onClick={() => catalogQuery.refetch()}
            >
              {t('Retry')}
            </Button>
          </div>
        )}

        {!catalogQuery.isLoading && !catalogQuery.isError && (
          <div className='bg-card grid overflow-hidden rounded-xl border lg:min-h-0 lg:flex-1 lg:grid-cols-[320px_minmax(0,1fr)]'>
            <ModelDocSidebar
              documents={filteredDocuments}
              vendors={vendors}
              categories={categories}
              search={search}
              vendor={vendor}
              category={category}
              selectedSlug={selectedDocument?.slug ?? null}
              onSearchChange={setSearch}
              onVendorChange={setVendor}
              onCategoryChange={setCategory}
              onSelect={setSelectedSlug}
              onClearFilters={clearFilters}
            />
            <ModelDocViewer
              key={selectedDocument?.slug ?? 'empty'}
              document={selectedDocument}
            />
          </div>
        )}
      </PageTransition>
    </PublicLayout>
  )
}
