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
import type { Control } from 'react-hook-form'
import { useTranslation } from 'react-i18next'

import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import {
  Form,
  FormControl,
  FormDescription,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from '@/components/ui/form'
import { Input } from '@/components/ui/input'
import { Separator } from '@/components/ui/separator'
import { Switch } from '@/components/ui/switch'

import { FormDirtyIndicator } from '../components/form-dirty-indicator'
import { FormNavigationGuard } from '../components/form-navigation-guard'
import {
  SettingsForm,
  SettingsFormGrid,
  SettingsSwitchContent,
  SettingsSwitchItem,
} from '../components/settings-form-layout'
import { SettingsPageFormActions } from '../components/settings-page-context'
import { SettingsSection } from '../components/settings-section'
import { useSettingsForm } from '../hooks/use-settings-form'
import { useUpdateDynamicRoutingSettings } from '../hooks/use-update-option'
import type { DynamicRoutingFlatSettings } from '../types'
import { safeNumberFieldProps } from '../utils/numeric-field'
import {
  DYNAMIC_ROUTING_RECOVERY_FIELDS,
  DYNAMIC_ROUTING_SAMPLING_FIELDS,
  DYNAMIC_ROUTING_SENSITIVITY_FIELDS,
  type DynamicRoutingNumericFieldDefinition,
} from './lib/dynamic-routing-fields'
import {
  buildDynamicRoutingFormValues,
  dynamicRoutingFormSchema,
  type DynamicRoutingFormValues,
} from './lib/dynamic-routing-schema'

type DynamicRoutingNumberFieldProps = {
  control: Control<DynamicRoutingFormValues>
  definition: DynamicRoutingNumericFieldDefinition
  disabled: boolean
}

function DynamicRoutingNumberField(props: DynamicRoutingNumberFieldProps) {
  const { t } = useTranslation()

  return (
    <FormField
      control={props.control}
      name={props.definition.name}
      render={({ field }) => (
        <FormItem>
          <FormLabel>{t(props.definition.labelKey)}</FormLabel>
          <FormControl>
            <Input
              type='number'
              min={props.definition.min}
              max={props.definition.max}
              step={props.definition.step}
              disabled={props.disabled}
              {...safeNumberFieldProps(field)}
            />
          </FormControl>
          <FormDescription>
            {t(props.definition.descriptionKey)}
          </FormDescription>
          <FormMessage />
        </FormItem>
      )}
    />
  )
}

type DynamicRoutingFieldGroupProps = {
  title: string
  fields: DynamicRoutingNumericFieldDefinition[]
  control: Control<DynamicRoutingFormValues>
  disabled: boolean
}

function DynamicRoutingFieldGroup(props: DynamicRoutingFieldGroupProps) {
  return (
    <div className='flex min-w-0 flex-col gap-4'>
      <h4 className='text-sm font-medium'>{props.title}</h4>
      <SettingsFormGrid>
        {props.fields.map((definition) => (
          <DynamicRoutingNumberField
            key={definition.name}
            control={props.control}
            definition={definition}
            disabled={props.disabled}
          />
        ))}
      </SettingsFormGrid>
    </div>
  )
}

type DynamicRoutingSectionProps = {
  defaultValues: DynamicRoutingFlatSettings
}

export function DynamicRoutingSection(props: DynamicRoutingSectionProps) {
  const { t } = useTranslation()
  const updateDynamicRouting = useUpdateDynamicRoutingSettings()
  const { form, handleSubmit, handleReset, isDirty, isSubmitting } =
    useSettingsForm<DynamicRoutingFormValues>({
      resolver: zodResolver(dynamicRoutingFormSchema),
      defaultValues: buildDynamicRoutingFormValues(props.defaultValues),
      preserveDirtyValuesOnDefaultChange: true,
      onSubmit: async (values) => {
        await updateDynamicRouting.mutateAsync(values.dynamic_routing_setting)
      },
    })
  const isSaving = updateDynamicRouting.isPending || isSubmitting

  return (
    <SettingsSection title={t('Dynamic Routing')}>
      <FormNavigationGuard when={isDirty} />

      <Form {...form}>
        <SettingsForm onSubmit={handleSubmit}>
          <SettingsPageFormActions
            onSave={handleSubmit}
            onReset={handleReset}
            isSaving={isSaving}
            isResetDisabled={!isDirty}
          />
          <FormDirtyIndicator isDirty={isDirty} />

          <Alert>
            <AlertTitle>{t('Process-local learning')}</AlertTitle>
            <AlertDescription className='space-y-2 text-sm'>
              <p>
                {t(
                  'Observations are keyed by channel and model, and are limited by both sample count and age. Each application process learns independently; this state is not shared through Redis.'
                )}
              </p>
              <p>
                {t(
                  'TTFT and TPOT are the primary signals. Static priority and weight remain the cold-start policy and the fallback while dynamic routing is disabled.'
                )}
              </p>
              <p>
                {t(
                  'Dynamic routing currently applies only to streaming text generation; non-streaming, media, and task routes keep static routing.'
                )}
              </p>
              <p>
                {t(
                  'Severely degraded channels shed traffic quickly, while recovered channels regain traffic gradually to avoid oscillation.'
                )}
              </p>
            </AlertDescription>
          </Alert>

          <FormField
            control={form.control}
            name='dynamic_routing_setting.enabled'
            render={({ field }) => (
              <SettingsSwitchItem>
                <SettingsSwitchContent>
                  <FormLabel>{t('Enable dynamic routing')}</FormLabel>
                  <FormDescription>
                    {t(
                      'Continuously compare recent TTFT and TPOT, probe alternatives, and adjust channel traffic shares.'
                    )}
                  </FormDescription>
                </SettingsSwitchContent>
                <FormControl>
                  <Switch
                    checked={field.value}
                    onCheckedChange={field.onChange}
                    disabled={isSaving}
                  />
                </FormControl>
              </SettingsSwitchItem>
            )}
          />

          <Separator />

          <DynamicRoutingFieldGroup
            title={t('Sampling and window')}
            fields={DYNAMIC_ROUTING_SAMPLING_FIELDS}
            control={form.control}
            disabled={isSaving}
          />

          <Separator />

          <DynamicRoutingFieldGroup
            title={t('Switching sensitivity')}
            fields={DYNAMIC_ROUTING_SENSITIVITY_FIELDS}
            control={form.control}
            disabled={isSaving}
          />

          <Separator />

          <DynamicRoutingFieldGroup
            title={t('Recovery and hard failures')}
            fields={DYNAMIC_ROUTING_RECOVERY_FIELDS}
            control={form.control}
            disabled={isSaving}
          />
        </SettingsForm>
      </Form>
    </SettingsSection>
  )
}
