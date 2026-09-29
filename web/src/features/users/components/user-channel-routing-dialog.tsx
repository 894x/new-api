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
import { ArrowDown, ArrowUp, GripVertical } from 'lucide-react'
import { useEffect, useState } from 'react'
import { useFieldArray, useForm } from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { z } from 'zod'

import { Dialog } from '@/components/dialog'
import { EmptyState } from '@/components/empty-state'
import { ErrorState } from '@/components/error-state'
import { LoadingState } from '@/components/loading-state'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Combobox } from '@/components/ui/combobox'
import { Field, FieldLabel } from '@/components/ui/field'
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
import { formatNumber } from '@/lib/format'
import { handleServerError } from '@/lib/handle-server-error'
import { cn } from '@/lib/utils'

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

export function UserChannelRoutingDialog(props: {
  user: { id: number; username: string }
  open: boolean
  onOpenChange: (open: boolean) => void
}) {
  const { t } = useTranslation()
  const [model, setModel] = useState('')
  const [dirty, setDirty] = useState(false)
  const models = useQuery({
    queryKey: ['user-channel-routing', props.user.id, 'models'],
    queryFn: () => getUserChannelRouting(props.user.id),
    enabled: props.open,
  })
  const query = useQuery({
    queryKey: ['user-channel-routing', props.user.id, model],
    queryFn: () => getUserChannelRouting(props.user.id, model),
    enabled: props.open && model !== '',
    refetchOnWindowFocus: false,
    refetchOnReconnect: false,
  })

  return (
    <Dialog
      open={props.open}
      onOpenChange={props.onOpenChange}
      title={t('User channel routing')}
      description={t(
        'Channel routing for {{username}}. Changes apply only to this user and the selected model.',
        { username: props.user.username }
      )}
      contentClassName='sm:max-w-3xl'
      bodyClassName='space-y-4'
    >
      <Field>
        <FieldLabel htmlFor='user-routing-model'>{t('Model')}</FieldLabel>
        <Combobox
          id='user-routing-model'
          value={model}
          onValueChange={(value) => setModel(value ?? '')}
          disabled={dirty}
          options={(models.data?.data?.models ?? []).map((value) => ({
            value,
            label: value,
          }))}
          placeholder={t('Select model')}
          allowCustomValue
        />
      </Field>
      <p className='text-muted-foreground text-sm'>
        {t(
          'Drag channels to set priority. Higher values run first; equal values use channel weights. Switch off to pause a channel without losing its priority.'
        )}
      </p>
      <p className='text-muted-foreground text-xs'>
        {t(
          'User overrides take precedence over dynamic routing and session affinity. Group access, request compatibility, capacity limits, and fixed task channels still apply.'
        )}
      </p>
      {models.isError && (
        <ErrorState
          onRetry={() => {
            void models.refetch()
          }}
        />
      )}
      {model && query.isPending && <LoadingState />}
      {model && query.isError && (
        <ErrorState
          onRetry={() => {
            void query.refetch()
          }}
        />
      )}
      {model && query.data?.data && !query.isError && (
        <UserChannelRoutingEditor
          key={`${props.user.id}:${model}:${query.dataUpdatedAt}`}
          userId={props.user.id}
          config={query.data.data}
          onDirtyChange={setDirty}
          onReload={() => {
            setDirty(false)
            void query.refetch()
          }}
        />
      )}
    </Dialog>
  )
}

export function UserChannelRoutingEditor(props: {
  userId: number
  config: UserChannelRoutingConfig
  onDirtyChange: (dirty: boolean) => void
  onReload: () => void
}) {
  const { t, i18n } = useTranslation()
  const locale = toIntlLocale(i18n.resolvedLanguage || i18n.language)
  const queryClient = useQueryClient()
  const [dragged, setDragged] = useState<number | null>(null)
  const [dropTarget, setDropTarget] = useState<number | null>(null)
  const form = useForm<FormValues>({
    resolver: zodResolver(schema),
    defaultValues: {
      channels: props.config.channels.map((row) => ({
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
    mutationFn: (data: FormValues) =>
      patchUserChannelRouting(props.userId, {
        model: props.config.model,
        revision: props.config.revision,
        overrides: data.channels.filter((row) => {
          const original = originalById.get(row.channel_id)
          return (
            row.priority_override !== original?.priority_override ||
            row.enabled !== original.enabled
          )
        }),
      }),
    onSuccess: async () => {
      toast.success(t('Saved successfully'))
      props.onDirtyChange(false)
      await queryClient.invalidateQueries({
        queryKey: ['user-channel-routing', props.userId],
      })
    },
    onError: (error) => handleServerError(error, t('Failed to save')),
  })
  const dirty = values.some((row) => {
    const original = originalById.get(row.channel_id)
    return (
      row.priority_override !== original?.priority_override ||
      row.enabled !== original.enabled
    )
  })
  const { onDirtyChange } = props
  useEffect(() => {
    onDirtyChange(dirty)
  }, [dirty, onDirtyChange])

  function reorder(from: number, to: number) {
    if (
      save.isPending ||
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

  if (rows.fields.length === 0) {
    return <EmptyState title={t('No available channels')} />
  }

  return (
    <Form {...form}>
      <form
        className='space-y-4'
        onSubmit={form.handleSubmit((data) => save.mutate(data))}
      >
        <ol className='space-y-2' aria-label={t('Channel priority order')}>
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
                  if (dragged !== null && !save.isPending) {
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
                    draggable={!save.isPending && rows.fields.length > 1}
                    disabled={save.isPending || rows.fields.length < 2}
                    className='cursor-grab'
                    aria-label={t('Reorder {{channel}}', {
                      channel: channelLabel,
                    })}
                    onDragStart={(event) => {
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
                            disabled={save.isPending}
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
                    disabled={save.isPending || index === 0}
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
                      save.isPending || index === rows.fields.length - 1
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
                  <div className='text-muted-foreground space-y-1 text-xs'>
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
                            disabled={save.isPending}
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
                    disabled={save.isPending || row.priority_override === null}
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
          <Button type='submit' disabled={save.isPending || !dirty}>
            {save.isPending ? t('Saving...') : t('Save changes')}
          </Button>
        </div>
      </form>
    </Form>
  )
}
