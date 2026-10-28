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
import { useEffect, useMemo, useRef } from 'react'
import { useForm } from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import * as z from 'zod'

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
import { Switch } from '@/components/ui/switch'

import {
  SettingsForm,
  SettingsSwitchContent,
  SettingsSwitchItem,
} from '../components/settings-form-layout'
import { SettingsPageFormActions } from '../components/settings-page-context'
import { SettingsSection } from '../components/settings-section'
import { useResetForm } from '../hooks/use-reset-form'
import { useUpdateOption } from '../hooks/use-update-option'
import { safeNumberFieldProps } from '../utils/numeric-field'
import { WecomBotList } from './wecom-bot-list'

const createWecomBotSchema = (t: (key: string) => string) =>
  z.object({
    wecom_bot_setting: z.object({
      enabled: z.boolean(),
      error_notify_enabled: z.boolean(),
      error_cooldown_minutes: z.coerce
        .number()
        .min(1, t('Cooldown must be between 1 and 1440 minutes'))
        .max(1440, t('Cooldown must be between 1 and 1440 minutes')),
      disable_notify_enabled: z.boolean(),
      enable_notify_enabled: z.boolean(),
    }),
  })

type WecomBotFormInput = z.input<ReturnType<typeof createWecomBotSchema>>
type WecomBotFormValues = z.output<ReturnType<typeof createWecomBotSchema>>

type FlatWecomBotDefaults = {
  'wecom_bot_setting.enabled': boolean
  'wecom_bot_setting.error_notify_enabled': boolean
  'wecom_bot_setting.error_cooldown_minutes': number
  'wecom_bot_setting.disable_notify_enabled': boolean
  'wecom_bot_setting.enable_notify_enabled': boolean
}

type WecomBotSettingsSectionProps = {
  defaultValues: FlatWecomBotDefaults
}

const buildFormDefaults = (
  defaults: WecomBotSettingsSectionProps['defaultValues']
): WecomBotFormInput => ({
  wecom_bot_setting: {
    enabled: defaults['wecom_bot_setting.enabled'],
    error_notify_enabled:
      defaults['wecom_bot_setting.error_notify_enabled'],
    error_cooldown_minutes:
      defaults['wecom_bot_setting.error_cooldown_minutes'],
    disable_notify_enabled:
      defaults['wecom_bot_setting.disable_notify_enabled'],
    enable_notify_enabled: defaults['wecom_bot_setting.enable_notify_enabled'],
  },
})

const normalizeDefaults = (
  defaults: WecomBotSettingsSectionProps['defaultValues']
): FlatWecomBotDefaults => ({
  'wecom_bot_setting.enabled': defaults['wecom_bot_setting.enabled'],
  'wecom_bot_setting.error_notify_enabled':
    defaults['wecom_bot_setting.error_notify_enabled'],
  'wecom_bot_setting.error_cooldown_minutes':
    defaults['wecom_bot_setting.error_cooldown_minutes'],
  'wecom_bot_setting.disable_notify_enabled':
    defaults['wecom_bot_setting.disable_notify_enabled'],
  'wecom_bot_setting.enable_notify_enabled':
    defaults['wecom_bot_setting.enable_notify_enabled'],
})

const normalizeFormValues = (
  values: WecomBotFormValues
): FlatWecomBotDefaults => ({
  'wecom_bot_setting.enabled': values.wecom_bot_setting.enabled,
  'wecom_bot_setting.error_notify_enabled':
    values.wecom_bot_setting.error_notify_enabled,
  'wecom_bot_setting.error_cooldown_minutes':
    values.wecom_bot_setting.error_cooldown_minutes,
  'wecom_bot_setting.disable_notify_enabled':
    values.wecom_bot_setting.disable_notify_enabled,
  'wecom_bot_setting.enable_notify_enabled':
    values.wecom_bot_setting.enable_notify_enabled,
})

