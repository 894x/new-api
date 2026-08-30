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
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Loader2, RotateCcw } from 'lucide-react'
import { useEffect, useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { Alert, AlertDescription } from '@/components/ui/alert'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Skeleton } from '@/components/ui/skeleton'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import {
  ADMIN_PERMISSION_ACTIONS,
  ADMIN_PERMISSION_RESOURCES,
  hasPermission,
} from '@/lib/admin-permissions'
import { useAuthStore } from '@/stores/auth-store'

import {
  getChannelModelRoutingOverrides,
  getModelChannelRoutingOverrides,
  patchChannelModelRoutingOverrides,
  patchModelChannelRoutingOverrides,
} from '../api'
import { channelsQueryKeys } from '../lib/channel-actions'
import {
  collectChangedModelRoutingOverrides,
  createModelRoutingOverrideDraftState,
  getModelRoutingOverrideKey,
  MAX_MODEL_ROUTING_WEIGHT,
  mergeModelRoutingOverrideDraftState,
  parseRoutingOverrideInput,
  resetModelRoutingOverrideDraft,
  updateModelRoutingOverrideDraftField,
  type ModelRoutingOverrideDraft,
} from '../lib/model-routing-overrides'
import type {
  ModelRoutingOverride,
  ModelRoutingOverridePatch,
  ModelRoutingOverridesResponse,
} from '../types'
import { ChannelStatusFilter } from './channel-status-filter'

type ModelRoutingOverridesEditorProps = {
  channelId?: number
  model?: string
  disabled?: boolean
  disabledReason?: string
}

const EMPTY_MODEL_ROUTING_OVERRIDES: ModelRoutingOverride[] = []

type ModelRoutingOverrideRowProps = {
  row: ModelRoutingOverride
  draft: ModelRoutingOverrideDraft
  showChannel: boolean
  disabled: boolean
  priorityInvalid: boolean
  weightInvalid: boolean
  onChange: (field: keyof ModelRoutingOverrideDraft, value: string) => void
  onReset: () => void
}

function ModelRoutingOverrideRow(props: ModelRoutingOverrideRowProps) {
  const { t } = useTranslation()
  const rowLabel = props.showChannel ? props.row.channel_name : props.row.model
  const draftPriority = parseRoutingOverrideInput(props.draft.priority_override)
  const draftWeight = parseRoutingOverrideInput(props.draft.weight_override)
  const effectivePriority =
    draftPriority === undefined
      ? props.row.effective_priority
      : (draftPriority ?? props.row.default_priority)
  const effectiveWeight =
    draftWeight === undefined ||
    (draftWeight ?? 0) < 0 ||
    (draftWeight ?? 0) > MAX_MODEL_ROUTING_WEIGHT
      ? props.row.effective_weight
      : (draftWeight ?? props.row.default_weight)

  return (
    <TableRow>
      <TableCell className='min-w-40 whitespace-normal'>
        <div className='flex flex-col gap-1'>
          <span className='font-medium break-all'>{rowLabel}</span>
          {props.showChannel && (
            <Badge
              variant={props.row.channel_status === 1 ? 'secondary' : 'outline'}
            >
              {props.row.channel_status === 1 ? t('Enabled') : t('Disabled')}
            </Badge>
          )}
        </div>
      </TableCell>
      <TableCell className='min-w-36'>
        <Input
          type='number'
          step={1}
          value={props.draft.priority_override}
          placeholder={`${t('Inherit')} (${props.row.default_priority})`}
          aria-label={`${t('Priority')} · ${rowLabel}`}
          aria-invalid={props.priorityInvalid}
          disabled={props.disabled}
          onChange={(event) =>
            props.onChange('priority_override', event.target.value)
          }
        />
        <div className='text-muted-foreground mt-1 text-xs'>
          {t('Effective')}: {effectivePriority}
        </div>
      </TableCell>
      <TableCell className='min-w-36'>
        <Input
          type='number'
          min={0}
          max={MAX_MODEL_ROUTING_WEIGHT}
          step={1}
          value={props.draft.weight_override}
          placeholder={`${t('Inherit')} (${props.row.default_weight})`}
          aria-label={`${t('Weight')} · ${rowLabel}`}
          aria-invalid={props.weightInvalid}
          disabled={props.disabled}
          onChange={(event) =>
            props.onChange('weight_override', event.target.value)
          }
        />
        <div className='text-muted-foreground mt-1 text-xs'>
          {t('Effective')}: {effectiveWeight}
        </div>
      </TableCell>
      <TableCell className='w-16 text-right'>
        <Button
          type='button'
          variant='ghost'
          size='icon-sm'
          aria-label={`${t('Reset')} · ${rowLabel}`}
          title={t('Reset to channel defaults')}
          disabled={
            props.disabled ||
            (props.draft.priority_override === '' &&
              props.draft.weight_override === '')
          }
          onClick={props.onReset}
        >
          <RotateCcw aria-hidden='true' />
        </Button>
      </TableCell>
    </TableRow>
  )
}

