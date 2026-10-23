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

const createBalancePatrolSchema = (t: (key: string) => string) =>
  z.object({
    balance_patrol_setting: z.object({
      channel_enabled: z.boolean(),
      channel_minutes: z.coerce
        .number()
        .min(1, t('Patrol interval must be between 1 and 10080 minutes'))
        .max(10080, t('Patrol interval must be between 1 and 10080 minutes')),
      low_balance_threshold: z.coerce
        .number()
        .min(0, t('Threshold must be a non-negative number')),
      disable_on_zero_balance: z.boolean(),
      user_enabled: z.boolean(),
      user_minutes: z.coerce
        .number()
        .min(1, t('Patrol interval must be between 1 and 10080 minutes'))
        .max(10080, t('Patrol interval must be between 1 and 10080 minutes')),
      user_min_quota: z.coerce
        .number()
        .min(0, t('User threshold must be a non-negative integer')),
      user_list_limit: z.coerce
        .number()
        .min(0, t('List limit must be between 0 and 50'))
        .max(50, t('List limit must be between 0 and 50')),
    }),
  })

type BalancePatrolFormInput = z.input<ReturnType<typeof createBalancePatrolSchema>>
type BalancePatrolFormValues = z.output<ReturnType<typeof createBalancePatrolSchema>>

type FlatBalancePatrolDefaults = {
  'balance_patrol_setting.channel_enabled': boolean
  'balance_patrol_setting.channel_minutes': number
  'balance_patrol_setting.low_balance_threshold': number
  'balance_patrol_setting.disable_on_zero_balance': boolean
  'balance_patrol_setting.user_enabled': boolean
  'balance_patrol_setting.user_minutes': number
  'balance_patrol_setting.user_min_quota': number
  'balance_patrol_setting.user_list_limit': number
}

type BalancePatrolSettingsSectionProps = {
  defaultValues: FlatBalancePatrolDefaults
}

const buildFormDefaults = (
  defaults: BalancePatrolSettingsSectionProps['defaultValues']
): BalancePatrolFormInput => ({
  balance_patrol_setting: {
    channel_enabled: defaults['balance_patrol_setting.channel_enabled'],
    channel_minutes: defaults['balance_patrol_setting.channel_minutes'],
    low_balance_threshold:
      defaults['balance_patrol_setting.low_balance_threshold'],
    disable_on_zero_balance:
      defaults['balance_patrol_setting.disable_on_zero_balance'],
    user_enabled: defaults['balance_patrol_setting.user_enabled'],
    user_minutes: defaults['balance_patrol_setting.user_minutes'],
    user_min_quota: defaults['balance_patrol_setting.user_min_quota'],
    user_list_limit: defaults['balance_patrol_setting.user_list_limit'],
  },
})

const normalizeDefaults = (
  defaults: BalancePatrolSettingsSectionProps['defaultValues']
): FlatBalancePatrolDefaults => ({
  'balance_patrol_setting.channel_enabled':
    defaults['balance_patrol_setting.channel_enabled'],
  'balance_patrol_setting.channel_minutes':
    defaults['balance_patrol_setting.channel_minutes'],
  'balance_patrol_setting.low_balance_threshold':
    defaults['balance_patrol_setting.low_balance_threshold'],
  'balance_patrol_setting.disable_on_zero_balance':
    defaults['balance_patrol_setting.disable_on_zero_balance'],
  'balance_patrol_setting.user_enabled':
    defaults['balance_patrol_setting.user_enabled'],
  'balance_patrol_setting.user_minutes':
    defaults['balance_patrol_setting.user_minutes'],
  'balance_patrol_setting.user_min_quota':
    defaults['balance_patrol_setting.user_min_quota'],
  'balance_patrol_setting.user_list_limit':
    defaults['balance_patrol_setting.user_list_limit'],
})

const normalizeFormValues = (
  values: BalancePatrolFormValues
): FlatBalancePatrolDefaults => ({
  'balance_patrol_setting.channel_enabled':
    values.balance_patrol_setting.channel_enabled,
  'balance_patrol_setting.channel_minutes':
    values.balance_patrol_setting.channel_minutes,
  'balance_patrol_setting.low_balance_threshold':
    values.balance_patrol_setting.low_balance_threshold,
  'balance_patrol_setting.disable_on_zero_balance':
    values.balance_patrol_setting.disable_on_zero_balance,
  'balance_patrol_setting.user_enabled':
    values.balance_patrol_setting.user_enabled,
  'balance_patrol_setting.user_minutes':
    values.balance_patrol_setting.user_minutes,
  'balance_patrol_setting.user_min_quota':
    values.balance_patrol_setting.user_min_quota,
  'balance_patrol_setting.user_list_limit':
    values.balance_patrol_setting.user_list_limit,
})

