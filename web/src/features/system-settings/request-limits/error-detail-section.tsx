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
import { useEffect } from 'react'
import { useForm } from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import * as z from 'zod'

import {
  Form,
  FormControl,
  FormDescription,
  FormField,
  FormLabel,
} from '@/components/ui/form'
import { Switch } from '@/components/ui/switch'

import {
  SettingsForm,
  SettingsSwitchContent,
  SettingsSwitchItem,
} from '../components/settings-form-layout'
import { SettingsPageFormActions } from '../components/settings-page-context'
import { SettingsSection } from '../components/settings-section'
import { useUpdateOption } from '../hooks/use-update-option'

const errorDetailSchema = z.object({
  error_setting: z.object({
    hide_error_details: z.boolean(),
  }),
})

type ErrorDetailFormValues = z.infer<typeof errorDetailSchema>

type ErrorDetailSectionProps = {
  defaultValues: {
    'error_setting.hide_error_details': boolean
  }
}

const buildFormDefaults = (
  defaults: ErrorDetailSectionProps['defaultValues']
): ErrorDetailFormValues => ({
  error_setting: {
    hide_error_details: defaults['error_setting.hide_error_details'],
  },
})

export function ErrorDetailSection({ defaultValues }: ErrorDetailSectionProps) {
  const { t } = useTranslation()
  const updateOption = useUpdateOption()
  const form = useForm<ErrorDetailFormValues>({
    resolver: zodResolver(errorDetailSchema),
    defaultValues: buildFormDefaults(defaultValues),
  })

  useEffect(() => {
    form.reset(buildFormDefaults(defaultValues))
  }, [defaultValues, form])

  const onSubmit = async (values: ErrorDetailFormValues) => {
    const value = values.error_setting.hide_error_details
    if (value !== defaultValues['error_setting.hide_error_details']) {
      await updateOption.mutateAsync({
        key: 'error_setting.hide_error_details',
        value,
      })
    }
  }

  return (
    <SettingsSection title={t('Error Details')}>
      <Form {...form}>
        <SettingsForm onSubmit={form.handleSubmit(onSubmit)}>
          <SettingsPageFormActions
            onSave={form.handleSubmit(onSubmit)}
            isSaving={updateOption.isPending}
            saveLabel='Save error detail settings'
          />
          <FormField
            control={form.control}
            name='error_setting.hide_error_details'
            render={({ field }) => (
              <SettingsSwitchItem>
                <SettingsSwitchContent>
                  <FormLabel>{t('Hide error details from users')}</FormLabel>
                  <FormDescription>
                    {t(
                      'When enabled, users see a generic retry message and request ID, while administrators see the full error details.'
                    )}
                  </FormDescription>
                </SettingsSwitchContent>
                <FormControl>
                  <Switch
                    checked={field.value}
                    onCheckedChange={field.onChange}
                  />
                </FormControl>
              </SettingsSwitchItem>
            )}
          />
        </SettingsForm>
      </Form>
    </SettingsSection>
  )
}
