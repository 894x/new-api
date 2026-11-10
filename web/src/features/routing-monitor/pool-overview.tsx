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
import { useState } from 'react'
import { createPortal } from 'react-dom'
import { useTranslation } from 'react-i18next'

import { Dialog } from '@/components/dialog'
import { EmptyState } from '@/components/empty-state'
import { LoadingState } from '@/components/loading-state'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { NativeSelect, NativeSelectOption } from '@/components/ui/native-select'
import { Switch } from '@/components/ui/switch'
import { toIntlLocale } from '@/i18n/languages'
import { formatNumber } from '@/lib/format'

import { ChannelPoolPanel } from './channel-pool-panel'
import { ChannelStatus } from './channel-status'
import { MonitorPanel, type MonitorTarget } from './monitor-panel'
import { monitorTargetKey, overviewFlows } from './pool-overview-data'
import { PoolOverviewFlow } from './pool-overview-flow'
import { usePoolOverview } from './use-pool-overview'

export function PoolOverview(props: {
  targets: MonitorTarget[]
  selectedTarget: MonitorTarget
  onAdd: (target: MonitorTarget) => void
  onSelect: (target: MonitorTarget) => void
  onRemove: (target: MonitorTarget) => void
  isDirty: (target: MonitorTarget) => boolean
  channelPanelContainer?: HTMLElement | null
  onSelectChannel?: (id: number | undefined) => void
}) {
  const { t, i18n } = useTranslation()
  const locale = toIntlLocale(i18n.resolvedLanguage || i18n.language)
  const [adding, setAdding] = useState(false)
  const [paused, setPaused] = useState(false)
  const [expanded, setExpanded] = useState(false)
  const [showIdle, setShowIdle] = useState(false)
  const [equalWidth, setEqualWidth] = useState(false)
  const [search, setSearch] = useState('')
  const [nodeId, setNodeId] = useState('__all')
  const [focused, setFocused] = useState(false)
  const [channelId, setChannelId] = useState<number>()
  const data = usePoolOverview(props.targets, paused, nodeId)
  const allFlows = overviewFlows(data.pools)
  const term = search.trim().toLowerCase()
  const flows = allFlows.filter(
    (flow) =>
      !term ||
      [
        flow.route.group,
        flow.route.model,
        ...flow.targets.map(
          (pool) => `${pool.username ?? ''} ${pool.target.userId ?? ''}`
        ),
        ...flow.names.values(),
      ].some((name) => name.toLowerCase().includes(term)) ||
      flow.route.channels.some(
        (channel) => String(channel.channel_id) === term.replace(/^#/, '')
      )
  )
  const channels = new Map<number, Set<string>>()
  let total = 0
  for (const flow of flows) {
    total += flow.route.selection_count
    for (const channel of flow.route.channels) {
      if (!showIdle && channel.selection_count === 0) continue
      const owners = channels.get(channel.channel_id) ?? new Set<string>()
      owners.add(flow.key)
      channels.set(channel.channel_id, owners)
    }
  }
  const selectedKey = monitorTargetKey(props.selectedTarget)
  const quality =
    channelId === undefined
      ? []
      : flows.flatMap((flow) => {
          const channel = flow.route.channels.find(
            (item) => item.channel_id === channelId
          )
          return channel ? [{ flow, channel }] : []
        })
  const loading = data.pools.some((pool) => pool.loading)
  const unavailable = data.pools.some(
    (pool) => pool.unavailable || pool.partial
  )
  const select = (target: MonitorTarget, id?: number) => {
    props.onSelect(target)
    props.onSelectChannel?.(undefined)
    setFocused(true)
    setChannelId(id)
  }
  const selectChannel = (id: number) => {
    setFocused(true)
    setChannelId(id)
    props.onSelectChannel?.(id)
  }
  return (
    <section
      className='flex h-full min-h-0 min-w-0 flex-col gap-3'
      aria-label={t('Multi-pool overview')}
    >
      {channelId !== undefined &&
        props.channelPanelContainer &&
        createPortal(
          <ChannelPoolPanel
            key={channelId}
            channelId={channelId}
            name={
              data.pools
                .map((pool) => pool.names.get(channelId))
                .find(Boolean) ?? `#${channelId}`
            }
            pools={data.pools}
            isDirty={props.isDirty}
            onSelect={(target) => {
              if (
                !props.targets.some(
                  (item) => monitorTargetKey(item) === monitorTargetKey(target)
                )
              ) {
                props.onAdd(target)
              }
              select(target, channelId)
            }}
            onClose={() => {
              setFocused(false)
              setChannelId(undefined)
              props.onSelectChannel?.(undefined)
            }}
          />,
          props.channelPanelContainer
        )}
      <div className='flex shrink-0 flex-wrap items-center gap-2'>
        <Input
          value={search}
          onChange={(event) => setSearch(event.target.value)}
          placeholder={t('Search models')}
          aria-label={t('Search monitoring pools')}
          className='h-8 w-48'
        />
        <NativeSelect
          value={nodeId}
          onChange={(event) => setNodeId(event.target.value)}
          aria-label={t('Monitor node')}
          className='h-8 text-xs'
        >
          <NativeSelectOption value='__all'>
            {t('All nodes')}
          </NativeSelectOption>
          {[...data.nodes].map(([id, name]) => (
            <NativeSelectOption key={id} value={id}>
              {name}
            </NativeSelectOption>
          ))}
        </NativeSelect>
        <Button variant='outline' size='sm' onClick={() => setAdding(true)}>
          {t('Add monitoring pool')}
        </Button>
        <Button
          variant='outline'
          size='sm'
          onClick={() => setPaused((value) => !value)}
        >
          {paused ? t('Resume') : t('Pause')}
        </Button>
        <Button variant='ghost' size='sm' onClick={data.refresh}>
          {t('Refresh')}
        </Button>
      </div>
      <div
        className='flex shrink-0 flex-wrap gap-1.5'
        aria-label={t('Monitoring pools')}
      >
        {data.pools.map((pool) => {
          const key = monitorTargetKey(pool.target)
          const identity =
            pool.username ||
            (pool.target.userId
              ? `#${pool.target.userId}`
              : t('Default configuration'))
          return (
            <div
              key={key}
              className='flex max-w-full min-w-0 items-center rounded-md border'
            >
              <Button
                variant={key === selectedKey ? 'secondary' : 'ghost'}
                size='xs'
                className='max-w-80 min-w-0 rounded-r-none'
                aria-pressed={key === selectedKey}
                aria-label={`${identity} · ${pool.target.group} · ${pool.target.model}`}
                onClick={() => select(pool.target)}
              >
                <span className='truncate'>
                  {identity} · {pool.target.group} · {pool.target.model}
                </span>
                {props.isDirty(pool.target) && (
                  <span
                    className='bg-warning size-1.5 shrink-0 rounded-full'
                    title={t('Unsaved changes')}
                  />
                )}
              </Button>
              <Button
                variant='ghost'
                size='icon-xs'
                disabled={
                  props.targets.length <= 1 || props.isDirty(pool.target)
                }
                aria-label={t('Remove {{model}} from monitoring', {
                  model: `${identity} · ${pool.target.model}`,
                })}
                onClick={() => props.onRemove(pool.target)}
              >
                ×
              </Button>
            </div>
          )
        })}
      </div>
      <div className='flex shrink-0 flex-wrap items-center justify-between gap-2 rounded-lg border px-3 py-2 text-xs'>
        <div className='flex flex-wrap gap-4'>
          <span>
            {t('Routing pools')}: <b>{formatNumber(flows.length, locale)}</b>
          </span>
          <span>
            {t('Channels')}: <b>{formatNumber(channels.size, locale)}</b>
          </span>
          <span>
            {t('Shared channels')}:{' '}
            <b>
              {formatNumber(
                [...channels.values()].filter((owners) => owners.size > 1)
                  .length,
                locale
              )}
            </b>
          </span>
          <span>
            {t('Observed selections')}: <b>{formatNumber(total, locale)}</b>
          </span>
        </div>
        <Badge variant='outline'>
          {paused ? t('Paused') : t('Live updates')}
        </Badge>
      </div>
      <div className='flex shrink-0 flex-wrap items-center gap-3 text-xs'>
        <div
          className='flex gap-1'
          role='group'
          aria-label={t('Channel display')}
        >
          <Button
            size='xs'
            variant={expanded ? 'ghost' : 'secondary'}
            aria-pressed={!expanded}
            onClick={() => setExpanded(false)}
          >
            {t('Merged channels')}
          </Button>
          <Button
            size='xs'
            variant={expanded ? 'secondary' : 'ghost'}
            aria-pressed={expanded}
            onClick={() => setExpanded(true)}
          >
            {t('Channels by pool')}
          </Button>
        </div>
        <label className='text-muted-foreground flex items-center gap-2'>
          <Switch size='sm' checked={showIdle} onCheckedChange={setShowIdle} />
          {t('Show channels without traffic')}
        </label>
        <label className='text-muted-foreground flex items-center gap-2'>
          <Switch
            size='sm'
            checked={equalWidth}
            onCheckedChange={setEqualWidth}
          />
          {t('Equal line widths')}
        </label>
        <Button
          size='xs'
          variant='ghost'
          onClick={() => {
            setFocused(false)
            setChannelId(undefined)
            props.onSelectChannel?.(undefined)
          }}
        >
          {t('Clear highlight')}
        </Button>
      </div>
      {unavailable && (
        <p role='status' className='text-warning shrink-0 text-xs'>
          {t('Some monitoring data is unavailable. Counts may be incomplete.')}
        </p>
      )}
      {loading && !flows.length && (
        <LoadingState message={t('Loading routing observations...')} />
      )}
      {!loading && !flows.length && (
        <EmptyState
          className='min-h-48 flex-1'
          title={t('No routing observations')}
          description={t(
            'Priority routing requests and streaming requests using smart or discount-first routing will appear here.'
          )}
        />
      )}
      {!!flows.length && (
        <PoolOverviewFlow
          flows={flows}
          selectedTarget={props.selectedTarget}
          focused={focused}
          channelId={channelId}
          expanded={expanded}
          showIdle={showIdle}
          equalWidth={equalWidth}
          paused={paused}
          onSelect={select}
          onSelectChannel={selectChannel}
        />
      )}
      {quality.length > 0 && (
        <div
          className='max-h-40 shrink-0 overflow-auto rounded-lg border p-3'
          aria-label={t('Pools using this channel')}
        >
          <div className='text-muted-foreground mb-2 text-xs'>
            {t('Channel quality is shown separately for each pool.')} · #
            {channelId}
          </div>
          <div className='flex flex-wrap gap-2'>
            {quality.map(({ flow, channel }) => (
              <Button
                key={flow.key}
                variant='outline'
                size='sm'
                className='h-auto max-w-full flex-wrap justify-start gap-2 py-1.5 text-xs'
                onClick={() =>
                  select(
                    flow.targets.find(
                      (pool) => monitorTargetKey(pool.target) === selectedKey
                    )?.target ?? flow.targets[0].target,
                    channel.channel_id
                  )
                }
              >
                <span className='max-w-56 truncate'>
                  {flow.route.policy || t('Default configuration')} ·{' '}
                  {flow.route.model}
                </span>
                <span>
                  {formatNumber(channel.selected_share * 100, locale)}%
                </span>
                <span>
                  TTFT{' '}
                  {channel.ttft_ms === null
                    ? '—'
                    : `${formatNumber(channel.ttft_ms, locale)} ms`}
                </span>
                <ChannelStatus
                  channel={channel}
                  at={flow.at}
                  priority={flow.route.mode === 'priority'}
                  compact
                />
              </Button>
            ))}
          </div>
        </div>
      )}
      <p className='text-muted-foreground shrink-0 text-xs'>
        {t('Inherited default-pool traffic is counted once.')}{' '}
        {t('Live snapshots cover the last {{seconds}} seconds.', {
          seconds: flows[0]?.route.window_seconds ?? 60,
        })}
      </p>
      <Dialog
        open={adding}
        onOpenChange={setAdding}
        title={t('Add monitoring pool')}
        description={t('Choose a user, group and model for this panel.')}
        contentClassName='sm:max-w-lg'
      >
        <div className='min-h-80'>
          <MonitorPanel
            number={props.targets.length + 1}
            initialUserId={props.selectedTarget.userId}
            onChange={(target) => {
              if (target) {
                props.onAdd(target)
                setAdding(false)
                setFocused(false)
                setChannelId(undefined)
                props.onSelectChannel?.(undefined)
              }
            }}
          />
        </div>
      </Dialog>
    </section>
  )
}
