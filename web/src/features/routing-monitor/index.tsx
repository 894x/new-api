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
  Select,
  SelectTrigger,
  SelectValue,
  SelectContent,
  SelectItem,
} from '@/components/ui/select'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
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
  headerContainer?: HTMLElement | null
}

export default function RoutingMonitorWorkspace(
  props: RoutingMonitorWorkspaceProps
) {
  const { t, i18n } = useTranslation()
  const locale = toIntlLocale(i18n.resolvedLanguage || i18n.language)
  const client = useQueryClient()
  const [paused, setPaused] = useState(false)
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
  const activeChannels =
    route?.channels.filter((channel) => channel.active) ?? []
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
  let content = (
    <EmptyState
      title={t('No adaptive routing observations')}
      description={t(
        'Streaming requests using smart or discount-first routing will appear here. Priority routing and non-streaming requests are not included.'
      )}
    />
  )
  if (pending) {
    content = <LoadingState message={t('Loading routing observations...')} />
  } else if (hasError) {
    content = (
      <ErrorState
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
            'xl:flex xl:min-h-0 xl:flex-1 xl:flex-col xl:gap-4 xl:space-y-0'
        )}
      >
        <Card
          data-routing-monitor-flow=''
          className={props.embedded ? 'xl:min-h-0 xl:flex-1' : undefined}
        >
          <Tabs
            defaultValue='flow'
            className={props.embedded ? 'xl:min-h-0 xl:flex-1' : undefined}
          >
            <CardHeader className='flex shrink-0 flex-wrap items-center justify-between gap-3'>
              <CardTitle>{t('Live routing flow')}</CardTitle>
              <TabsList aria-label={t('Routing observation view')}>
                <TabsTrigger value='flow'>{t('Dynamic flow')}</TabsTrigger>
                <TabsTrigger value='channels'>
                  {t('Channel details')}
                </TabsTrigger>
              </TabsList>
            </CardHeader>
            <TabsContent
              value='flow'
              className={
                props.embedded ? 'xl:min-h-0 xl:overflow-auto' : undefined
              }
            >
              <RoutingFlow
                key={`${data.node?.id}-${route.group}-${route.model}-${route.policy}`}
                route={{ ...route, channels: activeChannels }}
                at={at}
                paused={paused || nodeStale}
                names={data.detail.data?.names}
                fill={props.embedded}
              />
            </TabsContent>
            <TabsContent
              value='channels'
              className={
                props.embedded ? 'xl:min-h-0 xl:overflow-auto' : undefined
              }
            >
              <CardContent>
                <ChannelDetails
                  route={route}
                  at={at}
                  names={data.detail.data?.names}
                />
              </CardContent>
            </TabsContent>
          </Tabs>
          <CardContent className='text-muted-foreground shrink-0 space-y-1 border-t pt-3 text-xs'>
            {t(
              'Line width represents target share. Dashed lines represent exploration or recovery probes.'
            )}
            <p>
              {t(
                'Selections include retries and local candidate decisions; they are not upstream request counts.'
              )}
            </p>
          </CardContent>
        </Card>
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
      </div>
    )
  }
  let selectedNodeLabel = t('No reporting nodes')
  if (data.node) {
    selectedNodeLabel =
      data.node.name + (data.node.local ? ` · ${t('Current node')}` : '')
  }
  if (data.nodeUnavailable && data.selectedNode) {
    selectedNodeLabel = `${data.selectedNode.name} · ${t('Unavailable')}`
  }
  const nodePicker = (
    <Select
      value={data.selectedNode?.id ?? data.node?.id ?? ''}
      onValueChange={(value) => {
        if (value) data.setNode(value)
      }}
    >
      <SelectTrigger
        aria-label={t('Monitor node')}
        className='max-w-full min-w-36 flex-1 sm:max-w-56'
      >
        <SelectValue>{selectedNodeLabel}</SelectValue>
      </SelectTrigger>
      <SelectContent alignItemWithTrigger={false}>
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
      <Button
        type='button'
        variant='outline'
        disabled={paused || data.monitor.isFetching}
        onClick={refresh}
      >
        {t('Refresh')}
      </Button>
    </>
  )
  const overview = (
    <RoutingOverview
      route={hasError || data.nodeUnavailable ? undefined : route}
      at={at}
      names={data.detail.data?.names}
      loading={pending}
      embedded={props.embedded}
    />
  )
  const header = (
    <div
      data-routing-monitor-controls=''
      className={
        props.embedded
          ? 'flex min-w-0 flex-1 flex-wrap items-center gap-x-3 gap-y-1'
          : 'space-y-3'
      }
    >
      {props.embedded && (
        <>
          {nodePicker}
          {actions}
        </>
      )}
      <div className='flex flex-wrap items-start justify-between gap-3'>
        {!props.embedded && (
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
