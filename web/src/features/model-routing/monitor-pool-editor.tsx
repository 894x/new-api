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
import { useTranslation } from 'react-i18next'

import { ErrorState } from '@/components/error-state'
import { LoadingState } from '@/components/loading-state'
import { canEditDefaultPool } from '@/features/routing-monitor/channel-pool-actions'
import type { MonitorTarget } from '@/features/routing-monitor/monitor-panel'
import { useAuthStore } from '@/stores/auth-store'

import { getRoutingPool, routingPoolKeys } from './api'
import { PoolCard } from './pool-card'
import { UserPoolCard } from './user-pool-card'

export function MonitorPoolEditor(props: {
  target: MonitorTarget
  onDirtyChange: (dirty: boolean) => void
}) {
  if (props.target.userId) {
    return (
      <UserPoolCard
        embedded
        userId={props.target.userId}
        model={props.target.model}
        group={props.target.group}
        groupLabel={props.target.group}
        onDirtyChange={props.onDirtyChange}
      />
    )
  }
  return <DefaultMonitorPoolEditor {...props} />
}

function DefaultMonitorPoolEditor(props: {
  target: MonitorTarget
  onDirtyChange: (dirty: boolean) => void
}) {
  const { t } = useTranslation()
  const actor = useAuthStore((state) => state.auth.user)
  const query = useQuery({
    queryKey: routingPoolKeys.detail(props.target.group, props.target.model),
    queryFn: () => getRoutingPool(props.target.group, props.target.model),
    staleTime: 10000,
    refetchOnWindowFocus: false,
    meta: { errorToast: false },
  })
  if (query.isPending) return <LoadingState />
  if (query.isError || !query.data) {
    return <ErrorState onRetry={() => void query.refetch()} />
  }
  const data = query.data
  return (
    <PoolCard
      embedded
      pool={{
        routing_group: props.target.group,
        model: props.target.model,
        mode: data.effective_mode || 'priority',
        revision: '',
        channels: data.candidates
          .filter((channel) => channel.available)
          .map((channel) => ({
            channel_id: channel.channel_id,
            priority: channel.priority,
            weight: channel.weight,
            enabled: true,
          })),
        ...data.pool,
        candidates: data.candidates,
        tuning_defaults: data.tuning_defaults,
      }}
      targetLabel={t('Default configuration')}
      canWrite={canEditDefaultPool(actor)}
      onDirtyChange={props.onDirtyChange}
    />
  )
}
