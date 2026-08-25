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
import { Loader2, RefreshCw } from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { StatusBadge, type StatusVariant } from '@/components/status-badge'
import { Button } from '@/components/ui/button'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { formatTimestampToDate } from '@/lib/format'

import type {
  AssetChannelReplicaDetails,
  AssetGroupChannelReplicaDetails,
} from '../types'

type ReplicaDetails =
  | AssetChannelReplicaDetails
  | AssetGroupChannelReplicaDetails

function replicaStatusVariant(state: string): StatusVariant {
  switch (state) {
    case 'ready':
      return 'success'
    case 'failed':
      return 'danger'
    case 'processing':
      return 'warning'
    default:
      return 'neutral'
  }
}

function upstreamId(replica: ReplicaDetails, resource: 'asset' | 'group') {
  return resource === 'asset'
    ? (replica as AssetChannelReplicaDetails).upstream_asset_id
    : (replica as AssetGroupChannelReplicaDetails).upstream_group_id
}

export function AdminReplicaDetails(props: {
  replicas: ReplicaDetails[]
  resource: 'asset' | 'group'
  canSync: boolean
  syncingChannelId?: number | 'all' | null
  onSync: (channelId: number) => void
}) {
  const { t } = useTranslation()

  return (
    <div className='rounded-lg border'>
      <Table compact>
        <TableHeader>
          <TableRow>
            <TableHead>{t('Channel')}</TableHead>
            <TableHead>{t('Backend')}</TableHead>
            <TableHead>{t('Status')}</TableHead>
            <TableHead>{t('Upstream ID')}</TableHead>
            <TableHead>{t('Last updated')}</TableHead>
            {props.canSync ? (
              <TableHead className='text-right'>{t('Actions')}</TableHead>
            ) : null}
          </TableRow>
        </TableHeader>
        <TableBody>
          {props.replicas.map((replica) => {
            const id = upstreamId(replica, props.resource)
            const stateLabel =
              {
                ready: t('Ready'),
                processing: t('Processing'),
                failed: t('Failed'),
                not_synced: t('Not synchronized'),
              }[replica.state] || replica.state
            const statusLabel =
              (props.resource === 'asset' &&
                (replica as AssetChannelReplicaDetails).upstream_status) ||
              stateLabel
            const errorCode =
              props.resource === 'asset'
                ? (replica as AssetChannelReplicaDetails).last_error_code
                : undefined
            const error = [errorCode, replica.last_error]
              .filter(Boolean)
              .join(': ')
            const syncing = props.syncingChannelId === replica.channel_id

            return (
              <TableRow key={replica.channel_id}>
                <TableCell>
                  <div className='flex min-w-40 flex-col gap-1'>
                    <span className='font-medium'>
                      {replica.channel_name || `#${replica.channel_id}`}
                    </span>
                    <StatusBadge
                      label={t(replica.enabled ? 'Enabled' : 'Disabled')}
                      variant={replica.enabled ? 'success' : 'neutral'}
                      copyable={false}
                    />
                    {error ? (
                      <span
                        className='text-destructive max-w-72 whitespace-normal'
                        title={error}
                      >
                        {error}
                      </span>
                    ) : null}
                  </div>
                </TableCell>
                <TableCell>
                  <code className='text-muted-foreground text-xs'>
                    {replica.backend || '-'}
                  </code>
                </TableCell>
                <TableCell>
                  <StatusBadge
                    label={statusLabel}
                    variant={replicaStatusVariant(replica.state)}
                    copyable={false}
                  />
                </TableCell>
                <TableCell>
                  <code className='block max-w-56 truncate text-xs' title={id}>
                    {id || '-'}
                  </code>
                </TableCell>
                <TableCell className='text-muted-foreground'>
                  {replica.updated_time
                    ? formatTimestampToDate(replica.updated_time)
                    : t('Never')}
                </TableCell>
                {props.canSync ? (
                  <TableCell className='text-right'>
                    {replica.enabled ? (
                      <Button
                        variant='outline'
                        size='sm'
                        disabled={Boolean(props.syncingChannelId)}
                        onClick={() => props.onSync(replica.channel_id)}
                      >
                        {syncing ? (
                          <Loader2 className='animate-spin' />
                        ) : (
                          <RefreshCw />
                        )}
                        {t(
                          props.resource === 'group'
                            ? 'Sync group and assets'
                            : 'Sync channel'
                        )}
                      </Button>
                    ) : null}
                  </TableCell>
                ) : null}
              </TableRow>
            )
          })}
        </TableBody>
      </Table>
    </div>
  )
}