export function BalancePatrolSettingsSection({
  defaultValues,
}: BalancePatrolSettingsSectionProps) {
  const { t } = useTranslation()
  const updateOption = useUpdateOption()
  const balancePatrolSchema = createBalancePatrolSchema(t)
  const baselineRef = useRef<FlatBalancePatrolDefaults>(
    normalizeDefaults(defaultValues)
  )
  const baselineSerializedRef = useRef<string>(
    JSON.stringify(normalizeDefaults(defaultValues))
  )

  const formDefaults = useMemo(
    () => buildFormDefaults(defaultValues),
    [defaultValues]
  )

  const form = useForm<BalancePatrolFormInput, unknown, BalancePatrolFormValues>({
    resolver: zodResolver(balancePatrolSchema),
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

  const onSubmit = async (values: BalancePatrolFormValues) => {
    const normalized = normalizeFormValues(values)
    const updates = (
      Object.keys(normalized) as Array<keyof FlatBalancePatrolDefaults>
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
    <SettingsSection title={t('Balance Patrol')}>
      <Form {...form}>
        <SettingsForm onSubmit={form.handleSubmit(onSubmit)}>
          <SettingsPageFormActions
            onSave={form.handleSubmit(onSubmit)}
            isSaving={updateOption.isPending}
            saveLabel='Save balance patrol settings'
          />
          <div>
            <h4 className='font-medium'>{t('Channel balance patrol')}</h4>
            <p className='text-muted-foreground mt-1 text-xs'>
              {t(
                'Periodically query upstream channel balances and keep them updated'
              )}
            </p>
          </div>
          <div className='grid grid-cols-1 gap-4 md:grid-cols-2'>
            <FormField
              control={form.control}
              name='balance_patrol_setting.channel_enabled'
              render={({ field }) => (
                <SettingsSwitchItem>
                  <SettingsSwitchContent>
                    <FormLabel>
                      {t('Enable channel balance patrol')}
                    </FormLabel>
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
              name='balance_patrol_setting.disable_on_zero_balance'
              render={({ field }) => (
                <SettingsSwitchItem>
                  <SettingsSwitchContent>
                    <FormLabel>
                      {t('Disable channels at zero balance')}
                    </FormLabel>
                    <FormDescription>
                      {t(
                        'Automatically disable a channel when its upstream balance is used up'
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
            <FormField
              control={form.control}
              name='balance_patrol_setting.channel_minutes'
              render={({ field }) => (
                <FormItem>
                  <FormLabel>
                    {t('Channel patrol interval (minutes)')}
                  </FormLabel>
                  <FormControl>
                    <Input
                      type='number'
                      min={1}
                      step={1}
                      {...safeNumberFieldProps(field)}
                    />
                  </FormControl>
                  <FormMessage />
                </FormItem>
              )}
            />
            <FormField
              control={form.control}
              name='balance_patrol_setting.low_balance_threshold'
              render={({ field }) => (
                <FormItem>
                  <FormLabel>
                    {t('Low balance alert threshold (USD)')}
                  </FormLabel>
                  <FormControl>
                    <Input
                      type='number'
                      min={0}
                      step={0.01}
                      {...safeNumberFieldProps(field)}
                    />
                  </FormControl>
                  <FormDescription>
                    {t(
                      'Alert the WeCom bot when a channel balance drops below this value; 0 disables the alert'
                    )}
                  </FormDescription>
                  <FormMessage />
                </FormItem>
              )}
            />
          </div>

          <div>
            <h4 className='font-medium'>{t('User quota patrol')}</h4>
            <p className='text-muted-foreground mt-1 text-xs'>
              {t(
                'Periodically check user balances and send an aggregated low-balance alert'
              )}
            </p>
          </div>
          <div className='grid grid-cols-1 gap-4 md:grid-cols-2'>
            <FormField
              control={form.control}
              name='balance_patrol_setting.user_enabled'
              render={({ field }) => (
                <SettingsSwitchItem>
                  <SettingsSwitchContent>
                    <FormLabel>{t('Enable user quota patrol')}</FormLabel>
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
              name='balance_patrol_setting.user_minutes'
              render={({ field }) => (
                <FormItem>
                  <FormLabel>{t('User patrol interval (minutes)')}</FormLabel>
                  <FormControl>
                    <Input
                      type='number'
                      min={1}
                      step={1}
                      {...safeNumberFieldProps(field)}
                    />
                  </FormControl>
                  <FormMessage />
                </FormItem>
              )}
            />
            <FormField
              control={form.control}
              name='balance_patrol_setting.user_min_quota'
              render={({ field }) => (
                <FormItem>
                  <FormLabel>
                    {t('User low-balance threshold (quota)')}
                  </FormLabel>
                  <FormControl>
                    <Input
                      type='number'
                      min={0}
                      step={1}
                      {...safeNumberFieldProps(field)}
                    />
                  </FormControl>
                  <FormDescription>
                    {t(
                      'Users below this quota are reported; 0 follows the global quota reminder threshold'
                    )}
                  </FormDescription>
                  <FormMessage />
                </FormItem>
              )}
            />
            <FormField
              control={form.control}
              name='balance_patrol_setting.user_list_limit'
              render={({ field }) => (
                <FormItem>
                  <FormLabel>{t('Max users listed per alert')}</FormLabel>
                  <FormControl>
                    <Input
                      type='number'
                      min={0}
                      max={50}
                      step={1}
                      {...safeNumberFieldProps(field)}
                    />
                  </FormControl>
                  <FormDescription>
                    {t(
                      'Only the lowest-balance users are listed in one aggregated alert'
                    )}
                  </FormDescription>
                  <FormMessage />
                </FormItem>
              )}
            />
          </div>
        </SettingsForm>
      </Form>
    </SettingsSection>
  )
}
