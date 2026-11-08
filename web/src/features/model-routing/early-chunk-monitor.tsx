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
import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import { EmptyState } from '@/components/empty-state'
import { ErrorState } from '@/components/error-state'
import { LoadingState } from '@/components/loading-state'
import { Button } from '@/components/ui/button'
import { Combobox } from '@/components/ui/combobox'
import { Input } from '@/components/ui/input'

import { EarlyChunkGridView } from './early-chunk-grid'
import { getEarlyChunkGrid } from './early-chunk-monitor-api'

export default function EarlyChunkMonitor(props: {
  model: string
  channels: { channel_id: number; channel_name: string }[]
}) {
  const { t } = useTranslation()
  const [selected, setSelected] = useState<string | null>(null)
  const [probe, setProbe] = useState(-1)
  const [draft, setDraft] = useState('')
  const channel =
    props.channels.find((c) => String(c.channel_id) === selected) ??
    props.channels[0]
  const query = useQuery({
    queryKey: ['early-chunk-grid', props.model, channel?.channel_id, probe],
    queryFn: ({ signal }) =>
      getEarlyChunkGrid(props.model, channel?.channel_id ?? 0, probe, signal),
    enabled: !!channel,
    refetchInterval: 10000,
    refetchOnWindowFocus: false,
    retry: false,
    meta: { errorToast: false },
  })
  const validProbe =
    draft === '' || (/^\d+$/.test(draft) && Number(draft) <= 30000)
  return (
    <section
      className='order-3 flex min-h-0 min-w-0 flex-col gap-3 xl:col-start-2 xl:row-span-2 xl:row-start-1 xl:h-full'
      aria-label={t('TTFT prediction grid')}
    >
      <div className='flex flex-wrap items-end gap-2'>
        <div className='min-w-0 flex-1'>
          <Combobox
            options={props.channels.map((c) => ({
              value: String(c.channel_id),
              label: `${c.channel_name} #${c.channel_id}`,
            }))}
            value={channel ? String(channel.channel_id) : null}
            onValueChange={setSelected}
            aria-label={t('Select channel')}
            placeholder={t('Select channel')}
            className='w-full'
          />
        </div>
        <form
          className='flex flex-wrap items-end gap-2'
          onSubmit={(e) => {
            e.preventDefault()
            if (validProbe) setProbe(draft === '' ? -1 : Number(draft))
          }}
        >
          <label className='space-y-1 text-xs'>
            <span>{t('Elapsed time (ms)')}</span>
            <Input
              value={draft}
              onChange={(e) => setDraft(e.target.value)}
              placeholder={t('Window midpoint')}
              inputMode='numeric'
              aria-invalid={!validProbe}
              className='w-36'
            />
          </label>
          <Button type='submit' variant='outline' disabled={!validProbe}>
            {t('Apply')}
          </Button>
        </form>
        <Button
          variant='outline'
          disabled={!channel || query.isFetching}
          onClick={() => void query.refetch()}
        >
          {t('Refresh')}
        </Button>
      </div>
      <p className='text-muted-foreground text-xs'>
        {t(
          'Statistics are shared by model and channel across all pools. Pool settings only control pre-emission.'
        )}
      </p>
      {!channel && <EmptyState title={t('No channels')} />}
      {channel && query.isPending && <LoadingState />}
      {channel && query.isError && (
        <ErrorState
          title={t('Prediction statistics unavailable')}
          description={t(
            'Collection or storage is unavailable. Pre-emission fails closed.'
          )}
          onRetry={() => void query.refetch()}
        />
      )}
      {query.data && !query.isError && (
        <EarlyChunkGridView
          key={`${props.model}:${channel?.channel_id}`}
          grid={query.data}
        />
      )}
    </section>
  )
}
