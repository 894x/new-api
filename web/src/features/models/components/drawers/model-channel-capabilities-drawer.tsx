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
import { ChannelStatusFilter } from '@/features/channels/components/channel-status-filter'
import { getChannelTypeLabel } from '@/features/channels/lib/channel-utils'

import { getModelChannelCapabilities } from '../../api'
import { modelsQueryKeys } from '../../lib'
import type { Model, ModelChannelCapability } from '../../types'

const PARAMETER_OVERRIDE_MODE_LABELS: Record<string, string> = {
  set: 'Set',
  delete: 'Delete',
  copy: 'Copy',
  move: 'Move',
  append: 'Append',
  prepend: 'Prepend',
  trim_prefix: 'Trim Prefix',
  trim_suffix: 'Trim Suffix',
  ensure_prefix: 'Ensure Prefix',
  ensure_suffix: 'Ensure Suffix',
  trim_space: 'Trim Space',
  to_lower: 'To Lower',
  to_upper: 'To Upper',
  replace: 'Replace',
  regex_replace: 'Regex Replace',
  set_header: 'Set Header',
  delete_header: 'Delete Header',
  copy_header: 'Copy Header',
  move_header: 'Move Header',
  pass_headers: 'Pass Headers',
  sync_fields: 'Sync Fields',
  return_error: 'Return Error',
  prune_objects: 'Prune Object Items',
}

const PARAMETER_OVERRIDE_FORMAT_LABELS: Record<string, string> = {
  legacy: 'Legacy',
  operations: 'Operations format',
  mixed: 'Mixed',
}

const PARAMETER_OVERRIDE_REDACTED_VALUE = '[REDACTED]'

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

function formatOverrideValue(value: unknown, hiddenLabel: string): string {
  if (value === PARAMETER_OVERRIDE_REDACTED_VALUE) return hiddenLabel
  if (typeof value === 'string') return value
  if (value === undefined) return ''
  try {
    return JSON.stringify(value, null, 2)
  } catch {
    return String(value)
  }
}

