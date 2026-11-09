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
import { Settings02Icon } from '@hugeicons/core-free-icons'
import { HugeiconsIcon } from '@hugeicons/react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useEffect, useEffectEvent, useId, useState } from 'react'
import { useForm, type FieldErrors } from 'react-hook-form'
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
import { Form } from '@/components/ui/form'
import { Label } from '@/components/ui/label'
import {
  Popover,
  PopoverContent,
  PopoverTitle,
  PopoverTrigger,
} from '@/components/ui/popover'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { dynamicRoutingSettingsSchema } from '@/features/system-settings/models/lib/dynamic-routing-schema'
import {
  ADMIN_PERMISSION_ACTIONS,
  ADMIN_PERMISSION_RESOURCES,
  hasPermission,
} from '@/lib/admin-permissions'
import { handleServerError } from '@/lib/handle-server-error'
import { cn } from '@/lib/utils'
import { useAuthStore } from '@/stores/auth-store'

import {
  deleteRoutingPool,
  DEFAULT_DISCOUNT_LIMITS,
  ROUTING_MODE_LABELS,
  getRoutingPool,
  routingPoolKeys,
  saveRoutingPool,
  type RoutingPoolView,
} from './api'
import { DiscountRoutingFields } from './discount-routing-fields'
import { earlyChunkPoolSchema } from './early-chunk'
import { EarlyChunkSettings } from './early-chunk-settings'
import { PoolChannelList } from './pool-channel-list'
import { PoolModeSelect } from './pool-mode-select'
import { PoolMonitorDisplay } from './pool-monitor-display'
import { PoolTuningPanel } from './pool-tuning-panel'

