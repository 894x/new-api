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
import { useTranslation } from 'react-i18next'

import {
  sideDrawerContentClassName,
  sideDrawerHeaderClassName,
} from '@/components/drawer-layout'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Badge } from '@/components/ui/badge'
import { Separator } from '@/components/ui/separator'
import {
  Sheet,
  SheetContent,
  SheetDescription,
  SheetHeader,
  SheetTitle,
} from '@/components/ui/sheet'
import { Skeleton } from '@/components/ui/skeleton'
import { getChannelTypeLabel } from '@/features/channels/lib/channel-utils'

import { getModelChannelCapabilities } from '../../api'
import { modelsQueryKeys } from '../../lib'
import type { Model, ModelChannelCapability } from '../../types'

type ModelChannelCapabilitiesDrawerProps = {
  open: boolean
  onOpenChange: (open: boolean) => void
  model: Model | null
}

function CapabilityValue(props: {
  capability: NonNullable<
    ModelChannelCapability['parameter_capabilities']
  >[string]
}) {
  const { t } = useTranslation()
  const parts: string[] = []
  if (props.capability.supported === true) parts.push(t('Supported'))
  if (props.capability.supported === false) parts.push(t('Unsupported'))
  if (props.capability.min !== undefined) {
    parts.push(`${t('Minimum')}: ${props.capability.min}`)
  }
  if (props.capability.max !== undefined) {
    parts.push(`${t('Maximum')}: ${props.capability.max}`)
  }
  if (props.capability.allowed_values?.length) {
    parts.push(
      `${t('Allowed values')}: ${props.capability.allowed_values.join(', ')}`
    )
  }
  if (props.capability.on_violation) {
    parts.push(`${t('On violation')}: ${props.capability.on_violation}`)
  }
  return <span>{parts.length > 0 ? parts.join(' · ') : t('Configured')}</span>
}

function ChannelCapabilityCard(props: { channel: ModelChannelCapability }) {
  const { t } = useTranslation()
  const parameterEntries = Object.entries(
    props.channel.parameter_capabilities ?? {}
  ).sort(([left], [right]) => left.localeCompare(right))

  return (
    <section className='flex flex-col gap-4 rounded-lg border p-4'>
      <div className='flex flex-wrap items-start justify-between gap-3'>
        <div className='min-w-0'>
          <h3 className='truncate font-medium'>{props.channel.channel_name}</h3>
          <p className='text-muted-foreground text-xs'>
            #{props.channel.channel_id} ·{' '}
            {t(getChannelTypeLabel(props.channel.channel_type))}
          </p>
        </div>
        <Badge
          variant={props.channel.channel_status === 1 ? 'secondary' : 'outline'}
        >
          {props.channel.channel_status === 1 ? t('Enabled') : t('Disabled')}
        </Badge>
      </div>

      {props.channel.configuration_error && (
        <Alert variant='destructive'>
          <AlertTitle>{t('Configuration error')}</AlertTitle>
          <AlertDescription>
            {props.channel.configuration_error}
          </AlertDescription>
        </Alert>
      )}

      <div className='grid gap-3 sm:grid-cols-2 lg:grid-cols-4'>
        <div className='rounded-md border p-3'>
          <p className='text-muted-foreground text-xs'>{t('Priority')}</p>
          <p className='mt-1 font-medium'>{props.channel.effective_priority}</p>
          <p className='text-muted-foreground text-xs'>
            {t('Channel default')}: {props.channel.default_priority}
          </p>
        </div>
        <div className='rounded-md border p-3'>
          <p className='text-muted-foreground text-xs'>{t('Weight')}</p>
          <p className='mt-1 font-medium'>{props.channel.effective_weight}</p>
          <p className='text-muted-foreground text-xs'>
            {t('Channel default')}: {props.channel.default_weight}
          </p>
        </div>
        <div className='rounded-md border p-3 sm:col-span-2'>
          <p className='text-muted-foreground text-xs'>{t('Model mapping')}</p>
          <p className='mt-1 font-mono text-sm break-all'>
            {props.channel.model} → {props.channel.upstream_model}
          </p>
        </div>
      </div>

      <div className='flex flex-col gap-2'>
        <p className='text-sm font-medium'>{t('Groups')}</p>
        <div className='flex flex-wrap gap-2'>
          {props.channel.groups.map((group) => (
            <Badge
              key={group.group}
              variant={group.enabled ? 'secondary' : 'outline'}
            >
              {group.group}
              {!group.enabled && ` · ${t('Disabled')}`}
            </Badge>
          ))}
        </div>
      </div>

      <div className='flex flex-col gap-2'>
        <p className='text-sm font-medium'>{t('Endpoints')}</p>
        <div className='flex flex-wrap gap-2'>
          {props.channel.endpoint_types.map((endpoint) => (
            <Badge key={endpoint} variant='outline'>
              {endpoint}
            </Badge>
          ))}
        </div>
      </div>

      <Separator />

      <div className='grid gap-4 lg:grid-cols-2'>
        <div className='flex flex-col gap-2'>
          <p className='text-sm font-medium'>{t('Parameter capabilities')}</p>
          {props.channel.parameter_capabilities_configured ? (
            <div className='flex flex-col gap-2'>
              {parameterEntries.map(([parameter, capability]) => (
                <div key={parameter} className='rounded-md border p-3 text-sm'>
                  <p className='font-mono font-medium'>{parameter}</p>
                  <p className='text-muted-foreground mt-1 text-xs'>
                    <CapabilityValue capability={capability} />
                  </p>
                </div>
              ))}
            </div>
          ) : (
            <p className='text-muted-foreground text-sm'>
              {t('Not configured')}
            </p>
          )}
        </div>

        <div className='flex flex-col gap-2'>
          <p className='text-sm font-medium'>{t('Video capabilities')}</p>
          {props.channel.video_capabilities_configured ? (
            <div className='flex flex-wrap gap-2'>
              {props.channel.video_resolutions.map((resolution) => (
                <Badge key={resolution} variant='outline'>
                  {resolution}
                </Badge>
              ))}
            </div>
          ) : (
            <p className='text-muted-foreground text-sm'>
              {t('Not configured')}
            </p>
          )}
        </div>
      </div>
    </section>
  )
}

