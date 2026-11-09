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
  AnalyticsUpIcon,
  Clock01Icon,
  Database01Icon,
  Flowchart01Icon,
  InformationCircleIcon,
  RefreshIcon,
  ServerStack01Icon,
  Table01Icon,
  UserIcon,
} from '@hugeicons/core-free-icons'
import { HugeiconsIcon } from '@hugeicons/react'
import { useQueryClient } from '@tanstack/react-query'
import { useState } from 'react'
import { createPortal } from 'react-dom'
import { useTranslation } from 'react-i18next'

import { EmptyState } from '@/components/empty-state'
import { ErrorState } from '@/components/error-state'
import { LoadingState } from '@/components/loading-state'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Combobox } from '@/components/ui/combobox'
import { NativeSelect, NativeSelectOption } from '@/components/ui/native-select'
import {
  Popover,
  PopoverContent,
  PopoverTitle,
  PopoverTrigger,
} from '@/components/ui/popover'
import {
  Select,
  SelectTrigger,
  SelectValue,
  SelectContent,
  SelectItem,
} from '@/components/ui/select'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from '@/components/ui/tooltip'
import { RoutingWorkspaceDisplay } from '@/features/model-routing/routing-workspace-display'
import { toIntlLocale } from '@/i18n/languages'
import { cn } from '@/lib/utils'

import { monitorKey } from './api'
import { ChannelDetails, RoutingEvents, ShareHistory } from './details'
import { RoutingFlow } from './flow'
import { RoutingOverview } from './overview'
import { useMonitorData } from './use-monitor-data'

export interface RoutingMonitorWorkspaceProps {
  userId?: number
  fixedGroup?: string
  fixedModel?: string
  embedded?: boolean
  panel?: boolean
  headerContainer?: HTMLElement | null
}

