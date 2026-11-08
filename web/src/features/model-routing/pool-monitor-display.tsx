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
import { lazy, Suspense, useState, type ReactNode } from 'react'
import { useTranslation } from 'react-i18next'

import { LoadingState } from '@/components/loading-state'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { cn } from '@/lib/utils'

import { RoutingWorkspaceDisplay } from './routing-workspace-display'

const RoutingMonitorWorkspace = lazy(() => import('@/features/routing-monitor'))
const EarlyChunkMonitor = lazy(() => import('./early-chunk-monitor'))

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
}) {
  const { t } = useTranslation()
  const [view, setView] = useState<'routing' | 'ttft'>('routing')
  const [headerContainer, setHeaderContainer] = useState<HTMLElement | null>(
    null
  )
  const groupLabel = props.groupLabel ?? props.group
  let identity = t('Default configuration')
  if (props.userId) {
    identity = `${props.username ?? t('User')} #${props.userId}`
  }
  return (
    <RoutingWorkspaceDisplay
      fullscreen={props.open}
      onFullscreenChange={props.onOpenChange}
      hideToolbar={!props.open}
      label={`${props.model} · ${groupLabel} · ${identity}`}
      exitFullscreenLabel={t('Back to card view')}
      toolbarActions={props.toolbarActions}
      toolbar={
        <div className='flex min-w-0 flex-1 flex-wrap items-center gap-3'>
          <div className='flex min-w-0 flex-wrap items-center gap-2'>
            <span className='min-w-0 text-base font-medium break-all'>
              {props.model}
            </span>
            <Badge variant='secondary'>{groupLabel}</Badge>
            <Badge variant='outline'>{identity}</Badge>
          </div>
          <div
            ref={setHeaderContainer}
            data-routing-monitor-header
            className='flex min-w-0 flex-1 flex-wrap items-center gap-2'
          />
          <div
            className='flex flex-wrap gap-1'
            aria-label={t('Monitoring view')}
          >
            <Button
              size='sm'
              variant={view === 'routing' ? 'secondary' : 'ghost'}
              aria-pressed={view === 'routing'}
              onClick={() => setView('routing')}
            >
              {t('Routing observations')}
            </Button>
            <Button
              size='sm'
              variant={view === 'ttft' ? 'secondary' : 'ghost'}
              aria-pressed={view === 'ttft'}
              onClick={() => setView('ttft')}
            >
              {t('TTFT prediction grid')}
            </Button>
          </div>
        </div>
      }
      refreshAction={null}
    >
      <div
        className={cn(
          'contents',
          props.open &&
            'grid min-h-0 flex-1 items-start gap-4 overflow-y-auto xl:grid-cols-[minmax(22rem,26rem)_minmax(0,1fr)] xl:grid-rows-[auto_1fr] xl:overflow-hidden'
        )}
      >
        <div
          key='pool-editor'
          className={cn(
            'contents',
            props.open &&
              'block min-w-0 order-2 xl:col-start-1 xl:row-start-2 xl:flex xl:h-full xl:min-h-0 xl:flex-col xl:overflow-hidden'
          )}
        >
          {props.children}
        </div>
        {props.open && (
          <Suspense
            fallback={
              <div className='order-3 min-w-0 xl:col-start-2 xl:row-span-2 xl:row-start-1'>
                <LoadingState message={t('Loading routing observations...')} />
              </div>
            }
          >
            {view === 'ttft' ? (
              <EarlyChunkMonitor
                model={props.model}
                channels={props.channels ?? []}
              />
            ) : (
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
