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
import { useForm } from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import { z } from 'zod'

import { FieldGroup } from '@/components/ui/field'
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

import { SettingsForm } from '../components/settings-form-layout'
import { SettingsPageFormActions } from '../components/settings-page-context'
import { SettingsSection } from '../components/settings-section'
import { useResetForm } from '../hooks/use-reset-form'
import { useUpdateOption } from '../hooks/use-update-option'

const schema = z.object({ quotaMB: z.number().int().min(0).max(1_000_000) })

export function AssetStorageSection({ quotaMB }: { quotaMB: number }) {
  const { t } = useTranslation()
  const updateOption = useUpdateOption()
  const form = useForm<z.infer<typeof schema>>({
    resolver: zodResolver(schema),
    defaultValues: { quotaMB },
  })
  useResetForm(form, { quotaMB })
  const onSave = form.handleSubmit(async (values) => {
    const result = await updateOption.mutateAsync({
      key: 'asset_storage_setting.default_quota_mb',
      value: String(values.quotaMB),
    })
    if (result.success) form.reset(values)
  })

  return (
    <SettingsSection title={t('Asset storage')}>
      <Form {...form}>
        <SettingsForm onSubmit={onSave}>
          <SettingsPageFormActions
            onSave={onSave}
            isSaving={form.formState.isSubmitting}
            isSaveDisabled={!form.formState.isDirty}
          />
          <FieldGroup>
            <FormField
              control={form.control}
              name='quotaMB'
              render={({ field }) => (
                <FormItem>
                  <FormLabel>{t('Default free asset quota (MB)')}</FormLabel>
                  <FormControl>
                    <Input
                      {...field}
                      type='number'
                      min={0}
                      max={1_000_000}
                      step={1}
                      onChange={(event) =>
                        field.onChange(event.target.valueAsNumber)
                      }
                      disabled={form.formState.isSubmitting}
                    />
                  </FormControl>
                  <FormDescription>
                    <span className='block'>
                      {t(
                        'Applies to each user. 1 MB = 1,000,000 bytes. Reusing identical content does not consume additional quota.'
                      )}
                    </span>
                    <span className='block'>
                      {t(
                        'Reducing the quota keeps existing assets. A quota of 0 prevents new storage usage.'
                      )}
                    </span>
                  </FormDescription>
                  <FormMessage />
                </FormItem>
              )}
            />
          </FieldGroup>
        </SettingsForm>
      </Form>
    </SettingsSection>
  )
}
