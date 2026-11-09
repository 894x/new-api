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
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { ConfirmDialog } from '@/components/confirm-dialog'
import {
  StaticDataTable,
  type StaticDataTableColumn,
} from '@/components/data-table'
import { EmptyState } from '@/components/empty-state'
import { ErrorState } from '@/components/error-state'
import { LoadingState } from '@/components/loading-state'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { routingPoolKeys } from '@/features/model-routing/api'
import { toIntlLocale } from '@/i18n/languages'
import {
  ADMIN_PERMISSION_ACTIONS,
  ADMIN_PERMISSION_RESOURCES,
  hasPermission,
} from '@/lib/admin-permissions'
import { formatNumber } from '@/lib/format'
import { handleServerError } from '@/lib/handle-server-error'
import {
  getServerErrorMessage,
  requireServerSuccess,
} from '@/lib/server-error-message'
import { useAuthStore } from '@/stores/auth-store'

import {
  batchUpdateChannelStatus,
  getChannelStatusImpact,
  type ChannelStatusRoutingPool,
  type ChannelStatusAction,
  type ChannelStatusTarget,
} from '../../api'
import { CHANNEL_STATUS } from '../../constants'
import { channelsQueryKeys } from '../../lib/channel-actions'

type ChannelStatusDialogProps = {
  target: ChannelStatusTarget
  action?: ChannelStatusAction
  onOpenChange: (open: boolean) => void
  onApplied?: () => void
}