export default function RoutingMonitorWorkspace(
  props: RoutingMonitorWorkspaceProps
) {
  const { t, i18n } = useTranslation()
  const locale = toIntlLocale(i18n.resolvedLanguage || i18n.language)
  const client = useQueryClient()
  const [paused, setPaused] = useState(false)
  const [view, setView] = useState('flow')
  const fixedIdentity =
    props.embedded ||
    props.fixedGroup !== undefined ||
    props.fixedModel !== undefined
      ? { group: props.fixedGroup ?? '', model: props.fixedModel ?? '' }
      : undefined
  const data = useMonitorData(props.userId, paused, fixedIdentity)
  const snapshot = data.monitor.data
  const route = data.route
  const at = snapshot?.generated_at ?? 0
  const dispatchedChannels = new Set(
    route?.requests?.map((request) => request.channel_id)
  )
  const activeChannels =
    route?.channels.filter(
      (channel) =>
        route?.mode === 'priority' ||
        channel.active ||
        dispatchedChannels.has(channel.channel_id)
    ) ?? []
  const inherited =
    !!props.userId && data.detail.data && data.detail.data.source !== 'user'
  const nodeStale = !!data.node && at - data.node.updated_at > 20000
  const source = data.detail.data?.source
  const hasError =
    data.monitor.isError ||
    (!data.fixed && data.catalog.isError) ||
    (!data.fixed && !props.userId && data.groupCatalog.isError) ||
    data.detail.isError
  const refresh = () => {
    void data.monitor.refetch()
    if (!data.fixed) void data.catalog.refetch()
    if (data.group && data.model) void data.detail.refetch()
    if (!data.fixed && !props.userId && data.group) {
      void data.groupCatalog.refetch()
    }
  }
  const pending =
    data.monitor.isPending ||
    (!data.fixed && data.catalog.isPending) ||
    (data.detail.isPending && !!data.model)
  const unavailable = hasError || nodeStale || data.nodeUnavailable
  const statusLabel = paused
    ? t('Paused')
    : t(unavailable ? 'Updates unavailable' : 'Live updates')
  const identityLabel = props.userId
    ? t('User #{{id}}', { id: props.userId })
    : t('Default configuration')
  const viewTitles: Record<string, string> = {
    flow: t('Live routing flow'),
    channels: t('Channel details'),
    history: t('Traffic share history'),
    events: t('Routing changes'),
  }
  const stateClassName = props.panel
    ? 'min-h-0 gap-2 p-3 [&_[data-slot=empty-icon]]:hidden [&_[data-slot=empty-description]]:text-xs'
    : undefined
  const flowHelp = route ? (
    <>
      <p>
        {t(
          'Each dot represents one upstream attempt on its channel, including retries.'
        )}
      </p>
      {route.mode === 'priority'
        ? t(
            'Line width represents observed traffic share. Priority and weight determine selection; channel auto-disable and recovery rules remain in effect.'
          )
        : t(
            route.aggregate
              ? 'Line width represents selection share across reporting nodes. Select a node to inspect its target allocation and quality.'
              : 'Line width represents target share. Dashed lines represent exploration or recovery probes.'
          )}
      <p>
        {route.mode === 'priority'
          ? t('Counts represent physical upstream attempts, including retries.')
          : t(
              'Selections include retries and local candidate decisions; they are not upstream request counts.'
            )}
      </p>
    </>
  ) : null
  let content = (
    <EmptyState
      className={stateClassName}
      title={t('No routing observations')}
      description={t(
        'Priority routing requests and streaming requests using smart or discount-first routing will appear here.'
      )}
    />
  )
  if (pending) {
    content = (
      <LoadingState
        className={props.panel ? 'min-h-0 p-3' : undefined}
        message={t('Loading routing observations...')}
      />
    )
  } else if (hasError) {
    content = (
      <ErrorState
        className={stateClassName}
        title={t('Unable to refresh routing observations')}
        description={t(
          'Live flow is stopped until monitoring data can be refreshed.'
        )}
        onRetry={refresh}
      />
    )
  } else if (data.nodeUnavailable) {
    content = (
      <EmptyState
        className={stateClassName}
        title={t('Updates unavailable')}
        description={t(
          'The selected node is unavailable. Select another node to continue.'
        )}
      />
    )
  } else if (route) {
    content = (
      <div
        className={cn(
          'space-y-4',
          props.embedded &&
            'xl:flex xl:min-h-0 xl:flex-1 xl:flex-col xl:gap-4 xl:space-y-0',
          props.panel && 'flex min-h-0 flex-1 flex-col space-y-0'
        )}
      >
        <Card
          data-routing-monitor-flow=''
          className={cn(
            props.embedded && 'xl:min-h-0 xl:flex-1',
            props.panel && 'relative min-h-0 flex-1 gap-0 py-0'
          )}
        >
          <Tabs
            value={view}
            onValueChange={(value) => {
              if (typeof value === 'string') setView(value)
            }}
            className={cn(
              props.embedded && 'xl:min-h-0 xl:flex-1',
              props.panel && 'relative min-h-0 flex-1 gap-0 overflow-hidden'
            )}
          >
            <CardHeader
              className={cn(
                'flex shrink-0 flex-wrap items-center justify-between gap-3',
                props.panel &&
                  'pointer-events-none absolute inset-x-3 top-3 z-10 flex-nowrap gap-2 rounded-none p-0'
              )}
            >
              <CardTitle
                title={props.panel ? viewTitles[view] : t('Live routing flow')}
                className={
                  props.panel
                    ? 'bg-background/85 ring-foreground/10 hidden max-w-[45%] min-w-0 truncate rounded-full px-2.5 py-1 text-xs shadow-sm ring-1 backdrop-blur-md @min-[18rem]/monitor-panel:block'
                    : undefined
                }
              >
                {props.panel ? viewTitles[view] : t('Live routing flow')}
              </CardTitle>
              <TabsList
                aria-label={t('Routing observation view')}
                className={
                  props.panel
                    ? 'bg-background/85 ring-foreground/10 pointer-events-auto ml-auto shrink-0 rounded-full shadow-sm ring-1 backdrop-blur-md [&_[data-slot=tabs-trigger]]:rounded-full'
                    : undefined
                }
              >
                <Tooltip>
                  <TabsTrigger
                    value='flow'
                    aria-label={t('Dynamic flow')}
                    render={props.panel ? <TooltipTrigger /> : undefined}
                    className={props.panel ? 'w-7 flex-none px-0' : undefined}
                  >
                    {props.panel ? (
                      <HugeiconsIcon
                        icon={Flowchart01Icon}
                        aria-hidden='true'
                      />
                    ) : (
                      t('Dynamic flow')
                    )}
                  </TabsTrigger>
                  {props.panel && (
                    <TooltipContent>{t('Dynamic flow')}</TooltipContent>
                  )}
                </Tooltip>
                <Tooltip>
                  <TabsTrigger
                    value='channels'
                    aria-label={t('Channel details')}
                    render={props.panel ? <TooltipTrigger /> : undefined}
                    className={props.panel ? 'w-7 flex-none px-0' : undefined}
                  >
                    {props.panel ? (
                      <HugeiconsIcon icon={Table01Icon} aria-hidden='true' />
                    ) : (
                      t('Channel details')
                    )}
                  </TabsTrigger>
                  {props.panel && (
                    <TooltipContent>{t('Channel details')}</TooltipContent>
                  )}
                </Tooltip>
                {props.panel && (
                  <>
                    <Tooltip>
                      <TabsTrigger
                        value='history'
                        aria-label={t('Traffic share history')}
                        render={<TooltipTrigger />}
                        className='w-7 flex-none px-0'
                      >
                        <HugeiconsIcon
                          icon={AnalyticsUpIcon}
                          aria-hidden='true'
                        />
                      </TabsTrigger>
                      <TooltipContent>
                        {t('Traffic share history')}
                      </TooltipContent>
                    </Tooltip>
                    <Tooltip>
                      <TabsTrigger
                        value='events'
                        aria-label={t('Routing changes')}
                        render={<TooltipTrigger />}
                        className='w-7 flex-none px-0'
                      >
                        <HugeiconsIcon icon={Clock01Icon} aria-hidden='true' />
                      </TabsTrigger>
                      <TooltipContent>{t('Routing changes')}</TooltipContent>
                    </Tooltip>
                  </>
                )}
              </TabsList>
            </CardHeader>
            <TabsContent
              value='flow'
              className={cn(
                props.embedded && 'xl:min-h-0 xl:overflow-auto',
                props.panel && 'min-h-0 overflow-auto overscroll-contain'
              )}
            >
              <RoutingFlow
                key={`${data.node?.id}-${route.group}-${route.model}-${route.policy}`}
                route={{ ...route, channels: activeChannels }}
                at={at}
                paused={paused || nodeStale}
                names={data.detail.data?.names}
                fill={props.embedded}
                panel={props.panel}
              />
            </TabsContent>
            <TabsContent
              value='channels'
              className={cn(
                props.embedded && 'xl:min-h-0 xl:overflow-auto',
                props.panel &&
                  'min-h-0 min-w-0 overflow-auto overscroll-contain pt-12 pb-3'
              )}
            >
              <CardContent>
                <ChannelDetails
                  route={route}
                  at={at}
                  names={data.detail.data?.names}
                />
              </CardContent>
            </TabsContent>
            {props.panel && (
              <>
                <TabsContent
                  value='history'
                  className='min-h-0 overflow-hidden pt-12 pb-2'
                >
                  <ShareHistory
                    route={route}
                    names={data.detail.data?.names}
                    compact
                  />
                </TabsContent>
                <TabsContent
                  value='events'
                  className='min-h-0 overflow-hidden pt-12 pb-2'
                >
                  <RoutingEvents
                    route={route}
                    names={data.detail.data?.names}
                    compact
                  />
                </TabsContent>
              </>
            )}
          </Tabs>
          {!props.panel && (
            <CardContent className='text-muted-foreground shrink-0 space-y-1 border-t pt-3 text-xs'>
              {flowHelp}
            </CardContent>
          )}
        </Card>
        {!props.panel && (
          <div
            data-routing-monitor-history-row=''
            className={cn(
              'grid min-w-0 gap-4 xl:grid-cols-[minmax(0,1.3fr)_minmax(0,1fr)]',
              props.embedded
                ? 'xl:h-[clamp(15rem,26vh,20rem)] xl:min-h-0 xl:shrink-0 xl:items-stretch'
                : 'items-start'
            )}
          >
            <ShareHistory
              route={route}
              names={data.detail.data?.names}
              fill={props.embedded}
            />
            <RoutingEvents
              route={route}
              names={data.detail.data?.names}
              fill={props.embedded}
            />
          </div>
        )}
      </div>
    )
  }
  let selectedNodeLabel = t('All nodes (combined)')
  if (data.node) {
    selectedNodeLabel =
      data.node.name + (data.node.local ? ` · ${t('Current node')}` : '')
  }
  if (data.nodeUnavailable && data.selectedNode) {
    selectedNodeLabel = `${data.selectedNode.name} · ${t('Unavailable')}`
  }
  const nodePicker = (
    <Tooltip>
      <Select
        value={data.selectedNode?.id ?? '__all'}
        onValueChange={(value) => {
          if (value) data.setNode(value)
        }}
      >
        <SelectTrigger
          aria-label={t('Monitor node')}
          render={props.panel ? <TooltipTrigger /> : undefined}
          className={cn(
            'max-w-full min-w-36 flex-1 sm:max-w-56',
            props.panel &&
              'size-6 min-w-0 flex-none justify-center gap-0 border-transparent p-0 data-[size=default]:h-6 sm:max-w-none [&>svg:last-child]:hidden'
          )}
        >
          {props.panel ? (
            <HugeiconsIcon icon={ServerStack01Icon} aria-hidden='true' />
          ) : (
            <SelectValue>{selectedNodeLabel}</SelectValue>
          )}
        </SelectTrigger>
        <SelectContent alignItemWithTrigger={false}>
          <SelectItem value='__all'>{t('All nodes (combined)')}</SelectItem>
          {data.nodes.length === 0 && !data.selectedNode && (
            <SelectItem value='__none' disabled>
              {t('No reporting nodes')}
            </SelectItem>
          )}
          {data.nodeUnavailable && data.selectedNode && (
            <SelectItem value={data.selectedNode.id} disabled>
              {data.selectedNode.name} · {t('Unavailable')}
            </SelectItem>
          )}
          {data.nodes.map((node) => (
            <SelectItem key={node.id} value={node.id}>
              {node.name}
              {node.local ? ` · ${t('Current node')}` : ''}
            </SelectItem>
          ))}
        </SelectContent>
      </Select>
      {props.panel && (
        <TooltipContent>
          {t('Monitor node')}: {selectedNodeLabel}
        </TooltipContent>
      )}
    </Tooltip>
  )
  const toolbar = (
    <div className='flex min-w-0 flex-1 flex-wrap items-center gap-2'>
      <Combobox
        className='h-8 min-w-40 flex-1 sm:max-w-64'
        aria-label={t('Monitor model')}
        placeholder={t('Select model')}
        value={data.model}
        options={data.models.map((value) => ({ value, label: value }))}
        onValueChange={(value) => data.setModel(value ?? '')}
      />
      <NativeSelect
        aria-label={t('Monitor group')}
        value={data.group}
        onChange={(event) => data.setGroup(event.target.value)}
        className='max-w-full min-w-28 flex-1 sm:max-w-48'
      >
        {data.groups.length === 0 && (
          <NativeSelectOption value=''>{t('Select group')}</NativeSelectOption>
        )}
        {data.groups.map((group) => (
          <NativeSelectOption key={group} value={group}>
            {data.catalog.data?.groups[group] || group}
          </NativeSelectOption>
        ))}
      </NativeSelect>
      {nodePicker}
    </div>
  )
  const actions = (
    <>
      {!props.panel && (
        <Button
          type='button'
          variant='outline'
          aria-pressed={paused}
          onClick={() => {
            if (!paused) {
              void client.cancelQueries({
                queryKey: monitorKey(props.userId),
                exact: true,
              })
            }
            setPaused((previous) => !previous)
          }}
        >
          {paused ? t('Resume live updates') : t('Pause live updates')}
        </Button>
      )}
      {props.panel ? (
        <Tooltip>
          <TooltipTrigger
            render={
              <Button
                type='button'
                variant='ghost'
                size='icon-xs'
                aria-label={t('Refresh')}
                disabled={data.monitor.isFetching}
                onClick={refresh}
              />
            }
          >
            <HugeiconsIcon icon={RefreshIcon} aria-hidden='true' />
          </TooltipTrigger>
          <TooltipContent>{t('Refresh')}</TooltipContent>
        </Tooltip>
      ) : (
        <Button
          type='button'
          variant='outline'
          disabled={paused || data.monitor.isFetching}
          onClick={refresh}
        >
          {t('Refresh')}
        </Button>
      )}
    </>
  )
  const overview = (
    <RoutingOverview
      route={hasError || data.nodeUnavailable ? undefined : route}
      at={at}
      names={data.detail.data?.names}
      loading={pending}
      embedded={props.embedded}
      compact={props.panel}
    />
  )
  const details = (
    <>
      <div className='flex flex-wrap items-start justify-between gap-3'>
        {!props.embedded && !props.panel && (
          <div>
            <h2 className='text-xl font-semibold'>
              {t('Routing observatory')}
            </h2>
            <p className='text-muted-foreground mt-1 text-sm'>
              {t('Observe allocation changes and their causes')}
            </p>
          </div>
        )}
        <div className='flex flex-wrap items-center gap-2'>
          {!props.embedded && (
            <Badge variant='outline'>
              {props.userId
                ? t('User #{{id}}', { id: props.userId })
                : t('Default configuration')}
            </Badge>
          )}
          {props.userId && source && (
            <Badge variant='secondary'>
              {source === 'user'
                ? t('User override')
                : t('Inherited base pool')}
            </Badge>
          )}
          <Badge
            variant={
              hasError || nodeStale || data.nodeUnavailable
                ? 'warning'
                : 'outline'
            }
          >
            {paused ? (
              t('Paused')
            ) : (
              <UpdateStatus
                unavailable={hasError || nodeStale || data.nodeUnavailable}
              />
            )}
          </Badge>
        </div>
      </div>
      {inherited && (
        <p
          role='status'
          className={
            props.embedded
              ? 'text-muted-foreground text-xs'
              : 'bg-muted rounded-lg px-3 py-2 text-sm'
          }
        >
          {t(
            'This user inherits the base pool. The displayed traffic is shared base-pool traffic, not traffic attributed only to this user.'
          )}
        </p>
      )}
      {snapshot?.coverage === 'local' && (
        <p className='text-muted-foreground text-xs'>
          {t('Current node only. Cluster snapshots are unavailable.')}
        </p>
      )}
      {snapshot?.partial && (
        <p role='status' className='text-warning text-xs'>
          {t(
            'Some node snapshots are unavailable. This view may be incomplete.'
          )}
        </p>
      )}
      {route?.requests_truncated && (
        <p role='status' className='text-warning text-xs'>
          {t(
            'Some request events exceeded the live display limit. Traffic counters remain available.'
          )}
        </p>
      )}
      {nodeStale && (
        <p role='status' className='text-warning text-xs'>
          {t('The selected node snapshot is stale. Flow animation is stopped.')}
        </p>
      )}
      {snapshot && (
        <p className='text-muted-foreground text-xs tabular-nums'>
          {t('Snapshot time')}:{' '}
          {new Date(at).toLocaleTimeString(locale, {
            hour: '2-digit',
            minute: '2-digit',
            second: '2-digit',
          })}
        </p>
      )}
      {route?.truncated && (
        <p role='status' className='text-warning text-xs'>
          {t(
            'This route exceeds the monitoring detail limit. Some channel details are omitted.'
          )}
        </p>
      )}
    </>
  )
  const header = (
    <div
      data-routing-monitor-controls=''
      className={cn(
        props.embedded
          ? 'flex min-w-0 flex-1 flex-wrap items-center gap-x-3 gap-y-1'
          : 'space-y-3',
        props.panel && 'flex w-max flex-none items-center gap-1 space-y-0'
      )}
    >
      {(props.embedded || props.panel) && (
        <>
          {nodePicker}
          {actions}
        </>
      )}
      {props.panel ? (
        <>
          <Badge
            variant={inherited ? 'warning' : 'outline'}
            aria-label={identityLabel}
            title={identityLabel}
            className='gap-1 px-1.5'
          >
            <HugeiconsIcon
              icon={props.userId ? UserIcon : Database01Icon}
              aria-hidden='true'
            />
            {props.userId && (
              <span className='hidden @min-[20rem]/monitor-panel:inline'>
                #{props.userId}
              </span>
            )}
          </Badge>
          <span
            role='status'
            aria-label={statusLabel}
            title={statusLabel}
            className={cn(
              'mx-1 size-1.5 shrink-0 rounded-full',
              unavailable ? 'bg-warning' : 'bg-success'
            )}
          >
            <span className='sr-only'>{statusLabel}</span>
          </span>
          <Popover>
            <Tooltip>
              <PopoverTrigger
                render={
                  <TooltipTrigger
                    render={
                      <Button
                        variant='ghost'
                        size='icon-xs'
                        aria-label={t('Routing observations')}
                        className={cn(
                          (inherited ||
                            unavailable ||
                            snapshot?.partial ||
                            route?.truncated ||
                            route?.requests_truncated) &&
                            'text-warning'
                        )}
                      />
                    }
                  />
                }
              >
                <HugeiconsIcon
                  icon={InformationCircleIcon}
                  aria-hidden='true'
                />
              </PopoverTrigger>
              <TooltipContent>{t('Routing observations')}</TooltipContent>
            </Tooltip>
            <PopoverContent
              align='end'
              className='w-80 max-w-[calc(100vw-2rem)] space-y-3 whitespace-normal'
            >
              <PopoverTitle>{t('Routing observations')}</PopoverTitle>
              <p className='text-muted-foreground text-xs'>
                {t('Monitor node')}: {selectedNodeLabel}
              </p>
              {details}
              {flowHelp && (
                <div className='text-muted-foreground space-y-2 border-t pt-3 text-xs'>
                  {flowHelp}
                </div>
              )}
            </PopoverContent>
          </Popover>
        </>
      ) : (
        details
      )}
    </div>
  )
  const body = (
    <div
      className={
        props.embedded
          ? 'flex min-h-0 min-w-0 flex-1 flex-col'
          : 'min-h-0 flex-1 space-y-4 overflow-y-auto overscroll-contain p-1'
      }
    >
      {!props.embedded && header}
      {!props.embedded && overview}
      {content}
    </div>
  )
  if (props.panel) {
    return (
      <>
        {props.headerContainer && createPortal(header, props.headerContainer)}
        <section
          aria-label={t('Live routing flow')}
          className='flex min-h-0 min-w-0 flex-1 flex-col gap-2 overflow-hidden p-2'
        >
          {!props.headerContainer && header}
          <div className='flex min-h-0 flex-1 flex-col justify-center overflow-auto overscroll-contain'>
            {content}
          </div>
          {overview}
        </section>
      </>
    )
  }
  if (props.embedded) {
    return (
      <>
        {props.headerContainer && createPortal(header, props.headerContainer)}
        {overview}
        <section
          aria-label={t('Live routing flow')}
          data-routing-monitor-live=''
          className='order-3 flex min-h-0 min-w-0 flex-col gap-3 xl:col-start-2 xl:row-span-2 xl:row-start-1 xl:h-full xl:overflow-hidden'
        >
          {!props.headerContainer && header}
          {body}
        </section>
      </>
    )
  }
  return (
    <RoutingWorkspaceDisplay toolbar={toolbar} refreshAction={actions}>
      {body}
    </RoutingWorkspaceDisplay>
  )
}

function UpdateStatus(props: { unavailable: boolean }) {
  const { t } = useTranslation()
  return props.unavailable ? t('Updates unavailable') : t('Live updates')
}