function ParameterOverrideRules(props: { channel: ModelChannelCapability }) {
  const { t } = useTranslation()
  const operations = props.channel.parameter_override_operations ?? []
  const legacyEntries = Object.entries(
    props.channel.parameter_override_legacy ?? {}
  ).sort(([left], [right]) => left.localeCompare(right))

  return (
    <div className='flex flex-col gap-3'>
      <div className='flex flex-wrap items-center justify-between gap-2'>
        <p className='text-sm font-medium'>{t('Parameter override rules')}</p>
        {props.channel.parameter_override_configured && (
          <Badge variant='outline'>
            {t(
              PARAMETER_OVERRIDE_FORMAT_LABELS[
                props.channel.parameter_override_mode
              ] ?? props.channel.parameter_override_mode
            )}
          </Badge>
        )}
      </div>

      {!props.channel.parameter_override_configured ? (
        <p className='text-muted-foreground text-sm'>{t('Not configured')}</p>
      ) : (
        <>
          {operations.length > 0 && (
            <Alert>
              <AlertTitle>{t('Evaluated at request time')}</AlertTitle>
              <AlertDescription>
                {t(
                  'Conditions use the request body and routing context, so the final result can vary by request.'
                )}
              </AlertDescription>
            </Alert>
          )}

          {legacyEntries.length > 0 && (
            <div className='flex flex-col gap-2'>
              <p className='text-muted-foreground text-xs font-medium'>
                {t('Legacy overrides')}
              </p>
              <div className='grid gap-2 sm:grid-cols-2'>
                {legacyEntries.map(([path, value]) => (
                  <div key={path} className='rounded-md border p-3 text-sm'>
                    <p className='font-mono font-medium'>{path}</p>
                    <pre className='text-muted-foreground mt-1 max-h-32 overflow-auto text-xs break-all whitespace-pre-wrap'>
                      {formatOverrideValue(value, t('Hidden'))}
                    </pre>
                  </div>
                ))}
              </div>
            </div>
          )}

          {operations.length > 0 && (
            <div className='flex flex-col gap-2'>
              <p className='text-muted-foreground text-xs font-medium'>
                {t('Ordered operations')}
              </p>
              {operations.map((operation, index) => (
                <div
                  key={operation.order}
                  className='flex flex-col gap-3 rounded-md border p-3'
                >
                  <div className='flex flex-wrap items-start justify-between gap-2'>
                    <div>
                      <p className='text-sm font-medium'>
                        {operation.description ||
                          t('Rule {{number}}', { number: index + 1 })}
                      </p>
                      {operation.description && (
                        <p className='text-muted-foreground text-xs'>
                          {t('Rule {{number}}', { number: index + 1 })}
                        </p>
                      )}
                    </div>
                    <Badge variant='secondary'>
                      {t(
                        PARAMETER_OVERRIDE_MODE_LABELS[operation.mode] ??
                          operation.mode
                      )}
                    </Badge>
                  </div>

                  <div className='grid gap-2 text-xs sm:grid-cols-2 lg:grid-cols-4'>
                    {operation.path && (
                      <div>
                        <p className='text-muted-foreground'>{t('Path')}</p>
                        <p className='font-mono break-all'>{operation.path}</p>
                      </div>
                    )}
                    {operation.from && (
                      <div>
                        <p className='text-muted-foreground'>{t('Source')}</p>
                        <p className='font-mono break-all'>{operation.from}</p>
                      </div>
                    )}
                    {operation.to && (
                      <div>
                        <p className='text-muted-foreground'>{t('Target')}</p>
                        <p className='font-mono break-all'>{operation.to}</p>
                      </div>
                    )}
                    {(operation.value_configured ??
                      operation.value !== undefined) && (
                      <div>
                        <p className='text-muted-foreground'>{t('Value')}</p>
                        <pre className='max-h-32 overflow-auto font-mono break-all whitespace-pre-wrap'>
                          {formatOverrideValue(operation.value, t('Hidden'))}
                        </pre>
                      </div>
                    )}
                  </div>

                  <div className='flex flex-wrap gap-2'>
                    {operation.keep_origin && (
                      <Badge variant='outline'>
                        {t('Keep original value')}
                      </Badge>
                    )}
                    {(operation.conditions ?? []).length === 0 && (
                      <Badge variant='outline'>{t('Always applies')}</Badge>
                    )}
                  </div>

                  {(operation.conditions ?? []).length > 0 && (
                    <div className='flex flex-col gap-2'>
                      <p className='text-muted-foreground text-xs font-medium'>
                        {t('Conditions')} · {operation.logic || 'OR'}
                      </p>
                      {(operation.conditions ?? []).map((condition) => (
                        <div
                          key={condition.order}
                          className='bg-muted/50 flex flex-wrap items-center gap-2 rounded-md px-3 py-2 text-xs'
                        >
                          <code>{condition.path}</code>
                          <Badge variant='outline'>{condition.mode}</Badge>
                          <code className='break-all'>
                            {formatOverrideValue(condition.value, t('Hidden'))}
                          </code>
                          {condition.invert && (
                            <Badge variant='outline'>{t('Inverted')}</Badge>
                          )}
                          {condition.pass_missing_key && (
                            <Badge variant='outline'>
                              {t('Pass when missing')}
                            </Badge>
                          )}
                        </div>
                      ))}
                    </div>
                  )}
                </div>
              ))}
            </div>
          )}
        </>
      )}
    </div>
  )
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

      <Separator />

      <ParameterOverrideRules channel={props.channel} />
    </section>
  )
}

export function ModelChannelCapabilitiesDrawer(
  props: ModelChannelCapabilitiesDrawerProps
) {
  const { t } = useTranslation()
  const [showAllChannels, setShowAllChannels] = useState(false)
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
  const visibleChannels = showAllChannels
    ? channels
    : channels.filter((channel) => channel.channel_status === 1)
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

              {channels.length > 0 && (
                <div className='flex justify-end'>
                  <ChannelStatusFilter
                    showAll={showAllChannels}
                    onShowAllChange={setShowAllChannels}
                  />
                </div>
              )}

              {channels.length === 0 && (
                <div className='text-muted-foreground rounded-lg border border-dashed p-8 text-center text-sm'>
                  {t('No channels support this exact model.')}
                </div>
              )}

              {channels.length > 0 && visibleChannels.length === 0 && (
                <div className='text-muted-foreground rounded-lg border border-dashed p-8 text-center text-sm'>
                  {t('No enabled channels support this exact model.')}
                </div>
              )}

              {visibleChannels.length > 0 && (
                <div className='flex flex-col gap-4'>
                  {visibleChannels.map((channel) => (
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
