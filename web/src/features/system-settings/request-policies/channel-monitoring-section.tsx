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
import { useMemo } from 'react'
import { useForm } from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import { z } from 'zod'

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
import { Textarea } from '@/components/ui/textarea'
import { toIntlLocale } from '@/i18n/languages'
import { formatNumber } from '@/lib/format'
import { handleServerError } from '@/lib/handle-server-error'
import { parseHttpStatusCodeRules } from '@/lib/http-status-code-rules'

import {
  SettingsForm,
  SettingsFormGrid,
} from '../components/settings-form-layout'
import { SettingsPageFormActions } from '../components/settings-page-context'
import { SettingsSection } from '../components/settings-section'
import { useResetForm } from '../hooks/use-reset-form'
import type { MonitoringSettings } from './defaults'
import { useSavePolicy } from './use-save-policy'

const fields = [
  { name: 'minutes', label: 'Probe interval (minutes)', min: 1, max: 1440 },
  {
    name: 'timeout_seconds',
    label: 'Probe timeout (seconds)',
    min: 1,
    max: 300,
  },
  {
    name: 'max_tokens',
    label: 'Maximum probe output tokens',
    min: 1,
    max: 512,
  },
  {
    name: 'slow_ms',
    label: 'Slow response threshold (ms)',
    min: 1,
    max: 300000,
  },
] as const
const createMonitoringSchema = (
  t: (key: string, options?: Record<string, unknown>) => string
) =>
  z.object({
    enabled: z.boolean(),
    stream: z.boolean(),
    excluded_status_codes: z
      .string()
      .max(1024)
      .superRefine((value, context) => {
        const parsed = parseHttpStatusCodeRules(value)
        if (!parsed.ok) {
          context.addIssue({
            code: 'custom',
            message: t('Invalid status code rules: {{tokens}}', {
              tokens: parsed.invalidTokens.join(', '),
            }),
          })
        }
      }),
    prompt: z
      .string()
      .trim()
      .min(1, t('Probe prompt is required'))
      .refine(
        (value) => new TextEncoder().encode(value).length <= 4096,
        t('Probe prompt is too long')
      ),
    minutes: z
      .number()
      .int(t('Enter an integer'))
      .min(1, t('Probe request limits are invalid'))
      .max(1440, t('Probe request limits are invalid')),
    timeout_seconds: z
      .number()
      .int(t('Enter an integer'))
      .min(1, t('Probe request limits are invalid'))
      .max(300, t('Probe request limits are invalid')),
    max_tokens: z
      .number()
      .int(t('Enter an integer'))
      .min(1, t('Probe request limits are invalid'))
      .max(512, t('Probe request limits are invalid')),
    slow_ms: z
      .number()
      .int(t('Enter an integer'))
      .min(1, t('Probe request limits are invalid'))
      .max(300000, t('Probe request limits are invalid')),
  })
type Values = z.infer<ReturnType<typeof createMonitoringSchema>>

