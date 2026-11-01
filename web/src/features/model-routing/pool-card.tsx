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
import { Dialog } from '@/components/dialog'
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
import { handleServerError } from '@/lib/handle-server-error'

import {
  deleteRoutingPool,
  getRoutingPool,
  routingPoolKeys,
  saveRoutingPool,
  type RoutingPoolView,
} from './api'
import { PoolChannelList } from './pool-channel-list'
import { PoolModeSelect } from './pool-mode-select'

const poolFormSchema = z.object({
  mode: z.enum(['priority', 'smart']),
  channels: z
    .array(
      z.object({
        channel_id: z.number().int().positive(),
        priority: z.number().int().min(0).max(2147483647),
        weight: z.number().int().min(0).max(2147483637),
        enabled: z.boolean(),
      })
    )
    .max(500),
})
type PoolFormValues = z.infer<typeof poolFormSchema>

export function PoolCard(props: {
  pool: RoutingPoolView
  canWrite: boolean
  groupLabel?: string
  onDirtyChange: (dirty: boolean) => void
}) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const [adding, setAdding] = useState(false)
  const [channelToAdd, setChannelToAdd] = useState<string | null>(null)
  const [editingStrategy, setEditingStrategy] = useState(false)
  const [restoring, setRestoring] = useState(false)
  const form = useForm<PoolFormValues>({
    resolver: zodResolver(poolFormSchema),
    defaultValues: { mode: props.pool.mode, channels: props.pool.channels },
  })
  const values = form.watch()
  const dirty = form.formState.isDirty
  const notifyDirty = useEffectEvent(props.onDirtyChange)
  useEffect(() => {
    notifyDirty(dirty)
  }, [dirty])
  useEffect(() => () => notifyDirty(false), [])
  useEffect(() => {
    if (!dirty) {
      form.reset({ mode: props.pool.mode, channels: props.pool.channels })
    }
  }, [props.pool.revision, dirty, form, props.pool.mode, props.pool.channels])
  const candidates = useQuery({
    queryKey: routingPoolKeys.detail(
      props.pool.routing_group,
      props.pool.model
    ),
    queryFn: () => getRoutingPool(props.pool.routing_group, props.pool.model),
    enabled: adding && props.canWrite,
    refetchOnWindowFocus: false,
    meta: { errorToast: false },
  })
  const save = useMutation({
    mutationFn: (data: PoolFormValues) => {
      if (!props.canWrite) {
        throw new Error(t('No permission to perform this action'))
      }
      return saveRoutingPool({ ...props.pool, ...data })
    },
    onSuccess: async (pool) => {
      form.reset({ mode: pool.mode, channels: pool.channels })
      toast.success(t('Saved successfully'))
      await queryClient.invalidateQueries({ queryKey: routingPoolKeys.all })
    },
    onError: (error) => handleServerError(error, t('Failed to save')),
  })
  const restore = useMutation({
    mutationFn: () => {
      if (!props.canWrite) {
        throw new Error(t('No permission to perform this action'))
      }
      return deleteRoutingPool(props.pool)
    },
    onSuccess: async () => {
      props.onDirtyChange(false)
      setRestoring(false)
      await queryClient.invalidateQueries({ queryKey: routingPoolKeys.all })
    },
    onError: (error) => handleServerError(error, t('Failed to save')),
  })
  const disabled = !props.canWrite || save.isPending || restore.isPending
  const available = candidates.data?.candidates ?? props.pool.candidates
  const unused = available.filter(
    (candidate) =>
      candidate.available &&
      !values.channels.some(
        (entry) => entry.channel_id === candidate.channel_id
      )
  )

  return (
    <Card className='min-w-0 gap-4 py-4'>
      <CardHeader className='gap-3 px-4'>
        <div className='flex min-w-0 flex-wrap items-start justify-between gap-2'>
          <CardTitle className='min-w-0 text-base break-all'>
            {props.pool.model}
          </CardTitle>
          <Badge
            variant='secondary'
            title={props.pool.routing_group}
            className='max-w-full'
          >
            <span className='truncate'>
              {props.groupLabel && props.groupLabel !== props.pool.routing_group
                ? `${props.groupLabel} · ${props.pool.routing_group}`
                : props.pool.routing_group}
            </span>
          </Badge>
        </div>
        <div className='flex flex-wrap items-center justify-between gap-2'>
          <span className='text-muted-foreground text-xs'>
            {values.mode === 'smart'
              ? t('Smart routing')
              : t('Priority routing')}
          </span>
          <Button
            type='button'
            size='sm'
            variant='ghost'
            disabled={disabled}
            onClick={() => setEditingStrategy(true)}
          >
            {t('Edit strategy')}
          </Button>
        </div>
      </CardHeader>
      <form
        className='contents'
        onSubmit={(event) => {
          void form.handleSubmit((data) => save.mutate(data))(event)
        }}
      >
        <CardContent className='space-y-3 px-4'>
          {values.channels.length === 0 ? (
            <p
              role='status'
              className='text-muted-foreground rounded-md border border-dashed p-5 text-sm'
            >
              {t(
                'This explicit pool has no channels. Requests will be rejected until channels are added.'
              )}
            </p>
          ) : (
            <PoolChannelList
              channels={values.channels}
              candidates={available}
              disabled={disabled}
              onChange={(channels) =>
                form.setValue(
                  'channels',
                  channels.map((channel) => ({
                    ...channel,
                    priority: channel.priority ?? 0,
                    weight: channel.weight ?? 0,
                  })),
                  {
                    shouldDirty: true,
                    shouldValidate: true,
                  }
                )
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
          {adding && (
            <div className='flex min-w-0 flex-wrap gap-2'>
              <Combobox
                options={unused.map((candidate) => ({
                  value: String(candidate.channel_id),
                  label: `${candidate.channel_name} #${candidate.channel_id}`,
                }))}
                value={channelToAdd}
                onValueChange={setChannelToAdd}
                disabled={disabled || candidates.isPending}
                aria-label={t('Select channel')}
                placeholder={t('Select channel')}
                className='min-w-0 flex-1'
              />
              <Button
                type='button'
                size='sm'
                variant='outline'
                disabled={disabled || !channelToAdd || candidates.isPending}
                onClick={() => {
                  const candidate = unused.find(
                    (entry) => String(entry.channel_id) === channelToAdd
                  )
                  if (!candidate) return
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
                    { shouldDirty: true }
                  )
                  setChannelToAdd(null)
                }}
              >
                {t('Add')}
              </Button>
              {candidates.isError && (
                <Button
                  type='button'
                  size='sm'
                  variant='outline'
                  onClick={() => void candidates.refetch()}
                >
                  {t('Retry')}
                </Button>
              )}
            </div>
          )}
        </CardContent>
        <CardFooter className='flex flex-wrap justify-between gap-2 px-4'>
          <Button
            type='button'
            variant='outline'
            size='sm'
            disabled={disabled}
            onClick={() => setAdding(!adding)}
          >
            {t('Add channel')}
          </Button>
          <div className='flex flex-wrap items-center gap-2'>
            {dirty && <Badge variant='outline'>{t('Unsaved changes')}</Badge>}
            <Button
              type='button'
              variant='ghost'
              size='sm'
              disabled={disabled || !dirty}
              onClick={() =>
                form.reset({
                  mode: props.pool.mode,
                  channels: props.pool.channels,
                })
              }
            >
              {t('Reset')}
            </Button>
            <Button type='submit' size='sm' disabled={disabled || !dirty}>
              {save.isPending ? t('Saving...') : t('Save')}
            </Button>
          </div>
          <Button
            type='button'
            size='sm'
            variant='link'
            className='text-muted-foreground px-0 text-xs'
            disabled={disabled}
            onClick={() => setRestoring(true)}
          >
            {t('Restore channel defaults')}
          </Button>
        </CardFooter>
      </form>
      <Dialog
        open={editingStrategy}
        onOpenChange={setEditingStrategy}
        title={t('Edit scheduling strategy')}
        description={t(
          'Administrators configure the strategy for this model and public group. Clients do not select it.'
        )}
        footer={
          <Button type='button' onClick={() => setEditingStrategy(false)}>
            {t('Done')}
          </Button>
        }
      >
        <div className='space-y-4'>
          <PoolModeSelect
            value={values.mode}
            disabled={disabled}
            onChange={(mode) =>
              form.setValue('mode', mode, { shouldDirty: true })
            }
          />
          <p className='text-muted-foreground text-sm'>
            {t(
              'Smart routing adjusts traffic using recent first-token latency, output speed and failures. Non-streaming and media requests use pool priorities.'
            )}
          </p>
          <p className='text-muted-foreground text-sm'>
            {t('Save the card to apply strategy changes.')}
          </p>
        </div>
      </Dialog>
      <ConfirmDialog
        open={restoring}
        onOpenChange={setRestoring}
        title={t('Restore channel defaults')}
        desc={t(
          'Remove this pool and inherit the channel defaults again? An empty pool instead rejects requests.'
        )}
        confirmText={t('Restore')}
        isLoading={restore.isPending}
        handleConfirm={() => restore.mutate()}
      />
    </Card>
  )
}
