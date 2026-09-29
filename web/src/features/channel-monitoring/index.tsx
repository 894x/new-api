/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as published by
the Free Software Foundation, either version 3 of the License, or
(at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/
import { useQuery } from '@tanstack/react-query'
import { type ReactNode, useMemo } from 'react'
import { useTranslation } from 'react-i18next'

import { SectionPageLayout } from '@/components/layout'
import { Skeleton } from '@/components/ui/skeleton'
import { getChannels } from '@/features/channels/api'
import {
  ChannelPerformanceDiagnostics,
  type ChannelMonitoringOption,
} from '@/features/performance-analytics/channel-diagnostics'

async function getChannelMonitoringOptions(): Promise<
  ChannelMonitoringOption[]
> {
  const pageSize = 100
  const firstResponse = await getChannels({
    p: 1,
    page_size: pageSize,
    id_sort: true,
  })
  if (!firstResponse.success || !firstResponse.data) {
    throw new Error(firstResponse.message || 'Failed to load channels')
  }

  const totalPages = Math.ceil(firstResponse.data.total / pageSize)
  const pages = await Promise.all(
    Array.from({ length: Math.max(0, totalPages - 1) }, (_, index) =>
      getChannels({
        p: index + 2,
        page_size: pageSize,
        id_sort: true,
      })
    )
  )
  if (pages.some((response) => !response.success || !response.data)) {
    throw new Error('Failed to load channels')
  }
  const channels = [
    ...firstResponse.data.items,
    ...pages.flatMap((response) => response.data?.items ?? []),
  ]

  return channels.map((channel) => ({
    id: channel.id,
    name: channel.name,
    status: channel.status,
  }))
}

export function ChannelMonitoring() {
  const { t } = useTranslation()
  const channelsQuery = useQuery({
    queryKey: ['channel-monitoring', 'options'],
    queryFn: getChannelMonitoringOptions,
    staleTime: 60_000,
    retry: false,
  })
  const channelOptions = useMemo(
    () => channelsQuery.data ?? [],
    [channelsQuery.data]
  )
  let content: ReactNode = (
    <ChannelPerformanceDiagnostics channelOptions={channelOptions} />
  )
  if (channelsQuery.isLoading) {
    content = <Skeleton className='h-24 rounded-xl' />
  } else if (channelsQuery.isError) {
    content = (
      <p role='alert' className='text-destructive text-sm'>
        {t('Failed to load channels')}
      </p>
    )
  }

  return (
    <SectionPageLayout>
      <SectionPageLayout.Title>
        {t('Channel Monitoring')}
      </SectionPageLayout.Title>
      <SectionPageLayout.Content>
        <div className='flex flex-col gap-3 sm:gap-4'>
          <p className='text-muted-foreground text-sm'>
            {t(
              'Monitor channel latency, concurrency, transport pools, and aggregated errors in one place.'
            )}
          </p>
          {content}
        </div>
      </SectionPageLayout.Content>
    </SectionPageLayout>
  )
}
