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
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import {
  Card,
  CardContent,
  CardFooter,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import { Skeleton } from '@/components/ui/skeleton'
import { patchUserChannelRouting } from '@/features/users/api'
import { canAccessUserGroupConfig } from '@/lib/admin-permissions'
import { handleServerError } from '@/lib/handle-server-error'
import { useAuthStore } from '@/stores/auth-store'

import {
  getUserRoutingPool,
  userRoutingPoolKeys,
  type UserRoutingPoolConfig,
} from './api'
import { PoolChannelList } from './pool-channel-list'

const schema = z.object({
  channels: z
    .array(
      z.object({
        channel_id: z.number().int().positive(),
        priority: z.number().int().min(0).max(2147483647).nullable(),
        weight: z.number().int().min(0).max(2147483637).nullable(),
        enabled: z.boolean(),
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
  return (
    <Card
      className='min-w-0 gap-4 py-4'
      role='region'
      aria-label={`${props.model} · ${props.groupLabel}`}
    >
      <CardHeader className='flex flex-wrap items-start justify-between gap-2 px-3'>
        <CardTitle className='min-w-0 text-base break-all'>
          {props.model}
        </CardTitle>
        <Badge variant='secondary' className='max-w-full'>
          <span className='truncate'>{props.groupLabel}</span>
        </Badge>
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
          onDirtyChange={props.onDirtyChange}
        />
      )}
    </Card>
  )
}

function UserPoolEditor(props: {
  config: UserRoutingPoolConfig
  group: string
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
  const initial: Values = {
    channels: (config.pool_channels ?? []).map((entry) => {
      const row = config.channels.find(
        (candidate) => candidate.channel_id === entry.channel_id
      )
      if (
        !config.configured ||
        config.source === 'default_pool' ||
        config.source === 'channel_defaults'
      ) {
        return entry
      }
      return {
        ...entry,
        priority: row?.priority_override ?? null,
        weight: row?.weight_override ?? null,
        enabled: row?.enabled ?? entry.enabled,
      }
    }),
  }
  const form = useForm<Values>({
    resolver: zodResolver(schema),
    defaultValues: initial,
  })
  const values = form.watch()
  const dirty = editing && (form.formState.isDirty || !config.configured)
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
          data?.channels
            .filter((row) => {
              const old = config.channels.find(
                (item) => item.channel_id === row.channel_id
              )
              return (
                !config.configured ||
                row.priority !== (old?.priority_override ?? null) ||
                row.weight !== (old?.weight_override ?? null) ||
                row.enabled !== old?.enabled
              )
            })
            .map((row) => ({
              channel_id: row.channel_id,
              priority_override: row.priority,
              weight_override: row.weight,
              inherit_weight: row.weight === null,
              enabled: row.enabled,
            })) ?? [],
      })
    },
    onSuccess: async () => {
      setEditingBase(null)
      setRestoreRevision(null)
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
      ])
    },
    onError: (error) => handleServerError(error, t('Failed to save')),
  })
  const disabled = !canWrite || !editing || save.isPending
  const sources = {
    user: t('User override'),
    legacy: t('Legacy user override'),
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
  return (
    <>
      <CardContent className='space-y-3 px-3'>
        <div className='flex flex-wrap items-center justify-between gap-2 text-xs'>
          <Badge variant={config.configured ? 'default' : 'outline'}>
            {config.configured ? t('User override') : sources[config.source]}
          </Badge>
          <span className='text-muted-foreground'>
            {props.group
              ? t('Strategy: {{strategy}} (inherited)', {
                  strategy:
                    config.mode === 'smart'
                      ? t('Smart routing')
                      : t('Priority routing'),
                })
              : t('Strategy follows each public group')}
          </span>
        </div>
        {editing && (
          <p className='text-muted-foreground text-xs'>
            {t(
              'Leave priority or weight empty to inherit its value. Disable a channel to exclude it for this user.'
            )}
          </p>
        )}
        {(config.pool_channels ?? []).length === 0 ? (
          <p role='status' className='text-muted-foreground text-sm'>
            {t('No available channels')}
          </p>
        ) : (
          <PoolChannelList
            channels={editing ? values.channels : (config.pool_channels ?? [])}
            candidates={candidates}
            disabled={disabled}
            removable={false}
            allowInheritance
            onChange={(channels) =>
              form.setValue('channels', channels, {
                shouldDirty: true,
                shouldValidate: true,
              })
            }
          />
        )}
        {form.formState.errors.channels && (
          <p role='alert' className='text-destructive text-sm'>
            {t(
              'Priority and weight must be whole numbers within the allowed range.'
            )}
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
      <CardFooter className='flex flex-wrap justify-between gap-2 px-3'>
        {!editing ? (
          <Button
            type='button'
            size='sm'
            variant='outline'
            disabled={!canWrite || save.isPending || !initial.channels.length}
            onClick={() => {
              form.reset(initial)
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
            {t('Restore inheritance')}
          </Button>
        )}
      </CardFooter>
      <ConfirmDialog
        open={restoreRevision !== null}
        onOpenChange={(open) => {
          if (!open) setRestoreRevision(null)
        }}
        title={t('Restore inheritance')}
        desc={t(
          'Remove this user override and follow the inherited routing configuration again?'
        )}
        confirmText={t('Restore')}
        isLoading={save.isPending}
        handleConfirm={() => save.mutate(null)}
      />
    </>
  )
}