export function ChannelStatusDialog(props: ChannelStatusDialogProps) {
  const { t, i18n } = useTranslation()
  const locale = toIntlLocale(i18n.resolvedLanguage || i18n.language)
  const queryClient = useQueryClient()
  const action = props.action ?? 'disable'
  const enabling = action === 'enable'
  const currentUser = useAuthStore((state) => state.auth.user)
  const canDisablePools =
    hasPermission(
      currentUser,
      ADMIN_PERMISSION_RESOURCES.CHANNEL,
      ADMIN_PERMISSION_ACTIONS.WRITE
    ) &&
    hasPermission(
      currentUser,
      ADMIN_PERMISSION_RESOURCES.USER_GROUP_CONFIG,
      ADMIN_PERMISSION_ACTIONS.READ
    ) &&
    hasPermission(
      currentUser,
      ADMIN_PERMISSION_RESOURCES.USER_GROUP_CONFIG,
      ADMIN_PERMISSION_ACTIONS.WRITE
    )
  const impact = useQuery({
    queryKey: [...channelsQueryKeys.all, 'status-impact', action, props.target],
    queryFn: ({ signal }) =>
      getChannelStatusImpact(props.target, action, signal),
    staleTime: 0,
    refetchOnMount: 'always',
    refetchOnWindowFocus: false,
    retry: false,
    meta: { errorToast: false },
  })
  const mutation = useMutation({
    mutationFn: async (updateRoutingPools: boolean) => {
      // Apply to exactly the channels reviewed in this snapshot, including when
      // the selection originated from a tag whose membership could change.
      if (!impact.data || impact.isFetching || impact.isError) return null
      const ids = impact.data.channels.map((channel) => channel.id)
      if (ids.length === 0) return null
      const response = await batchUpdateChannelStatus(
        ids,
        enabling ? CHANNEL_STATUS.ENABLED : CHANNEL_STATUS.MANUAL_DISABLED,
        updateRoutingPools
      )
      requireServerSuccess(response)
      return response.data ?? 0
    },
    onError: (error) =>
      handleServerError(
        error,
        enabling
          ? t('Failed to enable channels')
          : t('Failed to disable channels')
      ),
    onSuccess: (count, updateRoutingPools) => {
      if (count === null) return
      if (updateRoutingPools) {
        toast.success(
          enabling
            ? t('Channels and pool entries enabled')
            : t('Channels and pool entries disabled')
        )
      } else {
        toast.success(
          enabling
            ? t('{{count}} channel(s) enabled', { count })
            : t('{{count}} channel(s) disabled', { count })
        )
      }
      void queryClient.invalidateQueries({
        queryKey: channelsQueryKeys.lists(),
      })
      void queryClient.invalidateQueries({ queryKey: routingPoolKeys.all })
      void queryClient.invalidateQueries({ queryKey: ['user-routing-pools'] })
      void queryClient.invalidateQueries({ queryKey: ['user-channel-routing'] })
      void queryClient.invalidateQueries({
        queryKey: ['user-channel-routing-models'],
      })
      props.onApplied?.()
      props.onOpenChange(false)
    },
  })
  const columns: StaticDataTableColumn<ChannelStatusRoutingPool>[] = [
    {
      id: 'customer',
      header: t('Routing pool'),
      className: 'w-52',
      cell: (row) =>
        row.user_id > 0 ? (
          <div className='space-y-1'>
            <div>{`${row.username} (#${row.user_id})`}</div>
            <Badge variant='outline'>
              {row.source === 'legacy_user'
                ? t('Legacy user override')
                : t('User override')}
            </Badge>
          </div>
        ) : (
          t('Default channel pool')
        ),
    },
    {
      id: 'channel',
      header: t('Channel'),
      className: 'w-44',
      cell: (row) => {
        const channel = impact.data?.channels.find(
          (item) => item.id === row.channel_id
        )
        return `#${row.channel_id} ${channel?.name ?? ''}`
      },
    },
    {
      id: 'model',
      header: t('Model'),
      className: 'w-48',
      cell: (row) => row.model,
    },
    {
      id: 'group',
      header: t('Group'),
      className: 'w-44',
      cell: (row) => row.routing_group || t('All groups'),
    },
    {
      id: 'priority',
      header: t('Priority'),
      className: 'w-32',
      cell: (row) =>
        row.priority === null
          ? t('Inherit')
          : formatNumber(row.priority, locale),
    },
    {
      id: 'status',
      header: t('Pool status'),
      className: 'w-28',
      cell: (row) => (
        <Badge variant={row.enabled ? 'default' : 'secondary'}>
          {row.enabled ? t('Enabled') : t('Disabled')}
        </Badge>
      ),
    },
  ]

  return (
    <ConfirmDialog
      open
      onOpenChange={(open) => {
        if (!mutation.isPending) props.onOpenChange(open)
      }}
      title={enabling ? t('Enable channels?') : t('Disable channels?')}
      desc={
        enabling
          ? t(
              'Review default pools and user overrides before enabling these channels.'
            )
          : t(
              'Review default pools and user overrides before disabling these channels.'
            )
      }
      confirmText={enabling ? t('Confirm enable') : t('Confirm disable')}
      destructive={!enabling}
      disabled={
        impact.isFetching || impact.isError || !impact.data?.channels.length
      }
      isLoading={mutation.isPending}
      handleConfirm={() => mutation.mutate(false)}
      additionalActions={
        canDisablePools && (
          <Button
            variant={enabling ? 'default' : 'destructive'}
            className='h-auto min-h-8 min-w-0 shrink whitespace-normal'
            disabled={
              mutation.isPending ||
              impact.isFetching ||
              impact.isError ||
              !impact.data?.channels.length
            }
            onClick={() => mutation.mutate(true)}
          >
            {enabling
              ? t('Enable channel and pool entries')
              : t('Disable channel and pool entries')}
          </Button>
        )
      }
      className='max-h-[90dvh] grid-rows-[auto_minmax(0,1fr)_auto] overflow-hidden data-[size=default]:max-w-[calc(100%-2rem)] data-[size=default]:sm:max-w-5xl'
    >
      <div
        className='min-h-0 space-y-3 overflow-y-auto'
        aria-busy={impact.isFetching}
      >
        {impact.isPending && (
          <LoadingState message={t('Checking routing pools...')} />
        )}
        {impact.isError && (
          <ErrorState
            title={t('Failed to check routing pools')}
            description={t(getServerErrorMessage(impact.error))}
            onRetry={() => void impact.refetch()}
            className='min-h-40'
          />
        )}
        {impact.data && !impact.isError && (
          <>
            <p className='text-sm font-medium'>
              {enabling
                ? t('{{channels}} channel(s), {{entries}} pool entries', {
                    channels: formatNumber(impact.data.channels.length, locale),
                    entries: formatNumber(
                      impact.data.routing_pools.length,
                      locale
                    ),
                  })
                : t(
                    '{{channels}} channel(s), {{customers}} affected customer(s)',
                    {
                      channels: formatNumber(
                        impact.data.channels.length,
                        locale
                      ),
                      customers: formatNumber(
                        impact.data.customer_count,
                        locale
                      ),
                    }
                  )}
            </p>
            <p className='text-muted-foreground text-xs'>
              {t(
                'Default pools are shown once per model and group. User overrides are listed separately, including customers without active API keys.'
              )}
            </p>
            <p className='text-muted-foreground text-xs'>
              {enabling
                ? t(
                    'Enabling pool entries preserves priorities and weights and restores existing memberships only.'
                  )
                : t(
                    'Disabling pool entries preserves their priorities and weights. Enabling the channel later will not enable these entries.'
                  )}
            </p>
            <div className='flex flex-wrap gap-2'>
              {impact.data.channels.map((channel) => (
                <Badge
                  key={channel.id}
                  variant='outline'
                  className='max-w-full break-all whitespace-normal'
                >
                  #{channel.id} {channel.name}
                </Badge>
              ))}
            </div>
            {impact.data.channels.length === 0 ? (
              <EmptyState
                title={
                  enabling
                    ? t('No channels in this selection')
                    : t('No enabled channels in this selection')
                }
                className='min-h-40'
              />
            ) : (
              <StaticDataTable
                className='overflow-x-auto'
                columns={columns}
                data={impact.data.routing_pools}
                getRowKey={(row) =>
                  JSON.stringify([
                    row.source,
                    row.user_id,
                    row.channel_id,
                    row.model,
                    row.routing_group,
                  ])
                }
                tableClassName='min-w-[60rem] table-fixed'
                tableProps={{ withContainer: false }}
                emptyContent={t('No matching routing pool entries')}
                containerProps={{
                  role: 'region',
                  'aria-label': t('Affected routing pools'),
                  tabIndex: 0,
                }}
              />
            )}
          </>
        )}
      </div>
    </ConfirmDialog>
  )
}
