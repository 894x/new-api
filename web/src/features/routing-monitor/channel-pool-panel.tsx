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
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { ConfirmDialog } from '@/components/confirm-dialog'
import { EmptyState } from '@/components/empty-state'
import { ErrorState } from '@/components/error-state'
import { LoadingState } from '@/components/loading-state'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { NativeSelect, NativeSelectOption } from '@/components/ui/native-select'
import { ChannelStatusDialog } from '@/features/channels/components/dialogs/channel-status-dialog'
import { routingPoolKeys } from '@/features/model-routing/api'
import { toIntlLocale } from '@/i18n/languages'
import { canAccessUserGroupConfig } from '@/lib/admin-permissions'
import { formatNumber } from '@/lib/format'
import { handleServerError } from '@/lib/handle-server-error'
import { getServerErrorMessage } from '@/lib/server-error-message'
import { useAuthStore } from '@/stores/auth-store'

import {
  canEditDefaultPool,
  disableChannelInPools,
  listChannelRoutingPools,
  type ChannelPoolUpdate,
} from './channel-pool-actions'
import type { MonitorTarget } from './monitor-panel'
import { monitorTargetKey, type OverviewPool } from './pool-overview-data'

type PoolEntry = {
  target: MonitorTarget
  username?: string
  enabled: boolean
  inherited: boolean
  writable: boolean
}
type DisableScope = 'monitored' | 'all' | 'global'