export function ModelChannelCapabilitiesDrawer(
  props: ModelChannelCapabilitiesDrawerProps
) {
  const { t } = useTranslation()
  const modelName = props.model?.model_name ?? ''
  const capabilityQuery = useQuery({
    queryKey: modelsQueryKeys.channelCapabilities(modelName),
    queryFn: () => getModelChannelCapabilities(modelName),
    enabled: props.open && modelName !== '',
  })
  const channels = capabilityQuery.data?.data?.channels ?? []
  const enabledChannelCount = channels.filter(
    (channel) => channel.channel_status === 1
  ).length
  const groups = new Set(
    channels.flatMap((channel) => channel.groups.map((group) => group.group))
  )

  return (
    <Sheet open={props.open} onOpenChange={props.onOpenChange}>
      <SheetContent className={sideDrawerContentClassName('sm:max-w-4xl')}>
        <SheetHeader className={sideDrawerHeaderClassName()}>
          <SheetTitle>{t('Model channel capabilities')}</SheetTitle>
          <SheetDescription>
            {t(
              'Read-only view of routing and effective channel capabilities for {{model}}.',
              { model: modelName }
            )}
          </SheetDescription>
        </SheetHeader>

        <div className='flex min-h-0 flex-1 flex-col gap-4 overflow-y-auto p-4'>
          {capabilityQuery.isLoading && (
            <div className='flex flex-col gap-3' aria-label={t('Loading')}>
              <Skeleton className='h-20 w-full' />
              <Skeleton className='h-64 w-full' />
            </div>
          )}

          {capabilityQuery.isError && (
            <Alert variant='destructive'>
              <AlertTitle>
                {t('Failed to load channel capabilities')}
              </AlertTitle>
              <AlertDescription>
                {capabilityQuery.error instanceof Error
                  ? capabilityQuery.error.message
                  : t('Please try again later.')}
              </AlertDescription>
            </Alert>
          )}

          {!capabilityQuery.isLoading && !capabilityQuery.isError && (
            <>
              <div className='grid gap-3 sm:grid-cols-3'>
                <div className='rounded-lg border p-3'>
                  <p className='text-muted-foreground text-xs'>
                    {t('Channels')}
                  </p>
                  <p className='mt-1 text-lg font-semibold'>
                    {channels.length}
                  </p>
                </div>
                <div className='rounded-lg border p-3'>
                  <p className='text-muted-foreground text-xs'>
                    {t('Enabled channels')}
                  </p>
                  <p className='mt-1 text-lg font-semibold'>
                    {enabledChannelCount}
                  </p>
                </div>
                <div className='rounded-lg border p-3'>
                  <p className='text-muted-foreground text-xs'>{t('Groups')}</p>
                  <p className='mt-1 text-lg font-semibold'>{groups.size}</p>
                </div>
              </div>

              {channels.length === 0 ? (
                <div className='text-muted-foreground rounded-lg border border-dashed p-8 text-center text-sm'>
                  {t('No channels support this exact model.')}
                </div>
              ) : (
                <div className='flex flex-col gap-4'>
                  {channels.map((channel) => (
                    <ChannelCapabilityCard
                      key={channel.channel_id}
                      channel={channel}
                    />
                  ))}
                </div>
              )}
            </>
          )}
        </div>
      </SheetContent>
    </Sheet>
  )
}
