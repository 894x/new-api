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
import {
  lazy,
  Suspense,
  useEffect,
  useEffectEvent,
  useState,
  type ReactNode,
} from 'react'
import { useTranslation } from 'react-i18next'

import { LoadingState } from '@/components/loading-state'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import {
  MonitorLayoutPicker,
  type MonitorLayout,
} from '@/features/routing-monitor/monitor-layout-picker'
import type { MonitorTarget } from '@/features/routing-monitor/monitor-panel'
import { monitorTargetKey } from '@/features/routing-monitor/pool-overview-data'
import { cn } from '@/lib/utils'

import { RoutingWorkspaceDisplay } from './routing-workspace-display'

const RoutingMonitorWorkspace = lazy(() => import('@/features/routing-monitor'))
const EarlyChunkMonitor = lazy(() => import('./early-chunk-monitor'))
const MonitorBoard = lazy(() =>
  import('@/features/routing-monitor/monitor-board').then((module) => ({
    default: module.MonitorBoard,
  }))
)
const PoolOverview = lazy(() =>
  import('@/features/routing-monitor/pool-overview').then((module) => ({
    default: module.PoolOverview,
  }))
)
const MonitorPoolEditor = lazy(() =>
  import('./monitor-pool-editor').then((module) => ({
    default: module.MonitorPoolEditor,
  }))
)

