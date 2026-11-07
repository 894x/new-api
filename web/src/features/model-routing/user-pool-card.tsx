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
import { zodResolver } from '@hookform/resolvers/zod'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useEffect, useEffectEvent, useState } from 'react'
import { useForm } from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { z } from 'zod'

import { ConfirmDialog } from '@/components/confirm-dialog'
import { ErrorState } from '@/components/error-state'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import {
  Card,
  CardContent,
  CardFooter,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import { Combobox } from '@/components/ui/combobox'
import { Skeleton } from '@/components/ui/skeleton'
import { patchUserChannelRouting } from '@/features/users/api'
import {
  ADMIN_PERMISSION_ACTIONS,
  ADMIN_PERMISSION_RESOURCES,
  canAccessUserGroupConfig,
  hasPermission,
} from '@/lib/admin-permissions'
import { handleServerError } from '@/lib/handle-server-error'
import { cn } from '@/lib/utils'
import { useAuthStore } from '@/stores/auth-store'

import {
  getUserRoutingPool,
  userRoutingPoolKeys,
  type UserRoutingPoolConfig,
} from './api'
import { PoolChannelList } from './pool-channel-list'
import { PoolMonitorDisplay } from './pool-monitor-display'
import { UserPoolStrategy } from './user-pool-strategy'

const schema = z.object({
  channels: z
    .array(
      z.object({
        channel_id: z.number().int().positive(),
        priority: z.number().int().min(0).max(2147483647).nullable(),
        weight: z.number().int().min(0).max(2147483637).nullable(),
        enabled: z.boolean(),
        retry_times_override: z
          .number()
          .int()
          .min(0)
          .max(99)
          .nullable()
          .optional(),
      })
    )
    .max(1000),
})
type Values = z.infer<typeof schema>

export function UserPoolCard(props: {
  userId: number
  model: string
  group: string
  groupLabel: string
  onDirtyChange: (dirty: boolean) => void
}) {
  const { t } = useTranslation()
  const actor = useAuthStore((state) => state.auth.user)
  const [monitorOpen, setMonitorOpen] = useState(false)
  const query = useQuery({
    queryKey: userRoutingPoolKeys.detail(
      props.userId,
      props.group,
      props.model
    ),
    queryFn: () => getUserRoutingPool(props.userId, props.group, props.model),
    refetchOnWindowFocus: false,
    meta: { errorToast: false },
  })
  const canMonitor =
    hasPermission(
      actor,
      ADMIN_PERMISSION_RESOURCES.CHANNEL,
      ADMIN_PERMISSION_ACTIONS.READ
    ) &&
    !!query.data &&
    canAccessUserGroupConfig(actor, query.data.user_role)
  return (
    <PoolMonitorDisplay
      open={monitorOpen && canMonitor}
      onOpenChange={setMonitorOpen}
      model={props.model}
      group={props.group}
      groupLabel={props.groupLabel}
      userId={props.userId}
      username={query.data?.username}
    >
      <Card
        className={cn(
          'min-w-0 gap-4 py-4',
          monitorOpen && canMonitor && 'xl:h-full xl:min-h-0'
        )}
        role='region'
        aria-label={`${props.model} · ${props.groupLabel}`}
      >
        <CardHeader className='flex shrink-0 flex-wrap items-start justify-between gap-2 px-3'>
          <CardTitle className='min-w-0 text-base break-all'>
            {props.model}
          </CardTitle>
          <div className='flex max-w-full flex-wrap items-center justify-end gap-2'>
            <Badge variant='secondary' className='max-w-full'>
              <span className='truncate'>{props.groupLabel}</span>
            </Badge>
            {canMonitor && (
              <Button
                type='button'
                size='sm'
                variant='outline'
                aria-pressed={monitorOpen}
                onClick={() => setMonitorOpen(true)}
              >
                {t('Monitoring view')}
              </Button>
            )}
          </div>
        </CardHeader>
        {query.isPending && (
          <CardContent className='px-3'>
            <Skeleton className='h-36 w-full' />
          </CardContent>
        )}
        {query.isError && (
          <CardContent className='px-3'>
            <ErrorState onRetry={() => void query.refetch()} />
          </CardContent>
        )}
        {query.data && (
          <UserPoolEditor
            key={`${props.userId}:${props.group}:${props.model}`}
            config={query.data}
            group={props.group}
            expanded={monitorOpen && canMonitor}
            onDirtyChange={props.onDirtyChange}
          />
        )}
      </Card>
    </PoolMonitorDisplay>
  )
}

function UserPoolEditor(props: {
  config: UserRoutingPoolConfig
  group: string
  expanded?: boolean
  onDirtyChange: (dirty: boolean) => void
}) {
  const { t } = useTranslation()
  const client = useQueryClient()
  const actor = useAuthStore((s) => s.auth.user)
  const [editingBase, setEditingBase] = useState<UserRoutingPoolConfig | null>(
    null
  )
  const config = editingBase ?? props.config
  const editing = editingBase !== null
  const canWrite = canAccessUserGroupConfig(actor, config.user_role, 'write')
  const [restoreRevision, setRestoreRevision] = useState<string | null>(null)
  const [channelToAdd, setChannelToAdd] = useState<string | null>(null)
  const configuredChannels = new Map(
    config.channels.map((row) => [row.channel_id, row])
  )
  const initial: Values = {
    channels: (config.pool_channels ?? []).map((entry) => {
      const row = configuredChannels.get(entry.channel_id)
      if (
        !config.configured ||
        config.source === 'default_pool' ||
        config.source === 'channel_defaults'
      ) {
        return { ...entry, retry_times_override: row?.retry_times_override }
      }
      return {
        ...entry,
        priority: row?.priority_override ?? null,
        weight: row?.weight_override ?? null,
        enabled: entry.enabled,
        retry_times_override: row?.retry_times_override,
      }
    }),
  }
  const form = useForm<Values>({
    resolver: zodResolver(schema),
    defaultValues: initial,
  })
  const values = form.watch()
  const dirty = editing && (form.formState.isDirty || !config.configured)
  const hasRetryError = form.formState.errors.channels?.some?.(
    (row) => row?.retry_times_override
  )
  const notifyDirty = useEffectEvent(props.onDirtyChange)
  useEffect(() => {
    notifyDirty(dirty)
  }, [dirty])
  useEffect(() => () => notifyDirty(false), [])
  const save = useMutation({
    mutationFn: (data: Values | null) => {
      if (
        !canAccessUserGroupConfig(
          useAuthStore.getState().auth.user,
          config.user_role,
          'write'
        )
      ) {
        throw new Error(t('No permission to perform this action'))
      }
      return patchUserChannelRouting(config.user_id, {
        model: config.model,
        ...(props.group ? { routing_group: props.group } : {}),
        revision:
          data === null
            ? (restoreRevision ?? config.revision)
            : config.revision,
        reset: data === null,
        overrides:
          data?.channels.map((row) => ({
            channel_id: row.channel_id,
            priority_override: row.priority,
            weight_override: row.weight,
            enabled: row.enabled,
            ...(row.retry_times_override !== undefined
              ? { retry_times_override: row.retry_times_override }
              : {}),
          })) ?? [],
      })
    },
    onSuccess: async () => {
      setEditingBase(null)
      setRestoreRevision(null)
      setChannelToAdd(null)
      props.onDirtyChange(false)
      toast.success(t('Saved successfully'))
      await Promise.all([
        client.invalidateQueries({
          queryKey: userRoutingPoolKeys.all(config.user_id),
        }),
        client.invalidateQueries({
          queryKey: ['user-channel-routing-models', config.user_id],
        }),
        client.invalidateQueries({
          queryKey: ['user-channel-routing', config.user_id],
        }),
        client.invalidateQueries({ queryKey: ['routing-monitor-pool'] }),
        client.invalidateQueries({ queryKey: ['routing-monitor'] }),
      ])
    },
    onError: (error) => handleServerError(error, t('Failed to save')),
  })
  const disabled = !canWrite || !editing || save.isPending
  const sources = {
    user: t('User override'),
    default_pool: t('Default routing pool'),
    channel_defaults: t('Channel defaults'),
  }
  const candidates = config.channels.map((row) => ({
    channel_id: row.channel_id,
    channel_name: row.channel_name,
    priority: row.inherited_priority,
    weight: row.inherited_weight ?? 0,
    available: row.available,
  }))
  const displayed = editing
    ? values.channels
    : (config.pool_channels ?? []).map((entry) => ({
        ...entry,
        retry_times_override: configuredChannels.get(entry.channel_id)
          ?.retry_times_override,
      }))
  const unused = candidates.filter(
    (candidate) =>
      candidate.available &&
      !values.channels.some((row) => row.channel_id === candidate.channel_id)
  )
  return (
    <>
      <CardContent
        className={cn(
          'space-y-3 px-3',
          props.expanded && 'xl:min-h-0 xl:flex-1 xl:overflow-y-auto'
        )}
      >
        <div className='flex flex-wrap items-center justify-between gap-2 text-xs'>
          <Badge variant={config.configured ? 'default' : 'outline'}>
            {config.configured ? t('User override') : sources[config.source]}
          </Badge>
          <UserPoolStrategy
            config={config}
            disabled={editing || save.isPending}
          />
        </div>
        {editing && (
          <p className='text-muted-foreground text-xs'>
            {t(
              'Only channels in this user pool can be selected. Empty priority or weight inherits the channel value. Clearing the pool restores the default routing pool.'
            )}
          </p>
        )}
        {editing && (
          <p className='text-muted-foreground text-xs'>
            {t(
              'Leave retries empty to inherit the global limit; 0 disables retries. Failed requests switch channels as usual.'
            )}
          </p>
        )}
        {!editing && !config.configured && (
          <p role='status' className='text-muted-foreground text-xs'>
            {t(
              'No user-specific channels are configured. The default routing pool is used.'
            )}
          </p>
        )}
        {displayed.length > 0 &&
          (editing || config.configured) &&
          displayed.every((row) => !row.enabled) && (
            <Alert variant='destructive'>
              <AlertDescription>
                {t(
                  'This user pool has no enabled channels. Requests for this model will fail.'
                )}
              </AlertDescription>
            </Alert>
          )}
        {displayed.length === 0 ? (
          <p role='status' className='text-muted-foreground text-sm'>
            {editing
              ? t(
                  'This user pool is empty. Saving will restore the default routing pool.'
                )
              : t('No available channels')}
          </p>
        ) : (
          <PoolChannelList
            model={config.model}
            channels={displayed}
            candidates={candidates}
            disabled={disabled}
            removable={editing}
            allowInheritance
            onChange={(channels) =>
              form.setValue('channels', channels, {
                shouldDirty: true,
                shouldValidate: true,
              })
            }
          />
        )}
        {editing && (
          <div className='flex min-w-0 flex-wrap gap-2'>
            <Combobox
              options={unused.map((candidate) => ({
                value: String(candidate.channel_id),
                label: `${candidate.channel_name} #${candidate.channel_id}`,
              }))}
              value={channelToAdd}
              onValueChange={setChannelToAdd}
              disabled={disabled}
              aria-label={t('Select channel')}
              placeholder={t('Select channel')}
              className='min-w-0 flex-1'
            />
            <Button
              type='button'
              size='sm'
              variant='outline'
              disabled={disabled || !channelToAdd}
              onClick={() => {
                const candidate = unused.find(
                  (row) => row.channel_id === Number(channelToAdd)
                )
                if (!candidate || disabled) return
                form.setValue(
                  'channels',
                  [
                    ...values.channels,
                    {
                      channel_id: candidate.channel_id,
                      priority: candidate.priority,
                      weight: candidate.weight,
                      enabled: true,
                    },
                  ],
                  { shouldDirty: true, shouldValidate: true }
                )
                setChannelToAdd(null)
              }}
            >
              {t('Add')}
            </Button>
          </div>
        )}
        {form.formState.errors.channels && !hasRetryError && (
          <p role='alert' className='text-destructive text-sm'>
            {t(
              'Priority and weight must be whole numbers within the allowed range.'
            )}
          </p>
        )}
        {hasRetryError && (
          <p role='alert' className='text-destructive text-sm'>
            {t('Retry count must be an integer from 0 to 99.')}
          </p>
        )}
        {save.isError && (
          <p role='alert' className='text-destructive text-xs'>
            {t(
              'Changes were not saved. Reload if another administrator changed this configuration.'
            )}
          </p>
        )}
      </CardContent>
      <CardFooter className='flex shrink-0 flex-wrap justify-between gap-2 px-3'>
        {!editing ? (
          <Button
            type='button'
            size='sm'
            variant='outline'
            disabled={
              !canWrite ||
              save.isPending ||
              (!candidates.some((row) => row.available) &&
                !initial.channels.length)
            }
            onClick={() => {
              form.reset(initial)
              setChannelToAdd(null)
              setEditingBase(config)
            }}
          >
            {config.configured
              ? t('Edit user override')
              : t('Create user override')}
          </Button>
        ) : (
          <div className='flex items-center gap-2'>
            <Button
              type='button'
              size='sm'
              variant='ghost'
              disabled={save.isPending}
              onClick={() => {
                form.reset(initial)
                setEditingBase(null)
                setChannelToAdd(null)
              }}
            >
              {t('Cancel')}
            </Button>
            <Button
              type='button'
              size='sm'
              disabled={!canWrite || save.isPending || !dirty}
              onClick={() =>
                void form.handleSubmit((data) => save.mutate(data))()
              }
            >
              {save.isPending ? t('Saving...') : t('Save')}
            </Button>
          </div>
        )}
        {config.configured && (
          <Button
            type='button'
            size='sm'
            variant='link'
            className='text-muted-foreground text-xs'
            disabled={!canWrite || save.isPending}
            onClick={() => setRestoreRevision(config.revision)}
          >
            {t('Clear and use default pool')}
          </Button>
        )}
      </CardFooter>
      <ConfirmDialog
        open={restoreRevision !== null}
        onOpenChange={(open) => {
          if (!open) setRestoreRevision(null)
        }}
        title={t('Clear and use default pool')}
        desc={t(
          'Clear this user pool and use the default routing pool for this model and public group?'
        )}
        confirmText={t('Clear and use default pool')}
        isLoading={save.isPending}
        handleConfirm={() => save.mutate(null)}
      />
    </>
  )
}
