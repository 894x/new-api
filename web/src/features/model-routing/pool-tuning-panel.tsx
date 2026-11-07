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
import { SlidersHorizontal } from 'lucide-react'
import { useState } from 'react'
import { useFormContext } from 'react-hook-form'
import { useTranslation } from 'react-i18next'

import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import {
  Collapsible,
  CollapsibleContent,
  CollapsibleTrigger,
} from '@/components/ui/collapsible'
import {
  FormControl,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from '@/components/ui/form'
import { Input } from '@/components/ui/input'
import {
  Popover,
  PopoverContent,
  PopoverDescription,
  PopoverTitle,
  PopoverTrigger,
} from '@/components/ui/popover'
import {
  DYNAMIC_ROUTING_RECOVERY_FIELDS,
  DYNAMIC_ROUTING_SAMPLING_FIELDS,
  DYNAMIC_ROUTING_SENSITIVITY_FIELDS,
  type DynamicRoutingNumericFieldDefinition,
} from '@/features/system-settings/models/lib/dynamic-routing-fields'
import type { DynamicRoutingSettings } from '@/features/system-settings/types'
import { toIntlLocale } from '@/i18n/languages'

import type { PoolFormValues } from './pool-card'

const recoveryFields = [
  ...DYNAMIC_ROUTING_RECOVERY_FIELDS,
  ...DYNAMIC_ROUTING_SAMPLING_FIELDS.filter(
    (field) =>
      field.name.endsWith('.min_samples') ||
      field.name.endsWith('.probe_fraction')
  ),
]
const advancedFields = [
  ...DYNAMIC_ROUTING_SAMPLING_FIELDS.filter(
    (field) =>
      field.name.endsWith('.max_samples') ||
      field.name.endsWith('.max_age_seconds')
  ),
  ...DYNAMIC_ROUTING_SENSITIVITY_FIELDS,
]

function PoolTuningField(props: {
  definition: DynamicRoutingNumericFieldDefinition
  defaults: DynamicRoutingSettings
  disabled: boolean
}) {
  const { t, i18n } = useTranslation()
  const form = useFormContext<PoolFormValues>()
  const key = props.definition.name.slice(
    'dynamic_routing_setting.'.length
  ) as Exclude<keyof PoolFormValues['tuning'], 'chat_prefix_enabled'>
  // The shared display formatter rounds to two places; probe fractions and
  // recovery steps require three to preserve the actual inherited value.
  const defaultValue = new Intl.NumberFormat(
    toIntlLocale(i18n.resolvedLanguage || i18n.language),
    { maximumFractionDigits: 3 }
  ).format(props.defaults[key])
  return (
    <FormField
      control={form.control}
      name={`tuning.${key}`}
      render={({ field }) => (
        <FormItem className='min-w-0 gap-1.5'>
          <div className='flex flex-wrap items-center justify-between gap-1'>
            <FormLabel className='text-xs leading-4'>
              {t(props.definition.labelKey)}
            </FormLabel>
            <span className='text-muted-foreground text-[10px]'>
              {field.value === undefined ? t('Inherited') : t('Override')}
            </span>
          </div>
          <FormControl>
            <Input
              {...field}
              value={
                field.value !== undefined && Number.isFinite(field.value)
                  ? field.value
                  : ''
              }
              type='number'
              min={props.definition.min}
              max={props.definition.max}
              step={props.definition.step}
              placeholder={String(props.defaults[key])}
              disabled={props.disabled}
              onChange={(event) => {
                form.clearErrors('tuning')
                field.onChange(
                  event.target.value === ''
                    ? undefined
                    : event.target.valueAsNumber
                )
              }}
            />
          </FormControl>
          <span className='text-muted-foreground text-[11px]'>
            {t('Global default: {{value}}', { value: defaultValue })}
          </span>
          <FormMessage className='text-xs' />
        </FormItem>
      )}
    />
  )
}

export function PoolTuningPanel(props: {
  open: boolean
  onOpenChange: (open: boolean) => void
  defaults?: DynamicRoutingSettings
  loading: boolean
  disabled: boolean
  saving: boolean
  onSave: () => void
  onRetry: () => void
}) {
  const { t } = useTranslation()
  const form = useFormContext<PoolFormValues>()
  const tuning = form.watch('tuning')
  const [advancedOpen, setAdvancedOpen] = useState(false)
  const advancedInvalid = advancedFields.some((definition) =>
    Boolean(
      form.formState.errors.tuning?.[
        definition.name.slice(
          'dynamic_routing_setting.'.length
        ) as keyof PoolFormValues['tuning']
      ]
    )
  )
  const defaults = props.defaults
  const overridden = Object.entries(tuning).some(
    ([key, value]) => key !== 'chat_prefix_enabled' && value !== undefined
  )
  return (
    <Popover open={props.open} onOpenChange={props.onOpenChange}>
      <PopoverTrigger
        render={
          <Button
            type='button'
            variant='outline'
            size='sm'
            aria-label={t('Scheduling parameters')}
          />
        }
      >
        <SlidersHorizontal className='size-4' aria-hidden='true' />
        <span className='hidden sm:inline'>{t('Scheduling parameters')}</span>
        {overridden && (
          <span
            className='bg-primary size-1.5 rounded-full'
            aria-hidden='true'
          />
        )}
      </PopoverTrigger>
      <PopoverContent
        align='end'
        sideOffset={8}
        role='dialog'
        className='max-h-[min(40rem,calc(100dvh-7rem))] w-[min(36rem,calc(100vw-1.5rem))] gap-0 overflow-hidden p-0'
      >
        <div className='shrink-0 space-y-2 border-b p-4'>
          <div className='flex flex-wrap items-center justify-between gap-2'>
            <PopoverTitle>{t('Scheduling parameters')}</PopoverTitle>
            <Badge variant='secondary'>{t('This pool only')}</Badge>
          </div>
          <PopoverDescription className='text-xs leading-5'>
            {t(
              'Leave blank to inherit the current global default. Changes apply only to this pool.'
            )}
          </PopoverDescription>
        </div>
        <div className='min-h-0 space-y-4 overflow-y-auto overscroll-contain p-4'>
          {defaults ? (
            <>
              <div className='grid min-w-0 grid-cols-1 gap-4 sm:grid-cols-2'>
                {recoveryFields.map((definition) => (
                  <PoolTuningField
                    key={definition.name}
                    definition={definition}
                    defaults={defaults}
                    disabled={props.disabled}
                  />
                ))}
              </div>
              <Collapsible
                open={advancedOpen || advancedInvalid}
                onOpenChange={setAdvancedOpen}
              >
                <CollapsibleTrigger
                  render={
                    <Button
                      type='button'
                      variant='ghost'
                      size='sm'
                      className='w-full justify-between'
                    />
                  }
                >
                  {t('Advanced settings')}
                </CollapsibleTrigger>
                <CollapsibleContent>
                  <div className='grid min-w-0 grid-cols-1 gap-4 pt-3 sm:grid-cols-2'>
                    {advancedFields.map((definition) => (
                      <PoolTuningField
                        key={definition.name}
                        definition={definition}
                        defaults={defaults}
                        disabled={props.disabled}
                      />
                    ))}
                  </div>
                </CollapsibleContent>
              </Collapsible>
              <p className='text-muted-foreground text-xs'>
                {t('Used by smart and discount-first routing.')}
              </p>
              {form.formState.errors.root?.tuning?.message && (
                <p role='alert' className='text-destructive text-xs'>
                  {form.formState.errors.root.tuning.message}
                </p>
              )}
            </>
          ) : (
            <div
              role={props.loading ? 'status' : 'alert'}
              className='space-y-2 text-sm'
            >
              <p>
                {props.loading
                  ? t('Loading...')
                  : t('Scheduling defaults are unavailable')}
              </p>
              {!props.loading && (
                <Button
                  type='button'
                  variant='outline'
                  size='sm'
                  onClick={props.onRetry}
                >
                  {t('Retry')}
                </Button>
              )}
            </div>
          )}
        </div>
        <div className='flex shrink-0 flex-wrap items-center justify-between gap-2 border-t p-3'>
          <Button
            type='button'
            variant='ghost'
            size='sm'
            disabled={props.disabled || !overridden}
            onClick={() => {
              form.clearErrors('tuning')
              form.setValue(
                'tuning',
                tuning.chat_prefix_enabled === undefined
                  ? {}
                  : { chat_prefix_enabled: tuning.chat_prefix_enabled },
                { shouldDirty: true }
              )
            }}
          >
            {t('Use global defaults')}
          </Button>
          <Button
            type='button'
            size='sm'
            disabled={
              props.disabled || !props.defaults || !form.formState.isDirty
            }
            onClick={props.onSave}
          >
            {props.saving ? t('Saving...') : t('Save')}
          </Button>
        </div>
      </PopoverContent>
    </Popover>
  )
}
