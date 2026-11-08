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
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { useState } from 'react'
import { useForm } from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { z } from 'zod'

import { Dialog } from '@/components/dialog'
import { Button } from '@/components/ui/button'
import { canAccessUserGroupConfig } from '@/lib/admin-permissions'
import { handleServerError } from '@/lib/handle-server-error'
import { useAuthStore } from '@/stores/auth-store'

import {
  DEFAULT_DISCOUNT_LIMITS,
  ROUTING_MODE_LABELS,
  routingPoolKeys,
  saveUserRoutingStrategy,
  userRoutingPoolKeys,
  type UserRoutingPoolConfig,
} from './api'
import { DiscountRoutingFields } from './discount-routing-fields'
import { earlyChunkPoolSchema } from './early-chunk'
import { EarlyChunkSettings } from './early-chunk-settings'
import { PoolModeSelect } from './pool-mode-select'

const schema = z.object({
  mode: z.enum(['inherit', 'priority', 'smart', 'discount']),
  max_ttft_ms: z.number().int().min(1).max(3600000),
  max_tpot_ms: z.number().int().min(1).max(60000),
  early_chunk: earlyChunkPoolSchema,
})
type Values = z.infer<typeof schema>

export function UserPoolStrategy(props: {
  config: UserRoutingPoolConfig
  disabled: boolean
}) {
  const { t } = useTranslation()
  const actor = useAuthStore((state) => state.auth.user)
  const [open, setOpen] = useState(false)
  const overridden = props.config.strategy_user_override
  return (
    <div className='flex flex-wrap items-center gap-2'>
      <span className='text-muted-foreground'>
        {t(
          overridden
            ? 'Strategy: {{strategy}}'
            : 'Strategy: {{strategy}} (inherited)',
          {
            strategy: t(ROUTING_MODE_LABELS[props.config.mode]),
          }
        )}
      </span>
      <Button
        type='button'
        size='sm'
        variant='outline'
        disabled={
          props.disabled ||
          !canAccessUserGroupConfig(actor, props.config.user_role, 'write')
        }
        onClick={() => setOpen(true)}
      >
        {t('Edit scheduling strategy')}
      </Button>
      {open && (
        <UserStrategyEditor
          config={props.config}
          onClose={() => setOpen(false)}
        />
      )}
    </div>
  )
}

function UserStrategyEditor(props: {
  config: UserRoutingPoolConfig
  onClose: () => void
}) {
  const { t } = useTranslation()
  const client = useQueryClient()
  const actor = useAuthStore((state) => state.auth.user)
  const [base] = useState(props.config)
  const form = useForm<Values>({
    resolver: zodResolver(schema),
    defaultValues: {
      mode: base.strategy_mode ?? 'inherit',
      early_chunk: base.early_chunk ?? {},
      max_ttft_ms: base.max_ttft_ms || DEFAULT_DISCOUNT_LIMITS.max_ttft_ms,
      max_tpot_ms: base.max_tpot_ms || DEFAULT_DISCOUNT_LIMITS.max_tpot_ms,
    },
  })
  const values = form.watch()
  const save = useMutation({
    mutationFn: (data: Values) => {
      if (
        !canAccessUserGroupConfig(
          useAuthStore.getState().auth.user,
          base.user_role,
          'write'
        )
      ) {
        throw new Error(t('No permission to perform this action'))
      }
      const { early_chunk, ...strategy } = data
      return saveUserRoutingStrategy(
        base.user_id,
        base.model,
        base.routing_group ?? '',
        {
          ...strategy,
          ...(form.formState.dirtyFields.early_chunk ? { early_chunk } : {}),
          revision: base.strategy_revision ?? '',
          max_ttft_ms: data.mode === 'discount' ? data.max_ttft_ms : 0,
          max_tpot_ms: data.mode === 'discount' ? data.max_tpot_ms : 0,
        }
      )
    },
    onSuccess: async () => {
      toast.success(t('Saved successfully'))
      await Promise.all([
        client.invalidateQueries({ queryKey: routingPoolKeys.all }),
        client.invalidateQueries({
          queryKey: userRoutingPoolKeys.all(base.user_id),
        }),
        client.invalidateQueries({ queryKey: ['routing-monitor-pool'] }),
        client.invalidateQueries({ queryKey: ['routing-monitor'] }),
      ])
      props.onClose()
    },
    onError: (error) => handleServerError(error, t('Failed to save')),
  })
  const disabled =
    save.isPending || !canAccessUserGroupConfig(actor, base.user_role, 'write')
  const changeMode = (mode: Values['mode']) => {
    form.setValue('mode', mode, { shouldDirty: true })
    if (mode !== 'discount') {
      form.setValue('max_ttft_ms', DEFAULT_DISCOUNT_LIMITS.max_ttft_ms)
      form.setValue('max_tpot_ms', DEFAULT_DISCOUNT_LIMITS.max_tpot_ms)
      form.clearErrors(['max_ttft_ms', 'max_tpot_ms'])
    }
  }
  return (
    <Dialog
      open
      onOpenChange={(open) => {
        if (!open && !save.isPending) props.onClose()
      }}
      title={t('Edit scheduling strategy')}
      description={t(
        'Override the strategy for this user, model and public group. Channel membership remains unchanged.'
      )}
      footer={
        <>
          <Button
            variant='ghost'
            disabled={save.isPending}
            onClick={props.onClose}
          >
            {t('Cancel')}
          </Button>
          <Button
            disabled={disabled || !form.formState.isDirty}
            onClick={() =>
              void form.handleSubmit((data) => save.mutate(data))()
            }
          >
            {save.isPending ? t('Saving...') : t('Save')}
          </Button>
        </>
      }
    >
      <div className='space-y-4'>
        <EarlyChunkSettings
          value={values.early_chunk}
          disabled={disabled}
          onChange={(value) =>
            form.setValue('early_chunk', value, { shouldDirty: true })
          }
        />
        <PoolModeSelect
          value={values.mode === 'inherit' ? base.mode : values.mode}
          inherited={values.mode === 'inherit'}
          onInherit={() => changeMode('inherit')}
          onChange={changeMode}
          disabled={disabled}
        />
        {values.mode === 'discount' && (
          <DiscountRoutingFields
            maxTTFT={values.max_ttft_ms}
            maxTPOT={values.max_tpot_ms}
            onChange={(field, value) =>
              form.setValue(field, value, { shouldDirty: true })
            }
            invalidTTFT={!!form.formState.errors.max_ttft_ms}
            invalidTPOT={!!form.formState.errors.max_tpot_ms}
            disabled={disabled}
          />
        )}
        {(form.formState.errors.max_ttft_ms ||
          form.formState.errors.max_tpot_ms) && (
          <p role='alert' className='text-destructive text-sm'>
            {t('Enter valid positive latency limits in milliseconds.')}
          </p>
        )}
        {save.isError && (
          <p role='alert' className='text-destructive text-sm'>
            {t(
              'Changes were not saved. Reload if another administrator changed this configuration.'
            )}
          </p>
        )}
      </div>
    </Dialog>
  )
}
