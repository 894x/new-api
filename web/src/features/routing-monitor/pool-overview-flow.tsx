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
import { useReducedMotion } from 'motion/react'
import { useLayoutEffect, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { ROUTING_MODE_LABELS } from '@/features/model-routing/api'
import { toIntlLocale } from '@/i18n/languages'
import { formatNumber } from '@/lib/format'
import { cn } from '@/lib/utils'

import { RequestParticles } from './flow'
import type { MonitorTarget } from './monitor-panel'
import { monitorTargetKey, type OverviewFlow } from './pool-overview-data'

function flowCurve(x1: number, y1: number, x2: number, y2: number) {
  const mid = (x1 + x2) / 2
  return `M ${x1} ${y1} C ${mid} ${y1}, ${mid} ${y2}, ${x2} ${y2}`
}

export function PoolOverviewFlow(props: {
  flows: OverviewFlow[]
  selectedTarget: MonitorTarget
  focused: boolean
  channelId?: number
  expanded: boolean
  showIdle: boolean
  equalWidth: boolean
  paused: boolean
  onSelect: (target: MonitorTarget, channelId?: number) => void
  onSelectChannel: (id: number) => void
}) {
  const { t, i18n } = useTranslation()
  const locale = toIntlLocale(i18n.resolvedLanguage || i18n.language)
  const reduceMotion = useReducedMotion()
  const container = useRef<HTMLDivElement>(null)
  const [availableHeight, setAvailableHeight] = useState(0)
  useLayoutEffect(() => {
    const element = container.current
    if (!element) return
    const measure = () => setAvailableHeight(element.clientHeight - 2)
    measure()
    const observer = new ResizeObserver(measure)
    observer.observe(element)
    return () => observer.disconnect()
  }, [])
  const selectedKey = monitorTargetKey(props.selectedTarget)
  const edges = props.flows.flatMap((flow) =>
    flow.route.channels
      .filter((channel) => props.showIdle || channel.selection_count > 0)
      .map((channel) => ({ flow, channel }))
  )
  const channels = new Map<
    string,
    { id: number; name: string; count: number; flows: Set<string> }
  >()
  for (const { flow, channel } of edges) {
    const key = props.expanded
      ? `${flow.key}:${channel.channel_id}`
      : String(channel.channel_id)
    let item = channels.get(key)
    if (!item) {
      item = {
        id: channel.channel_id,
        name: flow.names.get(channel.channel_id) || `#${channel.channel_id}`,
        count: 0,
        flows: new Set(),
      }
      channels.set(key, item)
    }
    item.count += channel.selection_count
    item.flows.add(flow.key)
  }
  const height = Math.max(
    380,
    availableHeight,
    channels.size * 92 + 70,
    props.flows.length * 125 + 80
  )
  const position = (index: number, count: number) =>
    80 +
    (count < 2 ? (height - 150) / 2 : (index * (height - 150)) / (count - 1))
  const channelPositions = new Map(
    [...channels.keys()].map((key, index) => [
      key,
      position(index, channels.size),
    ])
  )
  const poolPositions = new Map(
    props.flows.map((flow, index) => {
      const values = edges
        .filter((edge) => edge.flow.key === flow.key)
        .map(
          ({ channel }) =>
            channelPositions.get(`${flow.key}:${channel.channel_id}`) ?? 100
        )
      const y =
        props.expanded && values.length
          ? values.reduce((sum, value) => sum + value, 0) / values.length
          : position(index, props.flows.length)
      return [flow.key, y]
    })
  )
  const sources = [
    ...new Set(props.flows.map((flow) => flow.route.policy || 'base')),
  ]
  const sourcePositions = new Map(
    sources.map((source, index) => [source, position(index, sources.length)])
  )
  const maxCount = Math.max(
    1,
    ...edges.map(({ channel }) => channel.selection_count)
  )
  const activeFlow = props.flows.find((flow) =>
    flow.targets.some((pool) => monitorTargetKey(pool.target) === selectedKey)
  )?.key
  const related = (flow: OverviewFlow, channelId?: number) => {
    if (!props.focused) return true
    if (props.channelId !== undefined) {
      return channelId === undefined
        ? flow.route.channels.some(
            (channel) => channel.channel_id === props.channelId
          )
        : channelId === props.channelId
    }
    return flow.key === activeFlow
  }
  const chooseTarget = (flow: OverviewFlow) =>
    flow.targets.find((pool) => monitorTargetKey(pool.target) === selectedKey)
      ?.target ?? flow.targets[0].target
  const color = (flow: OverviewFlow) =>
    `var(--chart-${(props.flows.indexOf(flow) % 5) + 1})`
  return (
    <div
      ref={container}
      className='bg-muted/10 min-h-0 flex-1 overflow-auto rounded-lg border'
      aria-label={t('Multi-pool overview')}
    >
      <div className='relative min-w-[48rem]' style={{ height }}>
        <svg
          className='pointer-events-none absolute inset-0 size-full'
          viewBox={`0 0 1120 ${height}`}
          preserveAspectRatio='none'
          aria-hidden='true'
        >
          {props.flows.map((flow) => {
            const y = poolPositions.get(flow.key) ?? 100
            const sourceY =
              sourcePositions.get(flow.route.policy || 'base') ?? 100
            const entry = flowCurve(182, sourceY, 310, y)
            const paths = new Map<number, string>()
            return (
              <g key={flow.key}>
                <path
                  d={entry}
                  fill='none'
                  stroke={color(flow)}
                  strokeWidth={Math.max(
                    3,
                    Math.min(22, (flow.route.selection_count / maxCount) * 10)
                  )}
                  opacity={related(flow) ? 0.18 : 0.035}
                />
                {flow.route.channels
                  .filter(
                    (channel) => props.showIdle || channel.selection_count > 0
                  )
                  .map((channel) => {
                    const key = props.expanded
                      ? `${flow.key}:${channel.channel_id}`
                      : String(channel.channel_id)
                    const path = flowCurve(
                      585,
                      y,
                      850,
                      channelPositions.get(key) ?? 100
                    )
                    paths.set(channel.channel_id, `${entry} ${path}`)
                    return (
                      <path
                        key={key}
                        d={path}
                        fill='none'
                        stroke={color(flow)}
                        strokeWidth={
                          props.equalWidth
                            ? 7
                            : Math.max(
                                1.5,
                                (channel.selection_count / maxCount) * 28
                              )
                        }
                        strokeDasharray={
                          channel.selection_count === 0 ? '4 5' : undefined
                        }
                        opacity={
                          related(flow, channel.channel_id) ? 0.3 : 0.035
                        }
                      />
                    )
                  })}
                <RequestParticles
                  requests={flow.route.requests ?? []}
                  paths={paths}
                  at={flow.at}
                  disabled={props.paused || !!reduceMotion || !related(flow)}
                />
              </g>
            )
          })}
        </svg>
        <span className='text-muted-foreground absolute top-5 left-[2%] text-xs'>
          {t('User')}
        </span>
        <span className='text-muted-foreground absolute top-5 left-[27.7%] text-xs'>
          {t('Routing pools')}
        </span>
        <span className='text-muted-foreground absolute top-5 left-[75.9%] text-xs'>
          {t('Channels')}
        </span>
        {sources.map((source) => {
          const flow = props.flows.find(
            (item) => (item.route.policy || 'base') === source
          )
          if (!flow) return null
          const pool = flow.targets[0]
          const label =
            source === 'base'
              ? t('Default configuration')
              : pool.username || t('User #{{id}}', { id: pool.target.userId })
          return (
            <Button
              key={source}
              variant='outline'
              className={cn(
                'absolute left-[2%] h-16 w-[14.3%] min-w-0 flex-col items-start gap-1 overflow-hidden bg-card px-3 text-left whitespace-normal shadow-none',
                props.focused &&
                  !props.flows.some(
                    (item) =>
                      (item.route.policy || 'base') === source && related(item)
                  ) &&
                  'opacity-25'
              )}
              style={{ top: (sourcePositions.get(source) ?? 100) - 32 }}
              onClick={() => props.onSelect(chooseTarget(flow))}
            >
              <span className='w-full truncate text-xs'>{label}</span>
              <span className='text-muted-foreground text-[10px]'>
                {source === 'base'
                  ? t('Shared default pool')
                  : `#${pool.target.userId}`}
              </span>
            </Button>
          )
        })}
        {props.flows.map((flow) => (
          <Button
            key={flow.key}
            variant='outline'
            aria-label={t('View {{name}} pool', {
              name: `${flow.targets[0].username || flow.route.policy || t('Default configuration')} · ${flow.route.group} · ${flow.route.model}`,
            })}
            aria-pressed={
              props.focused &&
              flow.key === activeFlow &&
              props.channelId === undefined
            }
            className={cn(
              'absolute left-[27.7%] h-24 w-[24.55%] min-w-0 flex-col items-stretch gap-2 border-l-3 bg-card px-3 text-left shadow-none',
              props.focused && !related(flow) && 'opacity-25',
              props.focused &&
                flow.key === activeFlow &&
                props.channelId === undefined &&
                'ring-2 ring-primary/30'
            )}
            style={{
              top: (poolPositions.get(flow.key) ?? 100) - 48,
              borderLeftColor: color(flow),
            }}
            onClick={() => props.onSelect(chooseTarget(flow))}
          >
            <span className='flex min-w-0 items-center justify-between gap-2'>
              <span className='truncate text-xs font-semibold'>
                {flow.route.model}
              </span>
              <Badge variant='secondary' className='shrink-0 text-[9px]'>
                {t(ROUTING_MODE_LABELS[flow.route.mode])}
              </Badge>
            </span>
            <span className='text-muted-foreground flex min-w-0 gap-1 text-[10px]'>
              <span className='truncate'>{flow.route.group}</span>
              <span>·</span>
              <span className='truncate'>
                {flow.route.policy
                  ? flow.targets[0].username || flow.route.policy
                  : t('Shared default pool')}
              </span>
            </span>
            <span className='flex items-center justify-between border-t pt-1.5 text-[10px]'>
              <span>
                {t('Observed selections')}:{' '}
                {formatNumber(flow.route.selection_count, locale)}
              </span>
              <span className='text-muted-foreground'>
                {formatNumber(flow.route.channels.length, locale)}{' '}
                {t('Channels')}
              </span>
            </span>
          </Button>
        ))}
        {[...channels].map(([key, channel]) => (
          <Button
            key={key}
            variant='outline'
            aria-label={t('View channel {{id}}', { id: channel.id })}
            className={cn(
              'absolute left-[75.9%] h-20 w-[21.45%] min-w-0 flex-col items-stretch gap-3 bg-card px-3 text-left shadow-none',
              props.focused &&
                !props.flows.some(
                  (flow) =>
                    channel.flows.has(flow.key) && related(flow, channel.id)
                ) &&
                'opacity-25'
            )}
            style={{ top: (channelPositions.get(key) ?? 100) - 40 }}
            onClick={() => props.onSelectChannel(channel.id)}
          >
            <span className='flex min-w-0 justify-between gap-2'>
              <span className='truncate text-xs font-semibold'>
                {channel.name}
              </span>
              <span className='text-muted-foreground text-[10px]'>
                #{channel.id}
              </span>
            </span>
            <span className='text-muted-foreground flex justify-between gap-2 text-[10px]'>
              <span>
                {formatNumber(channel.count, locale)} {t('Observed selections')}
              </span>
              <span>
                {formatNumber(channel.flows.size, locale)} {t('Routing pools')}
              </span>
            </span>
          </Button>
        ))}
        {edges.map(({ flow, channel }) => {
          const key = props.expanded
            ? `${flow.key}:${channel.channel_id}`
            : String(channel.channel_id)
          const y =
            ((poolPositions.get(flow.key) ?? 100) +
              (channelPositions.get(key) ?? 100)) /
            2
          return (
            <Button
              key={`${flow.key}:${channel.channel_id}`}
              variant='outline'
              size='xs'
              aria-label={t('Inspect {{model}} via channel {{id}}', {
                model: flow.route.model,
                id: channel.channel_id,
              })}
              className={cn(
                'absolute left-[64%] h-5 -translate-y-1/2 bg-card px-1.5 text-[10px] shadow-none',
                props.focused &&
                  !related(flow, channel.channel_id) &&
                  'opacity-15'
              )}
              style={{ top: y }}
              onClick={() =>
                props.onSelect(chooseTarget(flow), channel.channel_id)
              }
            >
              {formatNumber(channel.selected_share * 100, locale)}%
            </Button>
          )
        })}
      </div>
    </div>
  )
}
