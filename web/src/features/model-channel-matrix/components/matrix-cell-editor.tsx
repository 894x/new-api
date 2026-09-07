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
import { useForm } from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { z } from 'zod'

import { Alert, AlertDescription } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import {
  Field,
  FieldDescription,
  FieldError,
  FieldGroup,
  FieldLabel,
} from '@/components/ui/field'
import { Input } from '@/components/ui/input'
import { patchChannelModelRoutingOverrides } from '@/features/channels/api'
import { channelsQueryKeys } from '@/features/channels/lib/channel-actions'
import {
  MAX_MODEL_ROUTING_WEIGHT,
  parseRoutingOverrideInput,
} from '@/features/channels/lib/model-routing-overrides'
import { toIntlLocale } from '@/i18n/languages'

import type { MatrixCell } from '../types'

const overrideFields = [
  'rpm_override',
  'tpm_override',
  'priority_override',
  'weight_override',
] as const
type OverrideField = (typeof overrideFields)[number]
type OverrideValues = Record<OverrideField, string>

export function MatrixCellEditor(props: {
  cell: MatrixCell
  canWrite: boolean
  trigger: HTMLElement | null
  onClose: () => void
}) {
  const { t, i18n } = useTranslation()
  const queryClient = useQueryClient()
  const labels = {
    rpm_override: t('RPM'),
    tpm_override: t('TPM'),
    priority_override: t('Priority'),
    weight_override: t('Weight'),
  }
  const defaults = {
    rpm_override: props.cell.default_rpm ?? 0,
    tpm_override: props.cell.default_tpm ?? 0,
    priority_override: props.cell.default_priority,
    weight_override: props.cell.default_weight,
  }
  const schema = z
    .object({
      rpm_override: z.string(),
      tpm_override: z.string(),
      priority_override: z.string(),
      weight_override: z.string(),
    })
    .superRefine((values, ctx) => {
      for (const field of overrideFields) {
        const value = parseRoutingOverrideInput(values[field])
        const min = field === 'priority_override' ? -Number.MAX_SAFE_INTEGER : 0
        const max =
          field === 'weight_override'
            ? MAX_MODEL_ROUTING_WEIGHT
            : Number.MAX_SAFE_INTEGER
        if (
          value === undefined ||
          (value !== null && (value < min || value > max))
        ) {
          ctx.addIssue({
            code: 'custom',
            path: [field],
            message: t('Enter an integer between {{min}} and {{max}}.', {
              min,
              max,
            }),
          })
        }
      }
    })
  const form = useForm<OverrideValues>({
    resolver: zodResolver(schema),
    defaultValues: {
      rpm_override:
        props.cell.rpm_override == null ? '' : String(props.cell.rpm_override),
      tpm_override:
        props.cell.tpm_override == null ? '' : String(props.cell.tpm_override),
      priority_override:
        props.cell.priority_override == null
          ? ''
          : String(props.cell.priority_override),
      weight_override:
        props.cell.weight_override == null
          ? ''
          : String(props.cell.weight_override),
    },
  })
  const mutation = useMutation({
    mutationFn: async (values: OverrideValues) => {
      const response = await patchChannelModelRoutingOverrides(
        props.cell.channel_id,
        [
          {
            channel_id: props.cell.channel_id,
            model: props.cell.model,
            rpm_override:
              parseRoutingOverrideInput(values.rpm_override) ?? null,
            tpm_override:
              parseRoutingOverrideInput(values.tpm_override) ?? null,
            priority_override:
              parseRoutingOverrideInput(values.priority_override) ?? null,
            weight_override:
              parseRoutingOverrideInput(values.weight_override) ?? null,
          },
        ]
      )
      if (!response.success) {
        throw new Error('Failed to update model routing overrides.')
      }
      return response
    },
    onSuccess: async (response) => {
      queryClient.setQueryData(
        channelsQueryKeys.modelRoutingOverridesByChannel(props.cell.channel_id),
        response
      )
      await Promise.all([
        queryClient.invalidateQueries({ queryKey: channelsQueryKeys.all }),
        queryClient.invalidateQueries({ queryKey: ['models'] }),
      ])
      toast.success(t('Model routing overrides updated.'))
      props.onClose()
    },
  })
  const values = form.watch()
  const disabled = !props.canWrite || mutation.isPending

  return (
    <Dialog
      open
      onOpenChange={(open) => {
        if (!open && !mutation.isPending) props.onClose()
      }}
    >
      <DialogContent
        className='max-h-[90dvh] overflow-y-auto sm:max-w-xl'
        showCloseButton={false}
        finalFocus={() => props.trigger}
      >
        <DialogHeader>
          <DialogTitle>{t('Model routing overrides')}</DialogTitle>
          <DialogDescription className='break-all'>
            {props.cell.model} · {props.cell.channel_name} (#
            {props.cell.channel_id})
          </DialogDescription>
        </DialogHeader>
        <div className='text-muted-foreground text-sm break-all'>
          {t('Upstream model')}:{' '}
          {props.cell.mapping_error
            ? t('Invalid model mapping')
            : props.cell.upstream_model}
        </div>
        <p className='text-muted-foreground text-sm'>
          {t(
            'Leave a field empty to inherit the channel default. Zero is saved as an explicit override.'
          )}
        </p>
        {!props.canWrite && (
          <Alert>
            <AlertDescription>
              {t('You have read-only access to routing overrides.')}
            </AlertDescription>
          </Alert>
        )}
        <form
          noValidate
          onSubmit={form.handleSubmit((fields) => {
            if (props.canWrite) mutation.mutate(fields)
          })}
          className='flex flex-col gap-4'
        >
          <FieldGroup className='grid grid-cols-1 gap-4 sm:grid-cols-2'>
            {overrideFields.map((field) => {
              const error = form.formState.errors[field]
              const parsed = parseRoutingOverrideInput(values[field])
              const effective = parsed ?? defaults[field]
              const capacity =
                field === 'rpm_override' || field === 'tpm_override'
              let effectiveLabel = '—'
              if (!error) {
                effectiveLabel =
                  capacity && effective === 0
                    ? t('Unlimited')
                    : effective.toLocaleString(toIntlLocale(i18n.language))
              }
              return (
                <Field
                  key={field}
                  data-invalid={Boolean(error)}
                  data-disabled={disabled}
                >
                  <FieldLabel htmlFor={`matrix-${field}`}>
                    {labels[field]}
                  </FieldLabel>
                  <Input
                    id={`matrix-${field}`}
                    type='text'
                    inputMode={
                      field === 'priority_override' ? 'text' : 'numeric'
                    }
                    autoComplete='off'
                    placeholder={`${t('Inherit')} (${defaults[field]})`}
                    aria-invalid={Boolean(error)}
                    aria-describedby={`matrix-${field}-description${error ? ` matrix-${field}-error` : ''}`}
                    disabled={disabled}
                    {...form.register(field)}
                  />
                  <FieldDescription id={`matrix-${field}-description`}>
                    {t('Effective')}: {effectiveLabel}
                  </FieldDescription>
                  {error && (
                    <FieldError id={`matrix-${field}-error`}>
                      {error.message}
                    </FieldError>
                  )}
                </Field>
              )
            })}
          </FieldGroup>
          <p className='text-muted-foreground text-xs'>
            {t(
              'RPM/TPM: 0 means unlimited. Priority is considered before weight; weight 0 still participates in selection.'
            )}
          </p>
          {mutation.isError && (
            <Alert variant='destructive'>
              <AlertDescription>
                {t('Failed to update model routing overrides.')}
              </AlertDescription>
            </Alert>
          )}
          <DialogFooter className='flex-wrap'>
            {props.canWrite && (
              <Button
                type='button'
                variant='ghost'
                disabled={disabled}
                onClick={() => {
                  for (const field of overrideFields) {
                    form.setValue(field, '', {
                      shouldDirty: true,
                      shouldValidate: true,
                    })
                  }
                }}
              >
                {t('Reset to channel defaults')}
              </Button>
            )}
            <Button
              type='button'
              variant='outline'
              disabled={mutation.isPending}
              onClick={props.onClose}
            >
              {t('Close')}
            </Button>
            {props.canWrite && (
              <Button
                type='submit'
                disabled={disabled || !form.formState.isDirty}
              >
                {mutation.isPending ? t('Saving...') : t('Save')}
              </Button>
            )}
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}
