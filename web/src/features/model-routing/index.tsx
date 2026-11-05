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
import { keepPreviousData, useQuery } from '@tanstack/react-query'
import { useDeferredValue, useEffect, useEffectEvent, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { EmptyState } from '@/components/empty-state'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { Combobox } from '@/components/ui/combobox'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { Skeleton } from '@/components/ui/skeleton'
import { FormNavigationGuard } from '@/features/system-settings/components/form-navigation-guard'
import { toIntlLocale } from '@/i18n/languages'
import {
  ADMIN_PERMISSION_ACTIONS,
  ADMIN_PERMISSION_RESOURCES,
  hasPermission,
} from '@/lib/admin-permissions'
import { formatNumber } from '@/lib/format'
import { ROLE } from '@/lib/roles'
import { useAuthStore } from '@/stores/auth-store'

import {
  getRoutingCatalog,
  listRoutingPools,
  routingPoolKeys,
  type PoolFilters,
} from './api'
import { CreatePoolDialog } from './create-pool-dialog'
import { PoolCard } from './pool-card'
import { RoutingWorkspaceDisplay } from './routing-workspace-display'

export function ModelRoutingWorkspace(props: {
  onDirtyChange: (dirty: boolean) => void
}) {
  const { t, i18n } = useTranslation()
  const locale = toIntlLocale(i18n.resolvedLanguage || i18n.language)
  const user = useAuthStore((state) => state.auth.user)
  const canRead =
    (user?.role ?? 0) >= ROLE.ADMIN &&
    hasPermission(
      user,
      ADMIN_PERMISSION_RESOURCES.CHANNEL,
      ADMIN_PERMISSION_ACTIONS.READ
    )
  const canWrite =
    canRead &&
    hasPermission(
      user,
      ADMIN_PERMISSION_RESOURCES.CHANNEL,
      ADMIN_PERMISSION_ACTIONS.WRITE
    )
  const [filters, setFilters] = useState<PoolFilters>({
    model: '',
    routing_group: '',
    mode: '',
    page: 1,
  })
  const [dirtyPools, setDirtyPools] = useState<Record<string, boolean>>({})
  const [creating, setCreating] = useState(false)
  const dirty = Object.values(dirtyPools).some(Boolean)
  const notifyDirty = useEffectEvent(props.onDirtyChange)
  useEffect(() => {
    notifyDirty(dirty)
  }, [dirty])
  useEffect(() => () => notifyDirty(false), [])
  const deferredModel = useDeferredValue(filters.model)
  const effectiveFilters = { ...filters, model: deferredModel }
  const strategyLabels = {
    '': t('All strategies'),
    priority: t('Priority routing'),
    smart: t('Smart routing'),
  }
  const catalog = useQuery({
    queryKey: routingPoolKeys.catalog(''),
    queryFn: () => getRoutingCatalog(),
    enabled: canRead,
    meta: { errorToast: false },
  })
  const pools = useQuery({
    queryKey: routingPoolKeys.list(effectiveFilters),
    queryFn: () => listRoutingPools(effectiveFilters),
    enabled: canRead,
    placeholderData: keepPreviousData,
    refetchOnWindowFocus: false,
    meta: { errorToast: false },
  })

  function updateFilters(patch: Partial<PoolFilters>) {
    if (!dirty) {
      setFilters((previous) => ({
        ...previous,
        ...patch,
        page: patch.page ?? 1,
      }))
    }
  }

  if (!canRead) {
    return (
      <Alert>
        <AlertDescription>
          {t("You don't have necessary permission")}
        </AlertDescription>
      </Alert>
    )
  }

  return (
    <div className='flex h-full min-h-0 min-w-0 flex-col gap-4'>
      <FormNavigationGuard when={dirty} />
      <p className='text-muted-foreground shrink-0 text-sm'>
        {t(
          'Each card overrides channel defaults for one model and public group. Customer-specific routing takes precedence.'
        )}
      </p>
      <RoutingWorkspaceDisplay
        toolbar={
          <>
            <Combobox
              options={[
                { value: '', label: t('All models') },
                ...(catalog.data?.models ?? []).map((value) => ({
                  value,
                  label: value,
                })),
              ]}
              value={filters.model}
              disabled={dirty}
              onValueChange={(value) => updateFilters({ model: value ?? '' })}
              placeholder={t('Search models')}
              aria-label={t('Search models')}
              className='h-8 min-w-40 flex-1 sm:max-w-72'
            />
            <Combobox
              options={[
                { value: '__all', label: t('All groups') },
                ...Object.entries(catalog.data?.groups ?? {}).map(
                  ([value, label]) => ({
                    value,
                    label: `${label || value} (${value})`,
                  })
                ),
              ]}
              value={filters.routing_group || '__all'}
              onValueChange={(value) =>
                updateFilters({
                  routing_group: value === '__all' ? '' : (value ?? ''),
                })
              }
              disabled={dirty}
              aria-label={t('Public group')}
              className='h-8 min-w-40 flex-1 sm:max-w-64'
            />
            <Select
              value={filters.mode || 'all'}
              onValueChange={(value) =>
                updateFilters({
                  mode: value === 'priority' || value === 'smart' ? value : '',
                })
              }
              disabled={dirty}
            >
              <SelectTrigger
                aria-label={t('Filter scheduling strategy')}
                className='w-full sm:w-44'
              >
                <SelectValue>{strategyLabels[filters.mode]}</SelectValue>
              </SelectTrigger>
              <SelectContent>
                <SelectItem value='all'>{t('All strategies')}</SelectItem>
                <SelectItem value='priority'>
                  {t('Priority routing')}
                </SelectItem>
                <SelectItem value='smart'>{t('Smart routing')}</SelectItem>
              </SelectContent>
            </Select>
            <Button
              type='button'
              className='sm:ml-auto'
              disabled={!canWrite || dirty}
              onClick={() => setCreating(true)}
            >
              {t('Configure routing pool')}
            </Button>
          </>
        }
        refreshAction={
          <Button
            type='button'
            variant='outline'
            disabled={dirty || pools.isFetching}
            onClick={() => {
              void pools.refetch()
              void catalog.refetch()
            }}
          >
            {t('Refresh')}
          </Button>
        }
      >
        {catalog.isError && (
          <Alert>
            <AlertDescription>
              {t('Failed to load group options.')}{' '}
              <Button
                type='button'
                variant='link'
                onClick={() => void catalog.refetch()}
              >
                {t('Retry')}
              </Button>
            </AlertDescription>
          </Alert>
        )}
        <div className='min-h-0 flex-1 overflow-y-auto overscroll-contain'>
          {pools.isPending && <Skeleton className='h-64 w-full' />}
          {pools.isError && (
            <Alert variant='destructive'>
              <AlertDescription>
                {t('Failed to load routing pools.')}{' '}
                <Button
                  type='button'
                  variant='outline'
                  onClick={() => void pools.refetch()}
                >
                  {t('Retry')}
                </Button>
              </AlertDescription>
            </Alert>
          )}
          {pools.data &&
            !pools.isError &&
            (pools.data.items.length === 0 ? (
              <EmptyState
                title={t('No configured routing pools')}
                description={t(
                  'Requests inherit channel defaults until a model and public group pool is configured.'
                )}
                action={
                  <Button
                    type='button'
                    disabled={!canWrite}
                    onClick={() => setCreating(true)}
                  >
                    {t('Configure routing pool')}
                  </Button>
                }
              />
            ) : (
              <div className='grid min-w-0 grid-cols-[repeat(auto-fill,minmax(min(100%,21rem),1fr))] items-start gap-4'>
                {pools.data.items.map((pool) => {
                  const key = `${pool.routing_group}\u0000${pool.model}`
                  return (
                    <PoolCard
                      key={key}
                      pool={pool}
                      canWrite={canWrite && !pools.isPlaceholderData}
                      groupLabel={catalog.data?.groups[pool.routing_group]}
                      onDirtyChange={(value) => {
                        setDirtyPools((previous) => {
                          if (!!previous[key] === value) return previous
                          const next = { ...previous, [key]: value }
                          return next
                        })
                      }}
                    />
                  )
                })}
              </div>
            ))}
        </div>
        {pools.data && (
          <div className='flex min-w-0 shrink-0 items-center justify-between gap-3'>
            <div className='flex min-w-0 flex-1 items-center gap-3 text-xs'>
              <span className='text-muted-foreground shrink-0'>
                {t('Routing pools: {{total}}', {
                  total: formatNumber(pools.data.total, locale),
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
                type='button'
                variant='outline'
                size='sm'
                disabled={dirty || pools.isFetching || filters.page <= 1}
                onClick={() => updateFilters({ page: filters.page - 1 })}
              >
                {t('Previous')}
              </Button>
              <span className='text-xs'>
                {formatNumber(filters.page, locale)}
              </span>
              <Button
                type='button'
                variant='outline'
                size='sm'
                disabled={
                  dirty ||
                  pools.isFetching ||
                  filters.page * pools.data.page_size >= pools.data.total
                }
                onClick={() => updateFilters({ page: filters.page + 1 })}
              >
                {t('Next')}
              </Button>
            </div>
          </div>
        )}
        <CreatePoolDialog
          open={creating}
          onOpenChange={setCreating}
          groups={catalog.data?.groups ?? {}}
          canWrite={canWrite}
          onCreated={(pool) =>
            updateFilters({
              model: pool.model,
              routing_group: pool.routing_group,
              mode: '',
              page: 1,
            })
          }
        />
      </RoutingWorkspaceDisplay>
    </div>
  )
}
