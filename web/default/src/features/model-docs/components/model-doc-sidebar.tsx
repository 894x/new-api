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
import { SearchIcon, XIcon } from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { ScrollArea } from '@/components/ui/scroll-area'
import {
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { cn } from '@/lib/utils'

import type { ModelDocument } from '../types'

const ALL_FILTER_VALUE = 'all'

type ModelDocSidebarProps = {
  documents: ModelDocument[]
  vendors: string[]
  categories: string[]
  search: string
  vendor: string
  category: string
  selectedSlug: string | null
  onSearchChange: (value: string) => void
  onVendorChange: (value: string) => void
  onCategoryChange: (value: string) => void
  onSelect: (slug: string) => void
  onClearFilters: () => void
}

function getCategoryLabel(category: string, t: (key: string) => string) {
  switch (category) {
    case 'video':
      return t('Video')
    case 'image':
      return t('Image')
    case 'audio':
      return t('Audio')
    case 'embedding':
      return t('Embeddings')
    case 'rerank':
      return t('Rerank')
    default:
      return t('Text')
  }
}

export function ModelDocSidebar(props: ModelDocSidebarProps) {
  const { t } = useTranslation()
  const hasFilters =
    props.search !== '' ||
    props.vendor !== ALL_FILTER_VALUE ||
    props.category !== ALL_FILTER_VALUE

  return (
    <aside className='bg-muted/20 flex min-h-0 flex-col border-b lg:border-r lg:border-b-0'>
      <div className='space-y-3 border-b p-4'>
        <div className='relative'>
          <SearchIcon
            aria-hidden='true'
            className='text-muted-foreground pointer-events-none absolute top-1/2 left-3 size-4 -translate-y-1/2'
          />
          <Input
            value={props.search}
            onChange={(event) => props.onSearchChange(event.target.value)}
            placeholder={t('Search model documentation...')}
            aria-label={t('Search model documentation')}
            className='bg-background pr-9 pl-9'
          />
          {props.search && (
            <Button
              type='button'
              variant='ghost'
              size='icon-sm'
              aria-label={t('Clear filters')}
              className='absolute top-1/2 right-1.5 -translate-y-1/2'
              onClick={() => props.onSearchChange('')}
            >
              <XIcon aria-hidden='true' className='size-3.5' />
            </Button>
          )}
        </div>
        <div className='grid grid-cols-2 gap-2'>
          <Select
            value={props.vendor}
            onValueChange={(value) =>
              value !== null && props.onVendorChange(value)
            }
          >
            <SelectTrigger
              className='bg-background w-full'
              aria-label={t('Filter by provider')}
            >
              <SelectValue>
                {props.vendor === ALL_FILTER_VALUE
                  ? t('All providers')
                  : props.vendor}
              </SelectValue>
            </SelectTrigger>
            <SelectContent align='start' alignItemWithTrigger={false}>
              <SelectGroup>
                <SelectItem value={ALL_FILTER_VALUE}>
                  {t('All providers')}
                </SelectItem>
                {props.vendors.map((vendor) => (
                  <SelectItem key={vendor} value={vendor}>
                    {vendor}
                  </SelectItem>
                ))}
              </SelectGroup>
            </SelectContent>
          </Select>
          <Select
            value={props.category}
            onValueChange={(value) =>
              value !== null && props.onCategoryChange(value)
            }
          >
            <SelectTrigger
              className='bg-background w-full'
              aria-label={t('Filter by model category')}
            >
              <SelectValue>
                {props.category === ALL_FILTER_VALUE
                  ? t('All model categories')
                  : getCategoryLabel(props.category, t)}
              </SelectValue>
            </SelectTrigger>
            <SelectContent align='start' alignItemWithTrigger={false}>
              <SelectGroup>
                <SelectItem value={ALL_FILTER_VALUE}>
                  {t('All model categories')}
                </SelectItem>
                {props.categories.map((category) => (
                  <SelectItem key={category} value={category}>
                    {getCategoryLabel(category, t)}
                  </SelectItem>
                ))}
              </SelectGroup>
            </SelectContent>
          </Select>
        </div>
        <div className='flex items-center justify-between gap-3'>
          <span className='text-muted-foreground text-xs'>
            {t('{{count}} models', { count: props.documents.length })}
          </span>
          {hasFilters && (
            <Button
              type='button'
              variant='ghost'
              size='xs'
              onClick={props.onClearFilters}
            >
              {t('Clear filters')}
            </Button>
          )}
        </div>
      </div>

      <ScrollArea className='h-[340px] min-h-0 lg:h-auto lg:flex-1'>
        <nav aria-label={t('Model documentation')} className='space-y-1.5 p-2'>
          {props.documents.map((document) => {
            const selected = props.selectedSlug === document.slug
            return (
              <button
                key={document.slug}
                type='button'
                data-testid={`model-doc-${document.slug}`}
                aria-pressed={selected}
                onClick={() => props.onSelect(document.slug)}
                className={cn(
                  'group relative w-full rounded-lg border px-3 py-3 text-left transition-colors',
                  selected
                    ? 'border-primary/25 bg-background text-foreground shadow-sm'
                    : 'border-transparent text-muted-foreground hover:border-border/70 hover:bg-background/70 hover:text-foreground'
                )}
              >
                {selected && (
                  <span
                    aria-hidden='true'
                    className='bg-primary absolute top-3 bottom-3 left-0 w-0.5 rounded-full'
                  />
                )}
                <span className='block truncate text-sm font-semibold'>
                  {document.title}
                </span>
                <span className='mt-1 block truncate font-mono text-[11px] opacity-70'>
                  {document.model}
                </span>
                <span className='mt-2 flex flex-wrap gap-1.5'>
                  <Badge variant='outline' className='text-[10px]'>
                    {document.vendor}
                  </Badge>
                  <Badge variant='secondary' className='text-[10px]'>
                    {getCategoryLabel(document.category, t)}
                  </Badge>
                </span>
              </button>
            )
          })}
          {props.documents.length === 0 && (
            <div className='px-4 py-10 text-center'>
              <p className='text-sm font-medium'>
                {t('No model documents found')}
              </p>
              <p className='text-muted-foreground mt-1 text-xs'>
                {t('Try another provider, category, or search term.')}
              </p>
            </div>
          )}
        </nav>
      </ScrollArea>
    </aside>
  )
}
