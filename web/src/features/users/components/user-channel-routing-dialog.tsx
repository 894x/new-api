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
import { ArrowDown, ArrowUp, GripVertical, Plus } from 'lucide-react'
import { useEffect, useEffectEvent, useRef, useState } from 'react'
import { useFieldArray, useForm } from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { z } from 'zod'

import { EmptyState } from '@/components/empty-state'
import { ErrorState } from '@/components/error-state'
import { LoadingState } from '@/components/loading-state'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Combobox } from '@/components/ui/combobox'
import {
  Form,
  FormControl,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from '@/components/ui/form'
import { Input } from '@/components/ui/input'
import { Switch } from '@/components/ui/switch'
import { toIntlLocale } from '@/i18n/languages'
import {
  ADMIN_PERMISSION_ACTIONS,
  ADMIN_PERMISSION_RESOURCES,
  hasPermission,
} from '@/lib/admin-permissions'
import { formatNumber } from '@/lib/format'
import { handleServerError } from '@/lib/handle-server-error'
import { ROLE } from '@/lib/roles'
import { cn } from '@/lib/utils'
import { useAuthStore } from '@/stores/auth-store'

import { getUserChannelRouting, patchUserChannelRouting } from '../api'
import type { UserChannelRoutingConfig } from '../types'

const schema = z.object({
  channels: z.array(
    z.object({
      channel_id: z.number().int(),
      priority_override: z.number().int().min(0).max(2147483647).nullable(),
      enabled: z.boolean(),
    })
  ),
})
type FormValues = z.infer<typeof schema>

export function UserChannelRoutingRuleEditor(props: {
  userId: number
  model: string
  channelGroups: string[]
  readOnly?: boolean
  onDirtyChange?: (dirty: boolean) => void
}) {
  const { t } = useTranslation()
  const dirtyGroups = useRef(new Set<string>())
  return (
    <div className='flex flex-col gap-3'>
      <p className='text-muted-foreground text-xs'>
        {t('Channel priorities are independent for each outlet group.')}
      </p>
      {props.channelGroups.map((group) => (
        <section
          key={group}
          aria-label={group}
          className='min-w-0 rounded-lg border p-3'
        >
          <h4 className='mb-3 text-sm font-medium break-all'>{group}</h4>
          <UserChannelRoutingGroupEditor
            userId={props.userId}
            model={props.model}
            group={group}
            readOnly={props.readOnly}
            onDirtyChange={(dirty) => {
              if (dirty) dirtyGroups.current.add(group)
              else dirtyGroups.current.delete(group)
              props.onDirtyChange?.(dirtyGroups.current.size > 0)
            }}
          />
        </section>
      ))}
    </div>
  )
}

function UserChannelRoutingGroupEditor(props: {
  userId: number
  model: string
  group: string
  readOnly?: boolean
  onDirtyChange: (dirty: boolean) => void
}) {
  const currentUser = useAuthStore((state) => state.auth.user)
  const canRead =
    (currentUser?.role ?? 0) >= ROLE.ADMIN &&
    hasPermission(
      currentUser,
      ADMIN_PERMISSION_RESOURCES.USER_GROUP_CONFIG,
      ADMIN_PERMISSION_ACTIONS.READ
    )
  const query = useQuery({
    queryKey: ['user-channel-routing', props.userId, props.model, props.group],
    queryFn: () =>
      getUserChannelRouting(
        props.userId,
        props.model,
        [props.group],
        props.group
      ),
    enabled: props.model !== '' && canRead,
    refetchOnWindowFocus: false,
    refetchOnReconnect: false,
  })

  if (!canRead) return null
  if (query.isPending) return <LoadingState />
  if (query.isError) {
    return <ErrorState onRetry={() => void query.refetch()} />
  }
  if (!query.data?.data) return null

  return (
    <UserChannelRoutingEditor
      key={`${props.userId}:${props.model}:${props.group}:${query.dataUpdatedAt}`}
      userId={props.userId}
      config={query.data.data}
      readOnly={props.readOnly}
      onDirtyChange={props.onDirtyChange}
      onReload={() => {
        props.onDirtyChange?.(false)
        void query.refetch()
      }}
    />
  )
}

export function UserChannelRoutingEditor(props: {
  userId: number
  config: UserChannelRoutingConfig
  onDirtyChange: (dirty: boolean) => void
  onReload: () => void
  readOnly?: boolean
}) {
  const { t, i18n } = useTranslation()
  const locale = toIntlLocale(i18n.resolvedLanguage || i18n.language)
  const queryClient = useQueryClient()
  const currentUser = useAuthStore((state) => state.auth.user)
  const canWrite =
    !props.readOnly &&
    (currentUser?.role ?? 0) >= ROLE.ADMIN &&
    hasPermission(
      currentUser,
      ADMIN_PERMISSION_RESOURCES.USER_GROUP_CONFIG,
      ADMIN_PERMISSION_ACTIONS.READ
    ) &&
    hasPermission(
      currentUser,
      ADMIN_PERMISSION_RESOURCES.USER_GROUP_CONFIG,
      ADMIN_PERMISSION_ACTIONS.WRITE
    )
  const [dragged, setDragged] = useState<number | null>(null)
  const [dropTarget, setDropTarget] = useState<number | null>(null)
  const [channelToAdd, setChannelToAdd] = useState<string | null>(null)
  const form = useForm<FormValues>({
    resolver: zodResolver(schema),
    defaultValues: {
      channels: props.config.channels
        .filter(
          (row) =>
            !props.config.routing_group ||
            row.configured ||
            row.priority_override !== null ||
            !row.enabled
        )
        .map((row) => ({
          channel_id: row.channel_id,
          priority_override: row.priority_override,
          enabled: row.enabled,
        })),
    },
  })
  const rows = useFieldArray({ control: form.control, name: 'channels' })
  const values = form.watch('channels')
  const originalById = new Map(
    props.config.channels.map((row) => [row.channel_id, row])
  )
  const save = useMutation({
    mutationFn: (data: FormValues) => {
      const actor = useAuthStore.getState().auth.user
      if (
        props.readOnly ||
        (actor?.role ?? 0) < ROLE.ADMIN ||
        !hasPermission(
          actor,
          ADMIN_PERMISSION_RESOURCES.USER_GROUP_CONFIG,
          ADMIN_PERMISSION_ACTIONS.READ
        ) ||
        !hasPermission(
          actor,
          ADMIN_PERMISSION_RESOURCES.USER_GROUP_CONFIG,
          ADMIN_PERMISSION_ACTIONS.WRITE
        )
      ) {
        throw new Error(t('No permission to perform this action'))
      }
      return patchUserChannelRouting(props.userId, {
        model: props.config.model,
        ...(props.config.routing_group
          ? { routing_group: props.config.routing_group }
          : {}),
        revision: props.config.revision,
        overrides: data.channels.filter((row) => {
          const original = originalById.get(row.channel_id)
          return (
            row.priority_override !== original?.priority_override ||
            row.enabled !== original?.enabled ||
            (props.config.routing_group && !original?.configured)
          )
        }),
      })
    },
    onSuccess: async () => {
      toast.success(t('Saved successfully'))
      props.onDirtyChange(false)
      await queryClient.invalidateQueries({
        queryKey: [
          'user-channel-routing',
          props.userId,
          props.config.model,
          props.config.routing_group ?? '',
        ],
      })
    },
    onError: (error) => handleServerError(error, t('Failed to save')),
  })
  const dirty = values.some((row) => {
    const original = originalById.get(row.channel_id)
    return (
      row.priority_override !== original?.priority_override ||
      row.enabled !== original?.enabled ||
      (!!props.config.routing_group && !original?.configured)
    )
  })
  const notifyDirty = useEffectEvent(props.onDirtyChange)
  useEffect(() => {
    notifyDirty(dirty)
  }, [dirty])
  useEffect(() => () => notifyDirty(false), [])
  const editingDisabled = !canWrite || save.isPending

  function reorder(from: number, to: number) {
    if (
      editingDisabled ||
      from === to ||
      from < 0 ||
      to < 0 ||
      to >= rows.fields.length
    ) {
      return
    }
    rows.move(from, to)
    for (let index = 0; index < rows.fields.length; index++) {
      form.setValue(
        `channels.${index}.priority_override`,
        rows.fields.length - index,
        { shouldDirty: true }
      )
    }
  }

  if (props.config.channels.length === 0) {
    return <EmptyState title={t('No available channels')} />
  }

  return (
    <Form {...form}>
      <div className='flex flex-col gap-4'>
        {props.config.routing_group && (
          <div className='flex flex-wrap gap-2'>
            <Combobox
              className='min-w-0 flex-1'
              options={props.config.channels
                .filter(
                  (candidate) =>
                    candidate.available &&
                    !values.some(
                      (row) => row.channel_id === candidate.channel_id
                    )
                )
                .map((candidate) => ({
                  value: String(candidate.channel_id),
                  label: `${candidate.channel_name} #${candidate.channel_id}`,
                }))}
              value={channelToAdd}
              onValueChange={setChannelToAdd}
              disabled={editingDisabled}
              aria-label={t('Select channel')}
              placeholder={t('Select channel')}
            />
            <Button
              type='button'
              variant='outline'
              disabled={editingDisabled || !channelToAdd}
              onClick={() => {
                if (editingDisabled) return
                const candidate = originalById.get(Number(channelToAdd))
                if (
                  !candidate ||
                  !candidate.available ||
                  values.some((row) => row.channel_id === candidate.channel_id)
                ) {
                  return
                }
                rows.append({
                  channel_id: candidate.channel_id,
                  priority_override: candidate.inherited_priority,
                  enabled: true,
                })
                setChannelToAdd(null)
              }}
            >
              <Plus />
              {t('Add channel')}
            </Button>
          </div>
        )}
        <ol
          className='flex flex-col gap-2'
          aria-label={t('Channel priority order')}
        >
          {rows.fields.map((item, index) => {
            const original = originalById.get(item.channel_id)
            if (!original) return null
            const row = values[index]
            const channelLabel = `${original.channel_name || t('Channel')} #${item.channel_id}`
            return (
              <li
                key={item.id}
                aria-label={channelLabel}
                onDragOver={(event) => {
                  if (dragged !== null && !editingDisabled) {
                    event.preventDefault()
                    setDropTarget(index)
                  }
                }}
                onDrop={(event) => {
                  event.preventDefault()
                  if (dragged !== null) reorder(dragged, index)
                  setDragged(null)
                  setDropTarget(null)
                }}
                className={cn(
                  'rounded-lg border p-3',
                  !row.enabled && 'bg-muted/40',
                  dragged === index && 'opacity-50',
                  dropTarget === index && 'border-primary'
                )}
              >
                <div className='flex flex-wrap items-center gap-2'>
                  <Button
                    type='button'
                    variant='ghost'
                    size='icon-sm'
                    draggable={!editingDisabled && rows.fields.length > 1}
                    disabled={editingDisabled || rows.fields.length < 2}
                    className='cursor-grab'
                    aria-label={t('Reorder {{channel}}', {
                      channel: channelLabel,
                    })}
                    onDragStart={(event) => {
                      if (editingDisabled) {
                        event.preventDefault()
                        return
                      }
                      event.dataTransfer.effectAllowed = 'move'
                      event.dataTransfer.setData(
                        'text/plain',
                        String(item.channel_id)
                      )
                      setDragged(index)
                    }}
                    onDragEnd={() => {
                      setDragged(null)
                      setDropTarget(null)
                    }}
                    onKeyDown={(event) => {
                      if (
                        event.key === 'ArrowUp' ||
                        event.key === 'ArrowDown'
                      ) {
                        event.preventDefault()
                        reorder(
                          index,
                          index + (event.key === 'ArrowUp' ? -1 : 1)
                        )
                      }
                    }}
                  >
                    <GripVertical />
                  </Button>
                  <span className='min-w-0 flex-1 font-medium break-all'>
                    {channelLabel}
                  </span>
                  {!original.available && (
                    <Badge variant='secondary'>{t('Unavailable')}</Badge>
                  )}
                  <FormField
                    control={form.control}
                    name={`channels.${index}.enabled`}
                    render={({ field }) => (
                      <FormItem className='flex items-center gap-2'>
                        <FormLabel className='sr-only'>
                          {t('Enable {{channel}} for this user', {
                            channel: channelLabel,
                          })}
                        </FormLabel>
                        <span aria-hidden='true' className='text-sm'>
                          {t('Enabled')}
                        </span>
                        <FormControl>
                          <Switch
                            checked={field.value}
                            disabled={editingDisabled}
                            onCheckedChange={(value) => {
                              field.onChange(value)
                            }}
                            aria-label={t('Enable {{channel}} for this user', {
                              channel: channelLabel,
                            })}
                          />
                        </FormControl>
                      </FormItem>
                    )}
                  />
                  <Button
                    type='button'
                    variant='ghost'
                    size='icon-sm'
                    disabled={editingDisabled || index === 0}
                    aria-label={t('Move {{channel}} up', {
                      channel: channelLabel,
                    })}
                    onClick={() => reorder(index, index - 1)}
                  >
                    <ArrowUp />
                  </Button>
                  <Button
                    type='button'
                    variant='ghost'
                    size='icon-sm'
                    disabled={
                      editingDisabled || index === rows.fields.length - 1
                    }
                    aria-label={t('Move {{channel}} down', {
                      channel: channelLabel,
                    })}
                    onClick={() => reorder(index, index + 1)}
                  >
                    <ArrowDown />
                  </Button>
                </div>
                <div className='mt-3 grid items-end gap-3 sm:grid-cols-[1fr_9rem_auto]'>
                  <div className='text-muted-foreground flex flex-col gap-1 text-xs'>
                    <p>
                      {t('Channel default')}:{' '}
                      {formatNumber(original.default_priority, locale)} ·{' '}
                      {t('Model override')}:{' '}
                      {original.model_priority === null
                        ? t('Inherited')
                        : formatNumber(original.model_priority, locale)}
                    </p>
                    <p>
                      {t('Effective priority')}:{' '}
                      {formatNumber(
                        row.priority_override ?? original.inherited_priority,
                        locale
                      )}
                    </p>
                  </div>
                  <FormField
                    control={form.control}
                    name={`channels.${index}.priority_override`}
                    render={({ field }) => (
                      <FormItem>
                        <FormLabel>{t('User priority')}</FormLabel>
                        <FormControl>
                          <Input
                            type='number'
                            min={0}
                            max={2147483647}
                            step={1}
                            disabled={editingDisabled}
                            aria-label={t('Priority for {{channel}}', {
                              channel: channelLabel,
                            })}
                            placeholder={String(original.inherited_priority)}
                            value={field.value ?? ''}
                            onChange={(event) => {
                              field.onChange(
                                event.target.value === ''
                                  ? null
                                  : Number(event.target.value)
                              )
                            }}
                          />
                        </FormControl>
                        <FormMessage />
                      </FormItem>
                    )}
                  />
                  <Button
                    type='button'
                    variant='outline'
                    disabled={editingDisabled || row.priority_override === null}
                    onClick={() => {
                      form.setValue(
                        `channels.${index}.priority_override`,
                        null,
                        { shouldDirty: true }
                      )
                    }}
                  >
                    {t('Inherit')}
                  </Button>
                </div>
              </li>
            )
          })}
        </ol>
        {save.isError && (
          <Alert variant='destructive'>
            <AlertDescription>
              {t(
                'Changes were not saved. Reload if another administrator changed this configuration.'
              )}
            </AlertDescription>
          </Alert>
        )}
        <div className='flex flex-wrap justify-end gap-2'>
          <Button
            type='button'
            variant='outline'
            disabled={save.isPending}
            onClick={() => {
              form.reset()
              props.onDirtyChange(false)
              props.onReload()
            }}
          >
            {t('Reload')}
          </Button>
          <Button
            type='button'
            disabled={editingDisabled || !dirty}
            onClick={() =>
              void form.handleSubmit((data) => save.mutate(data))()
            }
          >
            {save.isPending ? t('Saving...') : t('Save changes')}
          </Button>
        </div>
      </div>
    </Form>
  )
}
