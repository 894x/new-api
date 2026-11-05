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

import { Badge } from '@/components/ui/badge'

import type { MonitorChannel } from './api'
import { monitorChannelStatus } from './route-data'

export function ChannelStatus(props: { channel: MonitorChannel; at: number }) {
  const { t } = useTranslation()
  if (!props.channel.active) {
    return <Badge variant='outline'>{t('Retired candidate')}</Badge>
  }
  const status = monitorChannelStatus(props.channel, props.at)
  if (status === 'ejected') {
    return <Badge variant='destructive'>{t('Ejected')}</Badge>
  }
  if (status === 'degraded') {
    return <Badge variant='warning'>{t('Degraded')}</Badge>
  }
  if (status === 'unverified') {
    return <Badge variant='secondary'>{t('Awaiting samples')}</Badge>
  }
  if (status === 'probe') {
    return <Badge variant='outline'>{t('Exploration')}</Badge>
  }
  return <Badge variant='secondary'>{t('Healthy')}</Badge>
}