export function ModelRoutingOverridesEditor(
  props: ModelRoutingOverridesEditorProps
) {
  const { t } = useTranslation()
  const currentUser = useAuthStore((state) => state.auth.user)
  const canWrite = hasPermission(
    currentUser,
    ADMIN_PERMISSION_RESOURCES.CHANNEL,
    ADMIN_PERMISSION_ACTIONS.WRITE
  )
  const canRead = hasPermission(
    currentUser,
    ADMIN_PERMISSION_RESOURCES.CHANNEL,
    ADMIN_PERMISSION_ACTIONS.READ
  )
  const showChannel = props.channelId === undefined
  const queryKey = showChannel
    ? channelsQueryKeys.modelRoutingOverridesByModel(props.model || '')
    : channelsQueryKeys.modelRoutingOverridesByChannel(props.channelId || 0)

  const routingQuery = useQuery({
    queryKey,
    queryFn: () => {
      if (props.channelId !== undefined) {
        return getChannelModelRoutingOverrides(props.channelId)
      }
      if (props.model) return getModelChannelRoutingOverrides(props.model)
      throw new Error('A channel ID or model is required')
    },
    enabled: canRead && (props.channelId !== undefined || Boolean(props.model)),
  })

  if (!canRead) {
    return (
      <Alert>
        <AlertDescription>
          {t("You don't have necessary permission")}
        </AlertDescription>
      </Alert>
    )
  }

  if (routingQuery.isLoading) {
    return (
      <div className='space-y-3' aria-label={t('Model routing overrides')}>
        <Skeleton className='h-5 w-48' />
        <Skeleton className='h-24 w-full' />
      </div>
    )
  }

  if (routingQuery.isError) {
    return (
      <Alert variant='destructive'>
        <AlertDescription className='flex items-center justify-between gap-3'>
          <span>{t('Failed to load model routing overrides.')}</span>
          <Button
            type='button'
            variant='outline'
            size='sm'
            onClick={() => routingQuery.refetch()}
          >
            {t('Retry')}
          </Button>
        </AlertDescription>
      </Alert>
    )
  }

  return (
    <ModelRoutingOverrideRows
      key={showChannel ? props.model : props.channelId}
      rows={routingQuery.data?.data ?? EMPTY_MODEL_ROUTING_OVERRIDES}
      queryKey={queryKey}
      channelId={props.channelId}
      model={props.model}
      showChannel={showChannel}
      canWrite={canWrite}
      disabled={props.disabled === true}
      disabledReason={props.disabledReason}
    />
  )
}

type ModelRoutingOverrideRowsProps = {
  rows: ModelRoutingOverride[]
  queryKey: readonly unknown[]
  channelId?: number
  model?: string
  showChannel: boolean
  canWrite: boolean
  disabled: boolean
  disabledReason?: string
}

