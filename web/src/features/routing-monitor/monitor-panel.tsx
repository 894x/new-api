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
import { Exchange01Icon } from '@hugeicons/core-free-icons'
import { HugeiconsIcon } from '@hugeicons/react'
import { useQuery } from '@tanstack/react-query'
import { lazy, Suspense, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { EmptyState } from '@/components/empty-state'
import { ErrorState } from '@/components/error-state'
import { LoadingState } from '@/components/loading-state'
import { ModelGroupSelector } from '@/components/model-group-selector'
import { Button } from '@/components/ui/button'
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from '@/components/ui/tooltip'
import {
  getRoutingCatalog,
  getUserRoutingPool,
  routingPoolKeys,
  userRoutingPoolKeys,
} from '@/features/model-routing/api'
import { RoutingScopeSelect } from '@/features/model-routing/scope-select'
import {
  hasPermission,
  ADMIN_PERMISSION_RESOURCES,
  ADMIN_PERMISSION_ACTIONS,
} from '@/lib/admin-permissions'
import { cn } from '@/lib/utils'
import { useAuthStore } from '@/stores/auth-store'

const RoutingMonitorWorkspace = lazy(() => import('./index'))

export type MonitorTarget = {
  userId?: number
  group: string
  model: string
}

export function MonitorPanel(props: {
  number: number
  initialUserId?: number
  dense?: boolean
  target?: MonitorTarget
  onChange: (target?: MonitorTarget) => void
}) {
  const { t } = useTranslation()
  const actor = useAuthStore((state) => state.auth.user)
  const canReadDefaults = hasPermission(
    actor,
    ADMIN_PERMISSION_RESOURCES.CHANNEL,
    ADMIN_PERMISSION_ACTIONS.READ
  )
  const [userId, setUserId] = useState(props.initialUserId)
  const [group, setGroup] = useState('')
  const [model, setModel] = useState('')
  const [headerContainer, setHeaderContainer] = useState<HTMLElement | null>(
    null
  )
  const selecting = !props.target
  const catalog = useQuery({
    queryKey: routingPoolKeys.catalog(''),
    queryFn: () => getRoutingCatalog(),
    enabled: selecting && !userId && canReadDefaults,
    staleTime: 30000,
    meta: { errorToast: false },
  })
  const userCatalog = useQuery({
    queryKey: userRoutingPoolKeys.catalog(userId ?? 0),
    queryFn: () => getUserRoutingPool(userId ?? 0),
    enabled: selecting && !!userId,
    staleTime: 30000,
    meta: { errorToast: false },
  })
  const groupCatalog = useQuery({
    queryKey: routingPoolKeys.catalog(group),
    queryFn: () => getRoutingCatalog(group),
    enabled: selecting && !userId && canReadDefaults && !!group,
    staleTime: 30000,
    meta: { errorToast: false },
  })
  const groups = userId ? userCatalog.data?.key_groups : catalog.data?.groups
  let models = userId ? userCatalog.data?.models : groupCatalog.data?.models
  if (userId && userCatalog.data?.model_groups) {
    const modelGroups = userCatalog.data.model_groups
    models = models?.filter((name) => modelGroups[name]?.includes(group))
  }
  const validSelection =
    !!group &&
    !!model &&
    !!groups &&
    Object.hasOwn(groups, group) &&
    !!models?.includes(model)
  const failed = userId
    ? userCatalog.isError
    : catalog.isError || groupCatalog.isError
  const loading = userId
    ? userCatalog.isFetching
    : catalog.isFetching || groupCatalog.isFetching
  const target = props.target

  return (
    <section
      aria-label={t('Monitor panel {{number}}', { number: props.number })}
      className='bg-card @container/monitor-panel flex min-h-0 min-w-0 flex-col overflow-hidden rounded-xl border [&_*]:[scrollbar-width:none] [&_*::-webkit-scrollbar]:hidden'
    >
      <header className='flex min-w-0 shrink-0 items-center gap-1 border-b px-2 py-2 @min-[20rem]/monitor-panel:gap-2 @min-[20rem]/monitor-panel:px-3'>
        <div className='flex min-w-0 flex-1 items-center gap-1.5 overflow-hidden'>
          <span className='text-muted-foreground shrink-0 text-xs'>
            {target
              ? `#${props.number}`
              : t('Monitor panel {{number}}', { number: props.number })}
          </span>
          {target && (
            <p
              className='truncate text-sm font-medium'
              title={`${target.model} · ${target.group}`}
            >
              {target.model} · {target.group}
            </p>
          )}
        </div>
        {target && (
          <div
            ref={setHeaderContainer}
            data-monitor-panel-controls=''
            className='min-w-0 shrink-0'
          />
        )}
        {target && (
          <Tooltip>
            <TooltipTrigger
              render={
                <Button
                  variant='ghost'
                  size='icon-xs'
                  aria-label={t('Change monitoring pool')}
                  onClick={() => props.onChange()}
                />
              }
            >
              <HugeiconsIcon icon={Exchange01Icon} aria-hidden='true' />
            </TooltipTrigger>
            <TooltipContent>{t('Change monitoring pool')}</TooltipContent>
          </Tooltip>
        )}
      </header>
      {target ? (
        <Suspense
          fallback={
            <LoadingState message={t('Loading routing observations...')} />
          }
        >
          <RoutingMonitorWorkspace
            key={`${target.userId ?? 'base'}-${target.group}-${target.model}`}
            userId={target.userId}
            fixedGroup={target.group}
            fixedModel={target.model}
            headerContainer={headerContainer}
            panel
          />
        </Suspense>
      ) : (
        <div className='flex min-h-0 flex-1 items-center justify-center-safe overflow-y-auto p-2'>
          <EmptyState
            className={cn(
              'min-h-0 w-full gap-3 p-2',
              props.dense && 'gap-2 [&_[data-slot=empty-icon]]:hidden'
            )}
            title={t('Select a monitoring pool')}
            description={
              props.dense
                ? undefined
                : t('Choose a user, group and model for this panel.')
            }
            action={
              <div className='flex w-full max-w-64 min-w-0 flex-col gap-2 text-left'>
                <RoutingScopeSelect
                  userId={userId}
                  allowAllUsers={false}
                  disabled={false}
                  onChange={(next) => {
                    setUserId(next)
                    setGroup('')
                    setModel('')
                  }}
                />
                <ModelGroupSelector
                  className='w-full'
                  selectedGroup={group}
                  groups={Object.entries(groups ?? {}).map(
                    ([value, label]) => ({ value, label: label || value })
                  )}
                  onGroupChange={(value) => {
                    setGroup(value)
                    setModel('')
                  }}
                  selectedModel={model}
                  models={(models ?? []).map((value) => ({
                    value,
                    label: value,
                  }))}
                  onModelChange={setModel}
                />
                {failed && (
                  <ErrorState
                    className='min-h-0 p-2'
                    title={t('Failed to load routing pools.')}
                    onRetry={() => {
                      if (userId) void userCatalog.refetch()
                      else {
                        void catalog.refetch()
                        if (group) void groupCatalog.refetch()
                      }
                    }}
                  />
                )}
                <Button
                  aria-busy={loading}
                  disabled={!validSelection || failed || loading}
                  onClick={() => {
                    if (validSelection && !failed && !loading) {
                      props.onChange({ userId, group, model })
                    }
                  }}
                >
                  {t('Start monitoring')}
                </Button>
              </div>
            }
          />
        </div>
      )}
    </section>
  )
}
