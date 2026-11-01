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
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { useEffect, useEffectEvent, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { EmptyState } from '@/components/empty-state'
import { ErrorState } from '@/components/error-state'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Combobox } from '@/components/ui/combobox'
import { Skeleton } from '@/components/ui/skeleton'
import { FormNavigationGuard } from '@/features/system-settings/components/form-navigation-guard'
import { toIntlLocale } from '@/i18n/languages'
import { formatNumber } from '@/lib/format'

import { getUserRoutingPool, userRoutingPoolKeys } from './api'
import { UserPoolCard } from './user-pool-card'

export function UserRoutingWorkspace(props: {
  userId: number
  onDirtyChange: (dirty: boolean) => void
}) {
  const { t, i18n } = useTranslation()
  const client = useQueryClient()
  const locale = toIntlLocale(i18n.resolvedLanguage || i18n.language)
  const [model, setModel] = useState('')
  const [group, setGroup] = useState('__all')
  const [source, setSource] = useState('__all')
  const [page, setPage] = useState(1)
  const [dirtyCards, setDirtyCards] = useState<Record<string, boolean>>({})
  const dirty = Object.values(dirtyCards).some(Boolean)
  const notifyDirty = useEffectEvent(props.onDirtyChange)
  useEffect(() => {
    notifyDirty(dirty)
  }, [dirty])
  useEffect(() => () => notifyDirty(false), [])
  const catalog = useQuery({
    queryKey: userRoutingPoolKeys.catalog(props.userId),
    queryFn: () => getUserRoutingPool(props.userId),
    enabled: !dirty,
    refetchOnWindowFocus: false,
    meta: { errorToast: false },
  })
  const data = catalog.data
  const entries = Object.entries(data?.model_groups ?? {}).flatMap(
    ([name, groups]) =>
      groups.map((key) => ({
        model: name,
        group: key,
        configured: data?.configured_key_groups?.[name]?.includes(key) ?? false,
      }))
  )
  entries.sort(
    (a, b) =>
      Number(b.configured) - Number(a.configured) ||
      a.model.localeCompare(b.model) ||
      a.group.localeCompare(b.group)
  )
  const filtered = entries.filter(
    (entry) =>
      (!model || entry.model === model) &&
      (group === '__all' || entry.group === group) &&
      (source === '__all' || entry.configured === (source === 'user'))
  )
  const pageSize = 12
  const lastPage = Math.max(1, Math.ceil(filtered.length / pageSize))
  const currentPage = Math.min(page, lastPage)
  return (
    <div className='flex h-full min-h-0 min-w-0 flex-col gap-4'>
      <FormNavigationGuard when={dirty} />
      <div className='flex shrink-0 flex-wrap items-center gap-2 rounded-lg border p-3'>
        <Badge>{t('User routing pools')}</Badge>
        <span className='min-w-0 font-medium break-all'>
          {data?.username ?? t('User #{{id}}', { id: props.userId })}
        </span>
        <span className='text-muted-foreground text-sm'>#{props.userId}</span>
        <span className='text-muted-foreground text-xs'>
          {t(
            'User pools define the full channel list. Empty user pools follow the default routing pool.'
          )}
        </span>
      </div>
      <div className='flex shrink-0 flex-wrap items-center gap-2'>
        <Combobox
          className='h-8 min-w-40 flex-1 sm:max-w-72'
          aria-label={t('Search models')}
          placeholder={t('Search models')}
          value={model}
          disabled={dirty}
          options={[
            { value: '', label: t('All models') },
            ...[...new Set(entries.map((entry) => entry.model))].map(
              (value) => ({ value, label: value })
            ),
          ]}
          onValueChange={(value) => {
            setModel(value ?? '')
            setPage(1)
          }}
        />
        <Combobox
          className='h-8 min-w-40 flex-1 sm:max-w-64'
          aria-label={t('Public group')}
          value={group}
          disabled={dirty}
          options={[
            { value: '__all', label: t('All groups') },
            ...Object.entries(data?.key_groups ?? {}).map(([value, label]) => ({
              value,
              label: `${label || value} (${value})`,
            })),
          ]}
          onValueChange={(value) => {
            setGroup(value ?? '__all')
            setPage(1)
          }}
        />
        <Combobox
          className='h-8 min-w-40 flex-1 sm:max-w-52'
          aria-label={t('Configuration source')}
          value={source}
          disabled={dirty}
          options={[
            { value: '__all', label: t('All configurations') },
            { value: 'user', label: t('User override') },
            { value: 'inherited', label: t('Inherited') },
          ]}
          onValueChange={(value) => {
            setSource(value ?? '__all')
            setPage(1)
          }}
        />
        <Button
          variant='outline'
          className='sm:ml-auto'
          disabled={dirty || catalog.isFetching}
          onClick={() =>
            void client.invalidateQueries({
              queryKey: userRoutingPoolKeys.all(props.userId),
            })
          }
        >
          {t('Refresh')}
        </Button>
      </div>
      <div className='min-h-0 flex-1 overflow-y-auto overscroll-contain'>
        {catalog.isPending && <Skeleton className='h-64 w-full' />}
        {catalog.isError && (
          <ErrorState onRetry={() => void catalog.refetch()} />
        )}
        {data &&
          !catalog.isError &&
          (filtered.length === 0 ? (
            <EmptyState title={t('No available routing pools')} />
          ) : (
            <div
              role='list'
              aria-label={t('User routing pools')}
              className='min-w-0 columns-[24rem] gap-4'
            >
              {filtered
                .slice((currentPage - 1) * pageSize, currentPage * pageSize)
                .map((entry) => {
                  const key = `${entry.model}\u0000${entry.group}`
                  return (
                    <div
                      key={key}
                      role='listitem'
                      className='mb-4 inline-block w-full break-inside-avoid align-top'
                    >
                      <UserPoolCard
                        userId={props.userId}
                        model={entry.model}
                        group={entry.group}
                        groupLabel={entry.group}
                        onDirtyChange={(value) =>
                          setDirtyCards((old) =>
                            old[key] === value ? old : { ...old, [key]: value }
                          )
                        }
                      />
                    </div>
                  )
                })}
            </div>
          ))}
      </div>
      {data && !catalog.isError && (
        <div className='flex min-w-0 shrink-0 items-center justify-between gap-3 text-xs'>
          <div className='flex min-w-0 flex-1 items-center gap-3'>
            <span className='text-muted-foreground shrink-0'>
              {t('Routing pools: {{total}}', {
                total: formatNumber(filtered.length, locale),
              })}
            </span>
            {dirty && (
              <p
                role='status'
                className='text-muted-foreground truncate'
                title={t(
                  'Save or reset edited cards before changing filters or views.'
                )}
              >
                {t(
                  'Save or reset edited cards before changing filters or views.'
                )}
              </p>
            )}
          </div>
          <div className='flex shrink-0 items-center gap-2'>
            <Button
              size='sm'
              variant='outline'
              disabled={dirty || currentPage <= 1}
              onClick={() => setPage(currentPage - 1)}
            >
              {t('Previous')}
            </Button>
            <span>{formatNumber(currentPage, locale)}</span>
            <Button
              size='sm'
              variant='outline'
              disabled={dirty || currentPage >= lastPage}
              onClick={() => setPage(currentPage + 1)}
            >
              {t('Next')}
            </Button>
          </div>
        </div>
      )}
    </div>
  )
}