export function ChannelPoolPanel(props: {
  channelId: number
  name: string
  pools: OverviewPool[]
  isDirty: (target: MonitorTarget) => boolean
  onSelect: (target: MonitorTarget) => void
  onClose: () => void
}) {
  const { t, i18n } = useTranslation()
  const locale = toIntlLocale(i18n.resolvedLanguage || i18n.language)
  const actor = useAuthStore((state) => state.auth.user)
  const client = useQueryClient()
  const [scope, setScope] = useState<DisableScope>('monitored')
  const [review, setReview] = useState<PoolEntry[] | null>(null)
  const [globalOpen, setGlobalOpen] = useState(false)
  const [failures, setFailures] = useState<ChannelPoolUpdate[]>([])
  const allPools = useQuery({
    queryKey: [...routingPoolKeys.all, 'channel-memberships', props.channelId],
    queryFn: () => listChannelRoutingPools(props.channelId),
    enabled: scope === 'all',
    staleTime: 0,
    retry: false,
    meta: { errorToast: false },
  })
  const monitored: PoolEntry[] = props.pools.flatMap((pool) => {
    const channels =
      pool.userConfig?.pool_channels ??
      pool.baseConfig?.pool?.channels ??
      pool.baseConfig?.candidates
        .filter((row) => row.available)
        .map((row) => ({ ...row, enabled: true })) ??
      []
    const channel = channels.find(
      (entry) => entry.channel_id === props.channelId
    )
    if (!channel) return []
    return [
      {
        target: pool.target,
        username: pool.username,
        enabled: channel.enabled,
        inherited:
          !!pool.userConfig &&
          (!pool.userConfig.configured || pool.userConfig.source !== 'user'),
        writable: pool.target.userId
          ? !!pool.userConfig &&
            canAccessUserGroupConfig(actor, pool.userConfig.user_role, 'write')
          : canEditDefaultPool(actor),
      },
    ]
  })
  const entries: PoolEntry[] =
    scope === 'all'
      ? (allPools.data ?? []).flatMap((pool) => {
          const channel = pool.channels.find(
            (row) => row.channel_id === props.channelId
          )
          if (!channel) return []
          return [
            {
              target: {
                userId: pool.user_id || undefined,
                group: pool.routing_group,
                model: pool.model,
              },
              username: pool.username,
              enabled: channel.enabled,
              inherited: false,
              writable: pool.user_id
                ? canAccessUserGroupConfig(actor, 0, 'write')
                : canEditDefaultPool(actor),
            },
          ]
        })
      : monitored
  const incomplete =
    scope === 'all'
      ? allPools.isPending || allPools.isError || allPools.isFetching
      : props.pools.some(
          (pool) => pool.configError || (!pool.baseConfig && !pool.userConfig)
        )
  const enabled = entries.filter((entry) => entry.enabled)
  const dirty = entries.some((entry) => props.isDirty(entry.target))
  const denied =
    scope === 'global'
      ? !canEditDefaultPool(actor)
      : enabled.some((entry) => !entry.writable)
  const blocked =
    dirty || denied || (scope !== 'global' && (incomplete || !enabled.length))
  const update = useMutation({
    mutationFn: (targets: MonitorTarget[]) =>
      disableChannelInPools(props.channelId, targets, props.isDirty),
    onSuccess: (results) => {
      const failed = results.filter((result) => result.error)
      setFailures(failed)
      setReview(null)
      if (failed.length) {
        toast.warning(
          t('{{count}} pools failed to update. Successful updates were kept.', {
            count: formatNumber(failed.length, locale),
          })
        )
      } else {
        toast.success(
          t('Channel disabled in {{count}} pools', {
            count: formatNumber(
              results.filter((result) => result.changed).length,
              locale
            ),
          })
        )
      }
    },
    onError: (error) => handleServerError(error, t('Failed to save')),
    onSettled: async () => {
      await Promise.all([
        client.invalidateQueries({ queryKey: routingPoolKeys.all }),
        client.invalidateQueries({ queryKey: ['user-routing-pools'] }),
        client.invalidateQueries({ queryKey: ['user-channel-routing'] }),
        client.invalidateQueries({ queryKey: ['user-channel-routing-models'] }),
        client.invalidateQueries({ queryKey: ['routing-monitor-pool'] }),
        client.invalidateQueries({ queryKey: ['routing-monitor'] }),
      ])
    },
  })
  return (
    <Card
      role='region'
      aria-label={t('Channel routing pools')}
      className='min-h-0 min-w-0 flex-1 gap-4 py-4'
    >
      <CardHeader className='shrink-0 gap-3 px-3'>
        <div className='flex min-w-0 items-start justify-between gap-2'>
          <div className='min-w-0'>
            <CardTitle className='truncate text-base' title={props.name}>
              {props.name}
            </CardTitle>
            <p className='text-muted-foreground mt-1 text-xs'>
              #{props.channelId} · {t('Pools using this channel')}
            </p>
          </div>
          <Button variant='ghost' size='xs' onClick={props.onClose}>
            {t('Back to pool editor')}
          </Button>
        </div>
        <NativeSelect
          aria-label={t('Disable scope')}
          value={scope}
          onChange={(event) => {
            setScope(event.target.value as DisableScope)
            setFailures([])
          }}
        >
          <NativeSelectOption value='monitored'>
            {t('Current monitored pools')}
          </NativeSelectOption>
          <NativeSelectOption value='all'>
            {t('All manageable pools')}
          </NativeSelectOption>
          <NativeSelectOption value='global'>
            {t('Global channel')}
          </NativeSelectOption>
        </NativeSelect>
        <Button
          variant='destructive'
          size='sm'
          disabled={blocked || update.isPending}
          onClick={() => {
            if (scope === 'global') setGlobalOpen(true)
            else setReview(enabled)
          }}
        >
          {scope === 'global'
            ? t('Disable channel globally')
            : t('Disable in all related pools')}
        </Button>
        <p className='text-muted-foreground text-xs'>
          {scope === 'global'
            ? t('Global disabling affects all routing through this channel.')
            : t(
                "Only this channel's pool membership will be disabled. Its global status is unchanged."
              )}
        </p>
        {dirty && (
          <p role='status' className='text-warning text-xs'>
            {t('Save or reset related pool drafts first.')}
          </p>
        )}
        {denied && (
          <p role='status' className='text-warning text-xs'>
            {t('No permission to perform this action')}
          </p>
        )}
        <p className='text-muted-foreground text-xs'>
          {t('{{count}} related pools · {{enabled}} enabled', {
            count: formatNumber(entries.length, locale),
            enabled: formatNumber(enabled.length, locale),
          })}
        </p>
      </CardHeader>
      <CardContent className='min-h-0 space-y-2 overflow-y-auto px-3 xl:flex-1'>
        {scope === 'all' && allPools.isPending && (
          <LoadingState message={t('Checking routing pools...')} />
        )}
        {scope === 'all' && allPools.isError && (
          <ErrorState
            title={t('Failed to load all routing pools')}
            onRetry={() => void allPools.refetch()}
          />
        )}
        {scope === 'monitored' && incomplete && (
          <p role='status' className='text-warning text-xs'>
            {t(
              'Some pool configurations could not be loaded. Refresh before disabling.'
            )}
          </p>
        )}
        {!incomplete && !entries.length && (
          <EmptyState
            title={t('No matching routing pool entries')}
            className='min-h-32'
          />
        )}
        {entries.map((entry) => {
          const identity =
            entry.username ||
            (entry.target.userId
              ? `#${entry.target.userId}`
              : t('Default configuration'))
          const label = `${identity} · ${entry.target.group} · ${entry.target.model}`
          return (
            <Button
              key={monitorTargetKey(entry.target)}
              variant='outline'
              className='h-auto w-full min-w-0 flex-col items-stretch gap-2 p-3 text-left'
              aria-label={t('View {{name}} pool', { name: label })}
              onClick={() => props.onSelect(entry.target)}
            >
              <span className='flex min-w-0 items-center justify-between gap-2'>
                <span className='min-w-0 truncate font-medium'>
                  {entry.target.model}
                </span>
                <Badge variant={entry.enabled ? 'default' : 'outline'}>
                  {entry.enabled ? t('Enabled') : t('Disabled')}
                </Badge>
              </span>
              <span className='text-muted-foreground truncate text-xs'>
                {identity} · {entry.target.group}
              </span>
              <span className='flex flex-wrap gap-1'>
                {entry.inherited && (
                  <Badge variant='outline'>{t('Inherited')}</Badge>
                )}
                {!entry.writable && (
                  <Badge variant='outline'>{t('Read only')}</Badge>
                )}
                {props.isDirty(entry.target) && (
                  <Badge variant='outline'>{t('Unsaved changes')}</Badge>
                )}
              </span>
            </Button>
          )
        })}
        {failures.map((result) => (
          <p
            key={monitorTargetKey(result.target)}
            role='alert'
            className='text-destructive text-xs'
          >
            {result.target.model} · {result.target.group}:{' '}
            {t(getServerErrorMessage(result.error))}
          </p>
        ))}
      </CardContent>
      <ConfirmDialog
        open={review !== null}
        onOpenChange={(open) => {
          if (!open && !update.isPending) setReview(null)
        }}
        title={t('Disable channel in {{count}} pools?', {
          count: formatNumber(review?.length ?? 0, locale),
        })}
        desc={t(
          "Only this channel's pool membership will be disabled. Its global status is unchanged."
        )}
        confirmText={t('Disable in reviewed pools')}
        destructive
        isLoading={update.isPending}
        disabled={
          blocked || !!review?.some((entry) => props.isDirty(entry.target))
        }
        handleConfirm={() => {
          if (review) update.mutate(review.map((entry) => entry.target))
        }}
      >
        <div className='max-h-60 space-y-2 overflow-y-auto text-xs'>
          <p>
            {props.name} · #{props.channelId}
          </p>
          {review?.map((entry) => (
            <p key={monitorTargetKey(entry.target)}>
              {entry.username ||
                (entry.target.userId
                  ? `#${entry.target.userId}`
                  : t('Default configuration'))}{' '}
              · {entry.target.group} · {entry.target.model}
            </p>
          ))}
          {review?.some((entry) => !entry.target.userId) && (
            <p className='text-warning'>
              {t(
                'Default-pool changes also affect users who inherit that pool.'
              )}
            </p>
          )}
          {review?.some((entry) => entry.inherited) && (
            <p className='text-muted-foreground'>
              {t(
                'User pools that inherit defaults will get a user override when needed.'
              )}
            </p>
          )}
        </div>
      </ConfirmDialog>
      {globalOpen && (
        <ChannelStatusDialog
          target={{ ids: [props.channelId] }}
          action='disable'
          allowPoolUpdates={false}
          onOpenChange={setGlobalOpen}
          onApplied={() => {
            void client.invalidateQueries({ queryKey: ['routing-monitor'] })
          }}
        />
      )}
    </Card>
  )
}
