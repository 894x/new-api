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
import { toIntlLocale } from '@/i18n/languages'
import { formatNumber } from '@/lib/format'
import { handleServerError } from '@/lib/handle-server-error'
import {
  getServerErrorMessage,
  requireServerSuccess,
} from '@/lib/server-error-message'

import {
  batchUpdateChannelStatus,
  getChannelDisableImpact,
  type ChannelDisableImpactCustomer,
  type ChannelDisableTarget,
} from '../../api'
import { CHANNEL_STATUS } from '../../constants'
import { channelsQueryKeys } from '../../lib/channel-actions'

type ChannelDisableDialogProps = {
  target: ChannelDisableTarget
  onOpenChange: (open: boolean) => void
  onDisabled?: () => void
}

export function ChannelDisableDialog(props: ChannelDisableDialogProps) {
  const { t, i18n } = useTranslation()
  const locale = toIntlLocale(i18n.resolvedLanguage || i18n.language)
  const queryClient = useQueryClient()
  const impact = useQuery({
    queryKey: [...channelsQueryKeys.all, 'disable-impact', props.target],
    queryFn: ({ signal }) => getChannelDisableImpact(props.target, signal),
    staleTime: 0,
    refetchOnMount: 'always',
    refetchOnWindowFocus: false,
    retry: false,
    meta: { errorToast: false },
  })
  const mutation = useMutation({
    mutationFn: async () => {
      // Disable exactly the channels reviewed in this snapshot, including when
      // the selection originated from a tag whose membership could change.
      if (!impact.data || impact.isFetching || impact.isError) return null
      const ids = impact.data.channels.map((channel) => channel.id)
      if (ids.length === 0) return null
      const response = await batchUpdateChannelStatus(
        ids,
        CHANNEL_STATUS.MANUAL_DISABLED
      )
      requireServerSuccess(response)
      return response.data ?? 0
    },
    onError: (error) =>
      handleServerError(error, t('Failed to disable channels')),
    onSuccess: (count) => {
      if (count === null) return
      toast.success(t('{{count}} channel(s) disabled', { count }))
      void queryClient.invalidateQueries({
        queryKey: channelsQueryKeys.lists(),
      })
      props.onDisabled?.()
      props.onOpenChange(false)
    },
  })
  const columns: StaticDataTableColumn<ChannelDisableImpactCustomer>[] = [
    {
      id: 'customer',
      header: t('Username'),
      className: 'w-40',
      cell: (row) => `${row.username} (#${row.user_id})`,
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
      header: t('Key group'),
      className: 'w-44',
      cell: (row) => row.key_group,
    },
    {
      id: 'priority',
      header: t('Effective priority'),
      className: 'w-32',
      cell: (row) => (
        <div className='flex items-center gap-2'>
          <span>{formatNumber(row.effective_priority, locale)}</span>
          <Badge variant={row.primary ? 'default' : 'secondary'}>
            {row.primary ? t('Primary route') : t('Fallback')}
          </Badge>
        </div>
      ),
    },
    {
      id: 'alternatives',
      header: t('Channels remaining after disabling'),
      className: 'w-36',
      cell: (row) =>
        row.alternative_channels > 0 ? (
          formatNumber(row.alternative_channels, locale)
        ) : (
          <Badge variant='destructive'>{t('No remaining channels')}</Badge>
        ),
    },
  ]

  return (
    <ConfirmDialog
      open
      onOpenChange={(open) => {
        if (!mutation.isPending) props.onOpenChange(open)
      }}
      title={t('Disable channels?')}
      desc={t(
        'Review affected customers and routing priorities before disabling these channels.'
      )}
      confirmText={t('Confirm disable')}
      destructive
      disabled={
        impact.isFetching || impact.isError || !impact.data?.channels.length
      }
      isLoading={mutation.isPending}
      handleConfirm={() => mutation.mutate()}
      className='max-h-[90dvh] grid-rows-[auto_minmax(0,1fr)_auto] overflow-hidden data-[size=default]:max-w-[calc(100%-2rem)] data-[size=default]:sm:max-w-5xl'
    >
      <div
        className='min-h-0 space-y-3 overflow-y-auto'
        aria-busy={impact.isFetching}
      >
        {impact.isPending && (
          <LoadingState message={t('Checking affected customers...')} />
        )}
        {impact.isError && (
          <ErrorState
            title={t('Failed to check affected customers')}
            description={t(getServerErrorMessage(impact.error))}
            onRetry={() => void impact.refetch()}
            className='min-h-40'
          />
        )}
        {impact.data && !impact.isError && (
          <>
            <p className='text-sm font-medium'>
              {t(
                '{{channels}} channel(s), {{customers}} affected customer(s)',
                {
                  channels: formatNumber(impact.data.channels.length, locale),
                  customers: formatNumber(impact.data.customer_count, locale),
                }
              )}
            </p>
            <p className='text-muted-foreground text-xs'>
              {t(
                'Includes primary and fallback routes for active customers with valid API keys. Actual routing also depends on request parameters and capacity.'
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
                title={t('No enabled channels in this selection')}
                className='min-h-40'
              />
            ) : (
              <StaticDataTable
                className='overflow-x-auto'
                columns={columns}
                data={impact.data.customers}
                getRowKey={(row) =>
                  `${row.user_id}:${row.channel_id}:${row.model}:${row.key_group}`
                }
                tableClassName='min-w-[60rem] table-fixed'
                tableProps={{ withContainer: false }}
                emptyContent={t('No affected customers with valid API keys')}
                containerProps={{
                  role: 'region',
                  'aria-label': t('Affected customer routes'),
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