export function ChannelMonitoringSection(props: {
  defaultValues: MonitoringSettings
}) {
  const { t, i18n } = useTranslation()
  const locale = toIntlLocale(i18n.resolvedLanguage || i18n.language)
  const schema = useMemo(() => createMonitoringSchema(t), [t])
  const save = useSavePolicy()
  const defaults = useMemo(
    () => ({
      enabled:
        props.defaultValues['monitor_setting.channel_monitoring_enabled'] ??
        true,
      minutes:
        props.defaultValues['monitor_setting.channel_monitoring_minutes'] ?? 5,
      timeout_seconds:
        props.defaultValues[
          'monitor_setting.channel_monitoring_timeout_seconds'
        ] ?? 30,
      prompt:
        props.defaultValues['monitor_setting.channel_monitoring_prompt'] ??
        'Reply with OK.',
      max_tokens:
        props.defaultValues['monitor_setting.channel_monitoring_max_tokens'] ??
        128,
      stream:
        props.defaultValues['monitor_setting.channel_monitoring_stream'] ??
        true,
      slow_ms:
        props.defaultValues['monitor_setting.channel_monitoring_slow_ms'] ??
        3000,
      excluded_status_codes:
        props.defaultValues[
          'monitor_setting.channel_monitoring_excluded_status_codes'
        ] ?? '400',
    }),
    [props.defaultValues]
  )
  const form = useForm<Values>({
    resolver: zodResolver(schema),
    defaultValues: defaults,
  })
  useResetForm(form, defaults)
  async function submit(values: Values) {
    const options = Object.fromEntries(
      Object.entries(values).map(([key, value]) => [
        `monitor_setting.channel_monitoring_${key}`,
        String(value),
      ])
    )
    try {
      await save.mutateAsync(options)
      form.reset(values)
    } catch (error) {
      handleServerError(error)
    }
  }
  return (
    <SettingsSection title={t('Channel monitoring')}>
      <p className='text-muted-foreground text-sm'>
        {t(
          'Registered channels are probed in the background. Monitoring does not change channel routing or automatic disable policies.'
        )}
      </p>
      <Form {...form}>
        <SettingsForm onSubmit={form.handleSubmit(submit)}>
          <SettingsPageFormActions
            onSave={form.handleSubmit(submit)}
            isSaving={form.formState.isSubmitting}
          />
          <SettingsFormGrid>
            {(['enabled', 'stream'] as const).map((name) => (
              <FormField
                key={name}
                control={form.control}
                name={name}
                render={({ field }) => (
                  <FormItem className='flex items-center justify-between gap-4'>
                    <FormLabel>
                      {t(
                        name === 'enabled'
                          ? 'Enable registered channel monitoring'
                          : 'Use streaming probes'
                      )}
                    </FormLabel>
                    <FormControl>
                      <Switch
                        checked={field.value}
                        onCheckedChange={field.onChange}
                        disabled={save.isPending}
                      />
                    </FormControl>
                  </FormItem>
                )}
              />
            ))}
            {fields.map((spec) => (
              <FormField
                key={spec.name}
                control={form.control}
                name={spec.name}
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>{t(spec.label)}</FormLabel>
                    <FormControl>
                      <Input
                        type='number'
                        min={spec.min}
                        max={spec.max}
                        step={1}
                        value={Number.isFinite(field.value) ? field.value : ''}
                        onBlur={field.onBlur}
                        onChange={(event) =>
                          field.onChange(event.target.valueAsNumber)
                        }
                        name={field.name}
                        ref={field.ref}
                        disabled={save.isPending}
                      />
                    </FormControl>
                    <FormDescription>
                      {t('Allowed range: {{min}}–{{max}}', {
                        min: formatNumber(spec.min, locale),
                        max: formatNumber(spec.max, locale),
                      })}
                    </FormDescription>
                    <FormMessage />
                  </FormItem>
                )}
              />
            ))}
            <FormField
              control={form.control}
              name='excluded_status_codes'
              render={({ field }) => (
                <FormItem>
                  <FormLabel>
                    {t('Status codes excluded from monitoring statistics')}
                  </FormLabel>
                  <FormControl>
                    <Input
                      {...field}
                      maxLength={1024}
                      placeholder='400,422-429'
                      disabled={save.isPending}
                    />
                  </FormControl>
                  <FormDescription>
                    {t(
                      'Matching responses remain visible but do not affect availability or performance. Use codes or ranges separated by commas. Leave empty to include all status codes.'
                    )}
                  </FormDescription>
                  <FormMessage />
                </FormItem>
              )}
            />
            <FormField
              control={form.control}
              name='prompt'
              render={({ field }) => (
                <FormItem>
                  <FormLabel>{t('Probe prompt')}</FormLabel>
                  <FormControl>
                    <Textarea
                      rows={4}
                      maxLength={4096}
                      {...field}
                      disabled={save.isPending}
                    />
                  </FormControl>
                  <FormDescription>
                    {t(
                      'The same short prompt is converted to each target protocol. Channel configuration selects which models and protocols are monitored.'
                    )}
                  </FormDescription>
                  <FormMessage />
                </FormItem>
              )}
            />
          </SettingsFormGrid>
        </SettingsForm>
      </Form>
    </SettingsSection>
  )
}