export function WecomBotSettingsSection({
  defaultValues,
}: WecomBotSettingsSectionProps) {
  const { t } = useTranslation()
  const updateOption = useUpdateOption()
  const wecomBotSchema = createWecomBotSchema(t)
  const baselineRef = useRef<FlatWecomBotDefaults>(
    normalizeDefaults(defaultValues)
  )
  const baselineSerializedRef = useRef<string>(
    JSON.stringify(normalizeDefaults(defaultValues))
  )

  const formDefaults = useMemo(
    () => buildFormDefaults(defaultValues),
    [defaultValues]
  )

  const form = useForm<WecomBotFormInput, unknown, WecomBotFormValues>({
    resolver: zodResolver(wecomBotSchema),
    defaultValues: formDefaults,
  })

  useResetForm(form, formDefaults)

  useEffect(() => {
    const normalized = normalizeDefaults(defaultValues)
    const serialized = JSON.stringify(normalized)
    if (serialized === baselineSerializedRef.current) return
    baselineRef.current = normalized
    baselineSerializedRef.current = serialized
  }, [defaultValues])

  const onSubmit = async (values: WecomBotFormValues) => {
    const normalized = normalizeFormValues(values)
    const updates = (
      Object.keys(normalized) as Array<keyof FlatWecomBotDefaults>
    ).filter((key) => normalized[key] !== baselineRef.current[key])

    if (updates.length === 0) {
      toast.info(t('No changes to save'))
      return
    }

    for (const key of updates) {
      await updateOption.mutateAsync({
        key,
        value: normalized[key],
      })
    }

    baselineRef.current = normalized
    baselineSerializedRef.current = JSON.stringify(normalized)
  }

  return (
    <SettingsSection title={t('WeCom Channel Alert Bot')}>
      <Form {...form}>
        <SettingsForm onSubmit={form.handleSubmit(onSubmit)}>
          <SettingsPageFormActions
            onSave={form.handleSubmit(onSubmit)}
            isSaving={updateOption.isPending}
            saveLabel='Save WeCom bot settings'
          />
          <FormField
            control={form.control}
            name='wecom_bot_setting.enabled'
            render={({ field }) => (
              <SettingsSwitchItem>
                <SettingsSwitchContent>
                  <FormLabel>{t('Enable WeCom bot alerts')}</FormLabel>
                  <FormDescription>
                    {t(
                      'Send channel error and status alerts to a WeCom group robot'
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

          <div className='grid grid-cols-1 gap-4 md:grid-cols-2'>
            <FormField
              control={form.control}
              name='wecom_bot_setting.error_cooldown_minutes'
              render={({ field }) => (
                <FormItem>
                  <FormLabel>{t('Error cooldown (minutes)')}</FormLabel>
                  <FormControl>
                    <Input
                      type='number'
                      min={1}
                      max={1440}
                      step={1}
                      {...safeNumberFieldProps(field)}
                    />
                  </FormControl>
                  <FormDescription>
                    {t(
                      'The same channel and status code only alerts once within this window'
                    )}
                  </FormDescription>
                  <FormMessage />
                </FormItem>
              )}
            />
            <FormField
              control={form.control}
              name='wecom_bot_setting.error_notify_enabled'
              render={({ field }) => (
                <SettingsSwitchItem>
                  <SettingsSwitchContent>
                    <FormLabel>{t('Channel error alerts')}</FormLabel>
                    <FormDescription>
                      {t('Push real-time channel relay errors to the group')}
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
            <FormField
              control={form.control}
              name='wecom_bot_setting.disable_notify_enabled'
              render={({ field }) => (
                <SettingsSwitchItem>
                  <SettingsSwitchContent>
                    <FormLabel>{t('Auto-disable alerts')}</FormLabel>
                    <FormDescription>
                      {t('Notify when a channel is automatically disabled')}
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
            <FormField
              control={form.control}
              name='wecom_bot_setting.enable_notify_enabled'
              render={({ field }) => (
                <SettingsSwitchItem>
                  <SettingsSwitchContent>
                    <FormLabel>{t('Recovery alerts')}</FormLabel>
                    <FormDescription>
                      {t('Notify when a disabled channel is re-enabled')}
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
          </div>
        </SettingsForm>
      </Form>

      <WecomBotList />
    </SettingsSection>
  )
}
