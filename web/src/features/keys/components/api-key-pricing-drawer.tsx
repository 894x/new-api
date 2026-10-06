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
import { RefreshCw } from 'lucide-react'
import { useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'

import {
  sideDrawerContentClassName,
  sideDrawerFormClassName,
  sideDrawerHeaderClassName,
} from '@/components/drawer-layout'
import { EmptyState } from '@/components/empty-state'
import { ErrorState } from '@/components/error-state'
import { GroupBadge } from '@/components/group-badge'
import { LoadingState } from '@/components/loading-state'
import { Accordion } from '@/components/ui/accordion'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import {
  Sheet,
  SheetContent,
  SheetDescription,
  SheetHeader,
  SheetTitle,
} from '@/components/ui/sheet'
import { toIntlLocale } from '@/i18n/languages'
import { requireServerSuccess } from '@/lib/server-error-message'
import { useAuthStore } from '@/stores/auth-store'

import { getApiKey } from '../api'
import { useApiKeyPricingData } from '../hooks/use-api-key-pricing'
import { getApiKeyModelPrices } from '../lib/api-key-pricing'
import type { ApiKey } from '../types'
import { ApiKeyModelPriceItem } from './api-key-model-price-item'

type ApiKeyPricingDrawerProps = {
  apiKey: ApiKey
  open: boolean
  onOpenChange: (open: boolean) => void
}

export function ApiKeyPricingDrawer(props: ApiKeyPricingDrawerProps) {
  const { t, i18n } = useTranslation()
  const locale = toIntlLocale(i18n.resolvedLanguage || i18n.language)
  // Ratios can be smaller than 0.01; ordinary number formatting would round them to zero.
  const ratioFormatter = useMemo(
    () => new Intl.NumberFormat(locale, { maximumFractionDigits: 20 }),
    [locale]
  )
  const userId = useAuthStore((state) => state.auth.user?.id)
  const pricing = useApiKeyPricingData(props.open)
  const keyQuery = useQuery({
    queryKey: ['api-key-pricing-token', userId, props.apiKey.id],
    queryFn: async () => requireServerSuccess(await getApiKey(props.apiKey.id)),
    enabled: props.open && userId !== undefined,
    staleTime: 0,
  })
  const [search, setSearch] = useState('')
  const [expanded, setExpanded] = useState<string[] | undefined>()
  const apiKey = keyQuery.data?.data
  const entries = useMemo(
    () => (apiKey ? getApiKeyModelPrices(apiKey, pricing) : []),
    [apiKey, pricing]
  )
  const visibleEntries = entries.filter((entry) =>
    entry.model.model_name.toLowerCase().includes(search.trim().toLowerCase())
  )
  const refresh = async () =>
    Promise.all([pricing.refetch(), keyQuery.refetch()])
  const isLoading = pricing.isLoading || keyQuery.isLoading
  const error =
    pricing.error || keyQuery.error || (!keyQuery.isLoading && !apiKey)
  const group = apiKey?.group || pricing.currentGroup

  return (
    <Sheet open={props.open} onOpenChange={props.onOpenChange}>
      <SheetContent className={sideDrawerContentClassName('sm:max-w-3xl')}>
        <SheetHeader className={sideDrawerHeaderClassName('pr-12')}>
          <SheetTitle>{t('Model prices and discounts')}</SheetTitle>
          <SheetDescription className='break-all'>
            {t('API Key')}: {apiKey?.name || props.apiKey.name}
          </SheetDescription>
        </SheetHeader>
        <div className={sideDrawerFormClassName('gap-4')}>
          <div className='flex items-center justify-between gap-3'>
            <div className='flex min-w-0 flex-wrap items-center gap-2'>
              <span className='text-muted-foreground text-sm'>
                {t('Billing group')}
              </span>
              {!isLoading && !error && <GroupBadge group={group} />}
            </div>
            <Button
              variant='outline'
              size='sm'
              onClick={refresh}
              disabled={pricing.isFetching || keyQuery.isFetching}
            >
              <RefreshCw className='size-4' aria-hidden='true' />
              {t('Refresh')}
            </Button>
          </div>
          {group === 'auto' && !isLoading && !error && (
            <p className='text-muted-foreground text-xs'>
              {t(
                'Auto keys use the price of the group selected for each request.'
              )}
            </p>
          )}
          {isLoading && <LoadingState />}
          {!isLoading && error && (
            <ErrorState
              title={t('Unable to load model prices')}
              onRetry={refresh}
            />
          )}
          {!isLoading && !error && (
            <>
              <Input
                aria-label={t('Search models')}
                placeholder={t('Search models')}
                value={search}
                onChange={(event) => setSearch(event.target.value)}
              />
              <p className='text-muted-foreground text-xs'>
                {t(
                  'Model-specific ratios replace the regular group ratio. They are not multiplied together.'
                )}
              </p>
              {visibleEntries.length === 0 ? (
                <EmptyState
                  title={t('No models available for this key')}
                  description={t(
                    'Check the billing group, model restrictions, or search terms.'
                  )}
                />
              ) : (
                <Accordion
                  value={expanded ?? [visibleEntries[0].model.model_name]}
                  onValueChange={(value) => setExpanded(value as string[])}
                >
                  {visibleEntries.map((entry) => (
                    <ApiKeyModelPriceItem
                      key={entry.model.model_name}
                      entry={entry}
                      pricing={pricing}
                      ratioFormatter={ratioFormatter}
                    />
                  ))}
                </Accordion>
              )}
              <p className='text-muted-foreground text-xs'>
                {t(
                  'Prices use current model and group rates. Final charges depend on actual usage and applicable billing rules.'
                )}
              </p>
            </>
          )}
        </div>
      </SheetContent>
    </Sheet>
  )
}