const poolFormSchema = z.object({
  mode: z.enum(['priority', 'smart', 'discount']),
  tuning: z
    .object(dynamicRoutingSettingsSchema.shape)
    .omit({ enabled: true })
    .partial()
    .extend({
      chat_prefix_enabled: z.boolean().optional(),
      chat_early_chunk: earlyChunkPoolSchema.optional(),
    }),
  max_ttft_ms: z.number().int().min(1).max(3600000),
  max_tpot_ms: z.number().int().min(1).max(60000),
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
export type PoolFormValues = z.infer<typeof poolFormSchema>

export function PoolCard(props: {
  pool: RoutingPoolView
  canWrite: boolean
  groupLabel?: string
  targetLabel?: string
  onDirtyChange: (dirty: boolean) => void
}) {
  const { t } = useTranslation()
  const prefixControlId = useId()
  const queryClient = useQueryClient()
  const actor = useAuthStore((state) => state.auth.user)
  const canMonitor = hasPermission(
    actor,
    ADMIN_PERMISSION_RESOURCES.CHANNEL,
    ADMIN_PERMISSION_ACTIONS.READ
  )
  const [monitorOpen, setMonitorOpen] = useState(false)
  const [tuningOpen, setTuningOpen] = useState(false)
  const [adding, setAdding] = useState(false)
  const [channelToAdd, setChannelToAdd] = useState<string | null>(null)
  const [settingsOpen, setSettingsOpen] = useState(false)
  const [editingStrategy, setEditingStrategy] = useState(false)
  const [restoring, setRestoring] = useState(false)
  const form = useForm<PoolFormValues>({
    resolver: zodResolver(poolFormSchema),
    defaultValues: {
      mode: props.pool.mode,
      tuning: props.pool.tuning ?? {},
      channels: props.pool.channels,
      max_ttft_ms:
        props.pool.max_ttft_ms || DEFAULT_DISCOUNT_LIMITS.max_ttft_ms,
      max_tpot_ms:
        props.pool.max_tpot_ms || DEFAULT_DISCOUNT_LIMITS.max_tpot_ms,
    },
  })
  const values = form.watch()
  let prefixValue = 'inherit'
  let prefixLabel = t('Inherit global')
  if (values.tuning.chat_prefix_enabled !== undefined) {
    prefixValue = String(values.tuning.chat_prefix_enabled)
    prefixLabel = values.tuning.chat_prefix_enabled
      ? t('Enabled')
      : t('Disabled')
  }
  const dirty = form.formState.isDirty
  const notifyDirty = useEffectEvent(props.onDirtyChange)
  useEffect(() => {
    notifyDirty(dirty)
  }, [dirty])
  useEffect(() => () => notifyDirty(false), [])
  useEffect(() => {
    if (!dirty) {
      form.reset({
        mode: props.pool.mode,
        tuning: props.pool.tuning ?? {},
        channels: props.pool.channels,
        max_ttft_ms:
          props.pool.max_ttft_ms || DEFAULT_DISCOUNT_LIMITS.max_ttft_ms,
        max_tpot_ms:
          props.pool.max_tpot_ms || DEFAULT_DISCOUNT_LIMITS.max_tpot_ms,
      })
    }
  }, [
    props.pool.revision,
    dirty,
    form,
    props.pool.mode,
    props.pool.channels,
    props.pool.max_ttft_ms,
    props.pool.max_tpot_ms,
    props.pool.tuning,
  ])
  const candidates = useQuery({
    queryKey: routingPoolKeys.detail(
      props.pool.routing_group,
      props.pool.model
    ),
    queryFn: () => getRoutingPool(props.pool.routing_group, props.pool.model),
    enabled: tuningOpen || (adding && props.canWrite),
    refetchOnWindowFocus: false,
    meta: { errorToast: false },
  })
  const save = useMutation({
    mutationFn: (data: PoolFormValues) => {
      if (!props.canWrite) {
        throw new Error(t('No permission to perform this action'))
      }
      return saveRoutingPool({
        ...props.pool,
        ...data,
        max_ttft_ms: data.mode === 'discount' ? data.max_ttft_ms : 0,
        max_tpot_ms: data.mode === 'discount' ? data.max_tpot_ms : 0,
      })
    },
    onSuccess: async (pool) => {
      form.reset({
        mode: pool.mode,
        tuning: pool.tuning ?? {},
        channels: pool.channels,
        max_ttft_ms: pool.max_ttft_ms || DEFAULT_DISCOUNT_LIMITS.max_ttft_ms,
        max_tpot_ms: pool.max_tpot_ms || DEFAULT_DISCOUNT_LIMITS.max_tpot_ms,
      })
      toast.success(t('Saved successfully'))
      await Promise.all([
        queryClient.invalidateQueries({ queryKey: routingPoolKeys.all }),
        queryClient.invalidateQueries({ queryKey: ['user-routing-pools'] }),
        queryClient.invalidateQueries({ queryKey: ['routing-monitor-pool'] }),
        queryClient.invalidateQueries({ queryKey: ['routing-monitor'] }),
      ])
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
      await Promise.all([
        queryClient.invalidateQueries({ queryKey: routingPoolKeys.all }),
        queryClient.invalidateQueries({ queryKey: ['user-routing-pools'] }),
        queryClient.invalidateQueries({ queryKey: ['routing-monitor-pool'] }),
        queryClient.invalidateQueries({ queryKey: ['routing-monitor'] }),
      ])
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

  const tuningDefaults =
    candidates.data?.tuning_defaults ?? props.pool.tuning_defaults
  const revealTuning = () => {
    if (canMonitor) setMonitorOpen(true)
    setTuningOpen(true)
  }
  const handleInvalid = (errors: FieldErrors<PoolFormValues>) => {
    if (errors.tuning) revealTuning()
  }
  const validateAndSave = (data: PoolFormValues) => {
    form.clearErrors()
    const numericTuning = Object.entries(data.tuning).filter(
      ([key, value]) =>
        key !== 'chat_prefix_enabled' &&
        key !== 'chat_early_chunk' &&
        value !== undefined
    )
    if (numericTuning.length > 0) {
      if (!tuningDefaults) {
        form.setError('root.tuning', {
          message: t('Scheduling defaults are unavailable'),
        })
        revealTuning()
        return
      }
      const result = dynamicRoutingSettingsSchema.safeParse({
        ...tuningDefaults,
        enabled: true,
        ...Object.fromEntries(numericTuning),
      })
      if (!result.success) {
        for (const issue of result.error.issues) {
          const field = issue.path[0] as keyof PoolFormValues['tuning']
          form.setError(`tuning.${field}`, { message: t(issue.message) })
        }
        revealTuning()
        return
      }
    }
    save.mutate(data)
  }

  return (
    <PoolMonitorDisplay
      open={monitorOpen && canMonitor}
      onOpenChange={setMonitorOpen}
      model={props.pool.model}
      group={props.pool.routing_group}
      groupLabel={props.groupLabel}
      channels={available.filter((candidate) =>
        values.channels.some(
          (entry) => entry.channel_id === candidate.channel_id
        )
      )}
      toolbarActions={
        <Form {...form}>
          <PoolTuningPanel
            open={tuningOpen}
            onOpenChange={setTuningOpen}
            defaults={tuningDefaults}
            loading={candidates.isFetching && !tuningDefaults}
            disabled={disabled}
            saving={save.isPending}
            onSave={() =>
              void form.handleSubmit(validateAndSave, handleInvalid)()
            }
            onRetry={() => void candidates.refetch()}
          />
        </Form>
      }
    >
      <Card
        className={cn(
          'min-w-0 gap-4 py-4',
          monitorOpen && canMonitor && 'xl:h-full xl:min-h-0'
        )}
      >
        <CardHeader className='shrink-0 gap-3 px-3'>
          <div className='flex min-w-0 flex-wrap items-start justify-between gap-2'>
            <CardTitle className='min-w-0 text-base break-all'>
              {props.pool.model}
              {props.targetLabel && (
                <span className='text-muted-foreground block text-xs font-normal'>
                  {props.targetLabel}
                </span>
              )}
            </CardTitle>
            <div className='flex max-w-full flex-wrap items-center justify-end gap-2'>
              <Badge
                variant='secondary'
                title={props.pool.routing_group}
                className='max-w-full'
              >
                <span className='truncate'>
                  {props.groupLabel &&
                  props.groupLabel !== props.pool.routing_group
                    ? `${props.groupLabel} · ${props.pool.routing_group}`
                    : props.pool.routing_group}
                </span>
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
              <Popover open={settingsOpen} onOpenChange={setSettingsOpen}>
                <PopoverTrigger
                  render={
                    <Button
                      type='button'
                      size='icon-sm'
                      variant='ghost'
                      aria-label={`${t('Settings')}: ${props.pool.model}`}
                      title={t('Settings')}
                    />
                  }
                >
                  <HugeiconsIcon
                    icon={Settings02Icon}
                    strokeWidth={2}
                    aria-hidden='true'
                  />
                </PopoverTrigger>
                <PopoverContent
                  keepMounted
                  align='end'
                  sideOffset={8}
                  role='dialog'
                  className='max-h-[min(40rem,calc(100dvh-2rem))] w-[min(26rem,calc(100vw-1.5rem))] gap-4 overflow-y-auto overscroll-contain p-4'
                >
                  <PopoverTitle>{`${t('Settings')}: ${props.pool.model}`}</PopoverTitle>
                  <div className='flex flex-wrap items-center justify-between gap-2'>
                    <Button
                      type='button'
                      size='sm'
                      variant='ghost'
                      disabled={disabled}
                      onClick={() => {
                        setSettingsOpen(false)
                        setEditingStrategy(true)
                      }}
                    >
                      {t('Edit strategy')}
                    </Button>
                  </div>
                  <div className='space-y-1.5'>
                    <Label htmlFor={prefixControlId}>
                      {t('Prefix cache routing')}
                    </Label>
                    <Select
                      value={prefixValue}
                      onValueChange={(value) =>
                        form.setValue(
                          'tuning.chat_prefix_enabled',
                          value === 'inherit' ? undefined : value === 'true',
                          { shouldDirty: true }
                        )
                      }
                      disabled={disabled}
                    >
                      <SelectTrigger id={prefixControlId} className='w-full'>
                        <SelectValue>{prefixLabel}</SelectValue>
                      </SelectTrigger>
                      <SelectContent>
                        <SelectItem value='inherit'>
                          {t('Inherit global')}
                        </SelectItem>
                        <SelectItem value='true'>{t('Enabled')}</SelectItem>
                        <SelectItem value='false'>{t('Disabled')}</SelectItem>
                      </SelectContent>
                    </Select>
                    <p className='text-muted-foreground text-xs'>
                      {t(
                        'Override prefix cache routing for this pool. Capacity and lifetime use the global settings.'
                      )}
                    </p>
                  </div>
                  <EarlyChunkSettings
                    value={values.tuning.chat_early_chunk}
                    disabled={disabled}
                    onChange={(value) =>
                      form.setValue('tuning.chat_early_chunk', value, {
                        shouldDirty: true,
                      })
                    }
                  />
                </PopoverContent>
              </Popover>
            </div>
          </div>
          <span className='text-muted-foreground text-xs'>
            {t(ROUTING_MODE_LABELS[values.mode])}
          </span>
        </CardHeader>
        <form
          className='contents'
          onSubmit={(event) => {
            void form.handleSubmit(validateAndSave, handleInvalid)(event)
          }}
        >
          <CardContent
            className={cn(
              'space-y-3 px-3',
              monitorOpen &&
                canMonitor &&
                'xl:min-h-0 xl:flex-1 xl:overflow-y-auto'
            )}
          >
            {(form.formState.errors.max_ttft_ms ||
              form.formState.errors.max_tpot_ms) && (
              <p role='alert' className='text-destructive text-sm'>
                {t('Enter valid positive latency limits in milliseconds.')}
              </p>
            )}
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
                model={props.pool.model}
                validationAttempt={form.formState.submitCount}
                channels={values.channels}
                candidates={available}
                disabled={disabled}
                earlyChunk={values.tuning.chat_early_chunk}
                onEarlyChunkChange={(value) =>
                  form.setValue('tuning.chat_early_chunk', value, {
                    shouldDirty: true,
                  })
                }
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
          <CardFooter className='flex shrink-0 flex-wrap justify-between gap-2 px-3'>
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
                    tuning: props.pool.tuning ?? {},
                    channels: props.pool.channels,
                    max_ttft_ms:
                      props.pool.max_ttft_ms ||
                      DEFAULT_DISCOUNT_LIMITS.max_ttft_ms,
                    max_tpot_ms:
                      props.pool.max_tpot_ms ||
                      DEFAULT_DISCOUNT_LIMITS.max_tpot_ms,
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
              onChange={(mode) => {
                form.setValue('mode', mode, { shouldDirty: true })
                if (mode !== 'discount') {
                  form.setValue(
                    'max_ttft_ms',
                    DEFAULT_DISCOUNT_LIMITS.max_ttft_ms
                  )
                  form.setValue(
                    'max_tpot_ms',
                    DEFAULT_DISCOUNT_LIMITS.max_tpot_ms
                  )
                  form.clearErrors(['max_ttft_ms', 'max_tpot_ms'])
                }
              }}
            />
            <p className='text-muted-foreground text-sm'>
              {t(
                'Smart routing adjusts traffic using recent first-token latency, output speed and failures. Non-streaming and media requests use pool priorities.'
              )}
            </p>
            {values.mode === 'discount' && (
              <DiscountRoutingFields
                maxTTFT={values.max_ttft_ms}
                maxTPOT={values.max_tpot_ms}
                disabled={disabled}
                onChange={(field, value) =>
                  form.setValue(field, value, { shouldDirty: true })
                }
                invalidTTFT={!!form.formState.errors.max_ttft_ms}
                invalidTPOT={!!form.formState.errors.max_tpot_ms}
              />
            )}
            {(form.formState.errors.max_ttft_ms ||
              form.formState.errors.max_tpot_ms) && (
              <p role='alert' className='text-destructive text-sm'>
                {t('Enter valid positive latency limits in milliseconds.')}
              </p>
            )}
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
    </PoolMonitorDisplay>
  )
}