export function PoolMonitorDisplay(props: {
  open: boolean
  onOpenChange: (open: boolean) => void
  model: string
  group: string
  groupLabel?: string
  userId?: number
  username?: string
  children: ReactNode
  toolbarActions?: ReactNode
  channels?: { channel_id: number; channel_name: string }[]
  embedded?: boolean
  initialDirty?: boolean
  onDirtyChange?: (dirty: boolean) => void
}) {
  const { t } = useTranslation()
  const [view, setView] = useState<'routing' | 'ttft' | 'overview'>('routing')
  const [monitorLayout, setMonitorLayout] = useState<MonitorLayout>()
  const [headerContainer, setHeaderContainer] = useState<HTMLElement | null>(
    null
  )
  const groupLabel = props.groupLabel ?? props.group
  const initialTarget: MonitorTarget = {
    userId: props.userId,
    group: props.group,
    model: props.model,
  }
  const initialKey = monitorTargetKey(initialTarget)
  const [targets, setTargets] = useState<MonitorTarget[]>(() => [initialTarget])
  const [selectedTarget, setSelectedTarget] = useState(initialTarget)
  const [selectedChannel, setSelectedChannel] = useState<number>()
  const [channelPanelContainer, setChannelPanelContainer] =
    useState<HTMLElement | null>(null)
  const [dirtyPanels, setDirtyPanels] = useState<Record<string, boolean>>({})
  const overviewDirty = Object.values(dirtyPanels).some(Boolean)
  const notifyDirty = useEffectEvent((dirty: boolean) =>
    props.onDirtyChange?.(dirty)
  )
  useEffect(() => {
    notifyDirty(overviewDirty)
  }, [overviewDirty])
  useEffect(() => () => notifyDirty(false), [])
  const overview = view === 'overview' && !monitorLayout
  const selectedKey = monitorTargetKey(selectedTarget)
  let identity = t('Default configuration')
  let workspaceLabel = `${props.model} · ${groupLabel}`
  if (props.userId) {
    identity = `${props.username ?? t('User')} #${props.userId}`
  }
  workspaceLabel += ` · ${identity}`
  if (overview) workspaceLabel = t('Multi-pool overview')
  if (monitorLayout) workspaceLabel = t('Multi-panel monitoring')
  if (props.embedded) {
    return (
      <div className='flex min-h-0 flex-1 flex-col gap-2'>
        {props.toolbarActions && (
          <div className='flex shrink-0 flex-wrap gap-2'>
            {props.toolbarActions}
          </div>
        )}
        {props.children}
      </div>
    )
  }
  return (
    <RoutingWorkspaceDisplay
      fullscreen={props.open}
      onFullscreenChange={(open) => {
        if (!open) setSelectedChannel(undefined)
        props.onOpenChange(open)
      }}
      hideToolbar={!props.open}
      label={workspaceLabel}
      exitFullscreenLabel={t('Back to card view')}
      toolbarActions={
        <>
          {(!overview ||
            (selectedChannel === undefined && selectedKey === initialKey)) &&
            props.toolbarActions}
          {monitorLayout && (
            <Button
              variant='outline'
              onClick={() => {
                setMonitorLayout(undefined)
                setSelectedChannel(undefined)
              }}
            >
              {t('Single-panel mode')}
            </Button>
          )}
          <MonitorLayoutPicker
            value={monitorLayout}
            onChange={(layout) => {
              setMonitorLayout(layout)
              setSelectedChannel(undefined)
            }}
          />
        </>
      }
      toolbar={
        <div className='flex min-w-0 flex-1 flex-wrap items-center gap-3'>
          {monitorLayout && (
            <span className='font-medium'>{t('Multi-panel monitoring')}</span>
          )}
          {!monitorLayout && overview && (
            <span className='font-medium'>{t('Multi-pool overview')}</span>
          )}
          {!monitorLayout && !overview && (
            <div className='flex min-w-0 flex-wrap items-center gap-2'>
              <span className='min-w-0 text-base font-medium break-all'>
                {props.model}
              </span>
              <Badge variant='secondary'>{groupLabel}</Badge>
              <Badge variant='outline'>{identity}</Badge>
            </div>
          )}
          {!monitorLayout && !overview && (
            <div
              ref={setHeaderContainer}
              data-routing-monitor-header
              className='flex min-w-0 flex-1 flex-wrap items-center gap-2'
            />
          )}
          {!monitorLayout && (
            <div
              className='flex flex-wrap gap-1'
              aria-label={t('Monitoring view')}
            >
              <Button
                size='sm'
                variant={view === 'overview' ? 'secondary' : 'ghost'}
                aria-pressed={view === 'overview'}
                onClick={() => setView('overview')}
              >
                {t('Multi-pool overview')}
              </Button>
              <Button
                size='sm'
                variant={view === 'routing' ? 'secondary' : 'ghost'}
                aria-pressed={view === 'routing'}
                onClick={() => {
                  setView('routing')
                  setSelectedChannel(undefined)
                }}
              >
                {t('Routing observations')}
              </Button>
              <Button
                size='sm'
                variant={view === 'ttft' ? 'secondary' : 'ghost'}
                aria-pressed={view === 'ttft'}
                onClick={() => {
                  setView('ttft')
                  setSelectedChannel(undefined)
                }}
              >
                {t('TTFT prediction grid')}
              </Button>
            </div>
          )}
        </div>
      }
      refreshAction={null}
    >
      {props.open && monitorLayout && (
        <Suspense
          fallback={
            <LoadingState message={t('Loading routing observations...')} />
          }
        >
          <MonitorBoard layout={monitorLayout} userId={props.userId} />
        </Suspense>
      )}
      <div
        hidden={props.open && !!monitorLayout}
        className={cn(
          props.open && monitorLayout ? 'hidden' : 'contents',
          props.open &&
            !monitorLayout &&
            'grid min-h-0 flex-1 items-start gap-4 overflow-y-auto xl:grid-cols-[minmax(22rem,26rem)_minmax(0,1fr)] xl:grid-rows-[auto_1fr] xl:overflow-hidden'
        )}
      >
        <div
          key='pool-editor'
          data-pool-editor
          className={cn(
            'contents',
            props.open &&
              'block min-w-0 order-2 xl:col-start-1 xl:flex xl:h-full xl:min-h-0 xl:flex-col xl:overflow-hidden',
            props.open &&
              (view === 'ttft' || overview
                ? 'xl:row-span-2 xl:row-start-1'
                : 'xl:row-start-2')
          )}
        >
          <div
            ref={setChannelPanelContainer}
            hidden={!props.open || !overview || selectedChannel === undefined}
            className={
              props.open && overview && selectedChannel !== undefined
                ? 'flex min-h-0 flex-1 flex-col'
                : 'hidden'
            }
          />
          <div
            hidden={
              props.open &&
              overview &&
              (selectedKey !== initialKey || selectedChannel !== undefined)
            }
            className={
              props.open &&
              overview &&
              (selectedKey !== initialKey || selectedChannel !== undefined)
                ? 'hidden'
                : 'contents'
            }
          >
            {props.children}
          </div>
          {targets
            .filter((target) => monitorTargetKey(target) !== initialKey)
            .map((target) => {
              const key = monitorTargetKey(target)
              const active =
                props.open &&
                overview &&
                selectedChannel === undefined &&
                key === selectedKey
              return (
                <div
                  key={key}
                  hidden={!active}
                  className={active ? 'contents' : 'hidden'}
                >
                  <Suspense fallback={<LoadingState />}>
                    <MonitorPoolEditor
                      target={target}
                      onDirtyChange={(dirty) =>
                        setDirtyPanels((previous) =>
                          previous[key] === dirty
                            ? previous
                            : { ...previous, [key]: dirty }
                        )
                      }
                    />
                  </Suspense>
                </div>
              )
            })}
        </div>
        {props.open && !monitorLayout && (
          <Suspense
            fallback={
              <div className='order-3 min-w-0 xl:col-start-2 xl:row-span-2 xl:row-start-1'>
                <LoadingState message={t('Loading routing observations...')} />
              </div>
            }
          >
            {overview && (
              <div className='order-3 flex min-h-[32rem] min-w-0 flex-1 flex-col xl:col-start-2 xl:row-span-2 xl:row-start-1 xl:h-full xl:min-h-0'>
                <PoolOverview
                  targets={targets}
                  selectedTarget={selectedTarget}
                  channelPanelContainer={channelPanelContainer}
                  onSelectChannel={setSelectedChannel}
                  isDirty={(target) =>
                    monitorTargetKey(target) === initialKey
                      ? !!props.initialDirty
                      : !!dirtyPanels[monitorTargetKey(target)]
                  }
                  onSelect={setSelectedTarget}
                  onAdd={(target) => {
                    setTargets((previous) =>
                      previous.some(
                        (item) =>
                          monitorTargetKey(item) === monitorTargetKey(target)
                      )
                        ? previous
                        : [...previous, target]
                    )
                    setSelectedTarget(target)
                  }}
                  onRemove={(target) => {
                    const remaining = targets.filter(
                      (item) =>
                        monitorTargetKey(item) !== monitorTargetKey(target)
                    )
                    if (!remaining.length) return
                    setTargets(remaining)
                    if (monitorTargetKey(target) === selectedKey) {
                      setSelectedTarget(remaining[0])
                    }
                  }}
                />
              </div>
            )}
            {!overview && view === 'ttft' && (
              <EarlyChunkMonitor
                model={props.model}
                channels={props.channels ?? []}
              />
            )}
            {!overview && view !== 'ttft' && (
              <RoutingMonitorWorkspace
                userId={props.userId}
                fixedGroup={props.group}
                fixedModel={props.model}
                headerContainer={headerContainer}
                embedded
              />
            )}
          </Suspense>
        )}
      </div>
    </RoutingWorkspaceDisplay>
  )
}
