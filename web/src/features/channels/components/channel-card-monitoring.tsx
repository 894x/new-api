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

import { ChannelMonitoringStrip } from '@/features/channel-monitoring/monitoring-strip'
import { useChannelMonitoring } from '@/features/channel-monitoring/use-channel-monitoring'

import { getDefaultMonitoringModel } from '../lib/channel-utils'
import type { Channel } from '../types'

export function ChannelCardMonitoring(props: {
  channel: Channel
  channelIds?: number[]
  sensitiveVisible: boolean
}) {
  const { t } = useTranslation()
  const model = getDefaultMonitoringModel(props.channel)
  const monitoring = useChannelMonitoring(
    model,
    props.channelIds ?? [props.channel.id]
  )
  const channelName = props.sensitiveVisible ? props.channel.name : '••••'

  return (
    <section
      aria-label={t('Channel monitoring')}
      className='min-w-0 space-y-1.5 border-t pt-3'
    >
      <div className='text-muted-foreground flex min-w-0 items-center gap-2 text-[11px]'>
        <span className='shrink-0'>24h</span>
        {model && (
          <span
            className='min-w-0 truncate'
            title={props.sensitiveVisible ? model : undefined}
          >
            {props.sensitiveVisible ? model : '••••'}
          </span>
        )}
      </div>
      {model ? (
        <ChannelMonitoringStrip
          channelName={channelName}
          summary={monitoring.data?.find(
            (entry) => entry.channel_id === props.channel.id
          )}
          loading={monitoring.isLoading}
          error={monitoring.isError}
          showTimelineLabels={false}
        />
      ) : (
        <p role='status' className='text-muted-foreground text-xs'>
          {t('No models available')}
        </p>
      )}
    </section>
  )
}
