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
import { useTranslation } from 'react-i18next'

import { StatusBadge } from '@/components/status-badge'
import { cn } from '@/lib/utils'

import type { MonitorChannel } from './api'
import { monitorChannelStatus, monitorChannelTone } from './route-data'

export function ChannelStatus(props: {
  channel: MonitorChannel
  at: number
  priority?: boolean
}) {
  const { t } = useTranslation()
  const tone = monitorChannelTone(props.channel, props.at, props.priority)
  const status = monitorChannelStatus(props.channel, props.at)
  let label = t('Healthy')
  if (props.channel.status === 3) {
    label = t('Automatically disabled')
  } else if (props.channel.status === 2) {
    label = t('Manually disabled')
  } else if (!props.channel.active) {
    label = t('Retired candidate')
  } else if (props.priority) {
    label = props.channel.selection_count === 0 ? t('Standby') : t('Available')
  } else if (status === 'ejected') {
    label = t('Ejected')
  } else if (status === 'degraded') {
    label = t('Degraded')
  } else if (status === 'unverified') {
    label = t('Awaiting samples')
  } else if (status === 'probe') {
    label = t('Exploration')
  }
  return (
    <StatusBadge
      variant={tone}
      copyable={false}
      title={props.channel.status_reason || label}
      className={cn(
        'border text-xs',
        tone === 'neutral' && 'border-border text-foreground',
        tone === 'danger' && 'border-destructive/30 bg-destructive/10',
        tone === 'warning' && 'border-warning/40 bg-warning/10'
      )}
    >
      {label}
    </StatusBadge>
  )
}