function ModelRoutingOverrideRows(props: ModelRoutingOverrideRowsProps) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const [showAllChannels, setShowAllChannels] = useState(false)
  const [draftState, setDraftState] = useState(() =>
    createModelRoutingOverrideDraftState(props.rows)
  )
  useEffect(() => {
    setDraftState((currentState) =>
      mergeModelRoutingOverrideDraftState(props.rows, currentState)
    )
  }, [props.rows])
  const serialization = useMemo(
    () => collectChangedModelRoutingOverrides(props.rows, draftState.drafts),
    [draftState.drafts, props.rows]
  )
  const invalidFields = useMemo(
    () =>
      new Set(
        serialization.errors.map((error) => `${error.key}:${error.field}`)
      ),
    [serialization.errors]
  )
  const visibleRows =
    props.showChannel && !showAllChannels
      ? props.rows.filter((row) => row.channel_status === 1)
      : props.rows

  const mutation = useMutation({
    mutationFn: async (
      overrides: ModelRoutingOverridePatch[]
    ): Promise<ModelRoutingOverridesResponse> => {
      const response =
        props.channelId !== undefined
          ? await patchChannelModelRoutingOverrides(props.channelId, overrides)
          : await patchModelChannelRoutingOverrides(
              props.model || '',
              overrides
            )
      if (!response.success) {
        throw new Error(
          response.message || t('Failed to update model routing overrides.')
        )
      }
      return response
    },
    onSuccess: (response) => {
      queryClient.setQueryData(props.queryKey, response)
      setDraftState(createModelRoutingOverrideDraftState(response.data || []))
      void queryClient.invalidateQueries({
        queryKey: channelsQueryKeys.modelRoutingOverrides(),
      })
      toast.success(t('Model routing overrides updated.'))
    },
    onError: (error) => {
      toast.error(
        error instanceof Error
          ? error.message
          : t('Failed to update model routing overrides.')
      )
    },
  })

  const updateDraft = (
    row: ModelRoutingOverride,
    field: keyof ModelRoutingOverrideDraft,
    value: string
  ) => {
    setDraftState((currentState) =>
      updateModelRoutingOverrideDraftField(currentState, row, field, value)
    )
  }

  const resetDraft = (row: ModelRoutingOverride) => {
    setDraftState((currentState) =>
      resetModelRoutingOverrideDraft(currentState, row)
    )
  }

  const saveOverrides = () => {
    if (serialization.errors.length > 0) {
      toast.error(
        t('Use whole numbers; weight must be between 0 and 2147483637.')
      )
      return
    }
    if (serialization.overrides.length === 0) return
    mutation.mutate(serialization.overrides)
  }

  return (
    <div className='space-y-3'>
      <div className='flex flex-wrap items-start justify-between gap-3'>
        <div className='space-y-1'>
          <h3 className='text-sm font-semibold'>
            {t('Model routing overrides')}
          </h3>
          <p className='text-muted-foreground text-sm'>
            {t(
              'Leave a field empty to inherit the channel default. Zero is saved as an explicit override.'
            )}
          </p>
        </div>
        {props.showChannel && props.rows.length > 0 && (
          <ChannelStatusFilter
            showAll={showAllChannels}
            onShowAllChange={setShowAllChannels}
          />
        )}
      </div>

      {!props.canWrite && (
        <Alert>
          <AlertDescription>
            {t('You have read-only access to routing overrides.')}
          </AlertDescription>
        </Alert>
      )}

      {props.disabled && props.disabledReason && (
        <Alert>
          <AlertDescription>{props.disabledReason}</AlertDescription>
        </Alert>
      )}

      {props.rows.length === 0 && (
        <div className='text-muted-foreground rounded-lg border border-dashed p-6 text-center text-sm'>
          {props.showChannel
            ? t('No channels support this exact model.')
            : t('This channel has no configured models.')}
        </div>
      )}

      {props.rows.length > 0 && visibleRows.length === 0 && (
        <div className='text-muted-foreground rounded-lg border border-dashed p-6 text-center text-sm'>
          {t('No enabled channels support this exact model.')}
        </div>
      )}

      {visibleRows.length > 0 && (
        <div className='rounded-lg border'>
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>
                  {props.showChannel ? t('Channel') : t('Model')}
                </TableHead>
                <TableHead>{t('Priority')}</TableHead>
                <TableHead>{t('Weight')}</TableHead>
                <TableHead>
                  <span className='sr-only'>{t('Reset')}</span>
                </TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {visibleRows.map((row) => {
                const key = getModelRoutingOverrideKey(
                  row.channel_id,
                  row.model
                )
                const draft = draftState.drafts[key]
                if (!draft) return null
                return (
                  <ModelRoutingOverrideRow
                    key={key}
                    row={row}
                    draft={draft}
                    showChannel={props.showChannel}
                    disabled={
                      !props.canWrite || props.disabled || mutation.isPending
                    }
                    priorityInvalid={invalidFields.has(
                      `${key}:priority_override`
                    )}
                    weightInvalid={invalidFields.has(`${key}:weight_override`)}
                    onChange={(field, value) => updateDraft(row, field, value)}
                    onReset={() => resetDraft(row)}
                  />
                )
              })}
            </TableBody>
          </Table>
        </div>
      )}

      {serialization.errors.length > 0 && (
        <p className='text-destructive text-sm' role='alert'>
          {t('Use whole numbers; weight must be between 0 and 2147483637.')}
        </p>
      )}

      {props.rows.length > 0 && props.canWrite && !props.disabled && (
        <div className='flex justify-end'>
          <Button
            type='button'
            size='sm'
            disabled={
              mutation.isPending ||
              serialization.errors.length > 0 ||
              serialization.overrides.length === 0
            }
            onClick={saveOverrides}
          >
            {mutation.isPending && (
              <Loader2 className='animate-spin' aria-hidden='true' />
            )}
            {t('Save routing overrides')}
          </Button>
        </div>
      )}
    </div>
  )
}
