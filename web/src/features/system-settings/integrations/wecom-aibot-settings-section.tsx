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
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useMemo } from 'react'
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
import { Textarea } from '@/components/ui/textarea'
import { handleServerError } from '@/lib/handle-server-error'

import {
  getWecomAIBotConfig,
  getWecomAIBotStatus,
  updateWecomAIBotConfig,
} from '../api'
import {
  SettingsForm,
  SettingsSwitchContent,
  SettingsSwitchItem,
} from '../components/settings-form-layout'
import { SettingsPageFormActions } from '../components/settings-page-context'
import { SettingsSection } from '../components/settings-section'
import { useResetForm } from '../hooks/use-reset-form'
import { safeNumberFieldProps } from '../utils/numeric-field'

const wecomAIBotStateLabelKeys: Record<string, string> = {
  disabled: 'Disabled',
  standby: 'Standby (another instance holds the connection)',
  connecting: 'Connecting',
  connected: 'Connected',
  reconnecting: 'Reconnecting',
  kicked: 'Connection taken over by another client',
  auth_failed: 'Authentication failed',
}

const createWecomAIBotSchema = (
  t: (key: string) => string,
  hasStoredSecret: boolean
) =>
  z
    .object({
      enabled: z.boolean(),
      bot_id: z.string(),
      secret: z.string(),
      channel_id: z.coerce
        .number()
        .min(0, t('Channel ID must be a non-negative integer')),
      model: z.string(),
      max_tool_rounds: z.coerce
        .number()
        .min(1, t('Max tool rounds must be between 1 and 10'))
        .max(10, t('Max tool rounds must be between 1 and 10')),
      session_rounds: z.coerce
        .number()
        .min(1, t('Session rounds must be between 1 and 50'))
        .max(50, t('Session rounds must be between 1 and 50')),
      session_ttl_minutes: z.coerce
        .number()
        .min(1, t('Session TTL must be between 1 and 1440 minutes'))
        .max(1440, t('Session TTL must be between 1 and 1440 minutes')),
      allow_all_users: z.boolean(),
      user_whitelist: z.string(),
      group_enabled: z.boolean(),
      welcome_text: z.string(),
      admin_userids: z.string(),
      report_enabled: z.boolean(),
      report_targets: z.string(),
      report_hour: z.coerce
        .number()
        .min(0, t('Report hour must be between 0 and 23'))
        .max(23, t('Report hour must be between 0 and 23')),
    })
    .superRefine((values, ctx) => {
      if (!values.enabled) return
      const requireFilled = (path: 'bot_id' | 'model', message: string) => {
        if (!values[path].trim()) {
          ctx.addIssue({ code: 'custom', path: [path], message })
        }
      }
      requireFilled('bot_id', t('BotID is required when the bot is enabled'))
      requireFilled(
        'model',
        t('Model name is required when the bot is enabled')
      )
      if (values.channel_id <= 0) {
        ctx.addIssue({
          code: 'custom',
          path: ['channel_id'],
          message: t('A valid channel ID is required when the bot is enabled'),
        })
      }
      if (!values.secret.trim() && !hasStoredSecret) {
        ctx.addIssue({
          code: 'custom',
          path: ['secret'],
          message: t('Secret is required when the bot is enabled'),
        })
      }
    })

type WecomAIBotFormInput = z.input<ReturnType<typeof createWecomAIBotSchema>>
type WecomAIBotFormValues = z.output<ReturnType<typeof createWecomAIBotSchema>>

export function WecomAIBotSettingsSection() {
  const { t } = useTranslation()
  const queryClient = useQueryClient()

  const configQuery = useQuery({
    queryKey: ['wecom-aibot-config'],
    queryFn: getWecomAIBotConfig,
  })
  const statusQuery = useQuery({
    queryKey: ['wecom-aibot-status'],
    queryFn: getWecomAIBotStatus,
    refetchInterval: 15000,
  })

  const config = configQuery.data?.data
  const hasStoredSecret = Boolean(config?.secret_masked)

  const schema = useMemo(
    () => createWecomAIBotSchema(t, hasStoredSecret),
    [t, hasStoredSecret]
  )

  const formDefaults = useMemo(
    () => ({
      enabled: config?.enabled ?? false,
      bot_id: config?.bot_id ?? '',
      secret: '',
      channel_id: config?.channel_id ?? 0,
      model: config?.model ?? '',
      max_tool_rounds: config?.max_tool_rounds ?? 5,
      session_rounds: config?.session_rounds ?? 10,
      session_ttl_minutes: config?.session_ttl_minutes ?? 10,
      allow_all_users: config?.allow_all_users ?? false,
      user_whitelist: config?.user_whitelist ?? '',
      group_enabled: config?.group_enabled ?? true,
      welcome_text: config?.welcome_text ?? '',
      admin_userids: config?.admin_userids ?? '',
      report_enabled: config?.report_enabled ?? false,
      report_hour: config?.report_hour ?? 9,
      report_targets: config?.report_targets ?? '',
    }),
    [config]
  )

  const form = useForm<WecomAIBotFormInput, unknown, WecomAIBotFormValues>({
    resolver: zodResolver(schema),
    defaultValues: formDefaults,
  })

  useResetForm(form, formDefaults)

  const saveMutation = useMutation({
    mutationFn: (values: WecomAIBotFormValues) =>
      updateWecomAIBotConfig({
        enabled: values.enabled,
        bot_id: values.bot_id.trim(),
        secret: values.secret.trim(),
        channel_id: values.channel_id,
        model: values.model.trim(),
        max_tool_rounds: values.max_tool_rounds,
        session_rounds: values.session_rounds,
        session_ttl_minutes: values.session_ttl_minutes,
        allow_all_users: values.allow_all_users,
        user_whitelist: values.user_whitelist.trim(),
        group_enabled: values.group_enabled,
        welcome_text: values.welcome_text,
        admin_userids: values.admin_userids.trim(),
        report_enabled: values.report_enabled,
        report_hour: values.report_hour,
        report_targets: values.report_targets.trim(),
      }),
    onSuccess: () => {
      toast.success(t('Setting updated successfully'))
      form.reset({ ...form.getValues(), secret: '' })
      void queryClient.invalidateQueries({
        queryKey: ['wecom-aibot-config'],
      })
      void queryClient.invalidateQueries({
        queryKey: ['wecom-aibot-status'],
      })
    },
    onError: (error: Error) =>
      handleServerError(error, t('Failed to update setting')),
  })

  const status = statusQuery.data?.data
  const statusLabel = status
    ? t(wecomAIBotStateLabelKeys[status.state] ?? status.state)
    : '—'

  const onSubmit = (values: WecomAIBotFormValues) => saveMutation.mutate(values)

  return (
    <SettingsSection title={t('WeCom AI Bot')}>
      <Form {...form}>
        <SettingsForm onSubmit={form.handleSubmit(onSubmit)}>
          <SettingsPageFormActions
            onSave={form.handleSubmit(onSubmit)}
            isSaving={saveMutation.isPending}
            saveLabel='Save WeCom AI bot settings'
          />
          <FormField
            control={form.control}
            name='enabled'
            render={({ field }) => (
              <SettingsSwitchItem>
                <SettingsSwitchContent>
                  <FormLabel>{t('Enable the WeCom AI bot')}</FormLabel>
                  <FormDescription>
                    {t(
                      'Answer channel, balance and log queries from WeCom chats through a long-connection conversational bot'
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
          <p className='text-muted-foreground text-xs'>
            {t('Connection status')}: <span>{statusLabel}</span>
            {status?.last_error ? <span> · {status.last_error}</span> : null}
          </p>

          <div className='grid grid-cols-1 gap-4 md:grid-cols-2'>
            <FormField
              control={form.control}
              name='bot_id'
              render={({ field }) => (
                <FormItem>
                  <FormLabel>{t('BotID')}</FormLabel>
                  <FormControl>
                    <Input {...field} value={field.value ?? ''} />
                  </FormControl>
                  <FormDescription>
                    {t(
                      'Issued by WeCom Workbench - Smart Bots (API mode); only one long connection is allowed per bot'
                    )}
                  </FormDescription>
                  <FormMessage />
                </FormItem>
              )}
            />
            <FormField
              control={form.control}
              name='secret'
              render={({ field }) => (
                <FormItem>
                  <FormLabel>{t('Secret')}</FormLabel>
                  <FormControl>
                    <Input
                      type='password'
                      autoComplete='new-password'
                      placeholder={t('Leave blank to keep the current secret')}
                      {...field}
                      value={field.value ?? ''}
                    />
                  </FormControl>
                  <FormDescription>
                    {config?.secret_masked
                      ? t('Current secret: {{value}}', {
                          value: config.secret_masked,
                        })
                      : t('No secret configured yet')}
                  </FormDescription>
                  <FormMessage />
                </FormItem>
              )}
            />
            <FormField
              control={form.control}
              name='channel_id'
              render={({ field }) => (
                <FormItem>
                  <FormLabel>{t('Model channel ID')}</FormLabel>
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
                      'The channel that powers the bot conversations; usage is free of quota'
                    )}
                  </FormDescription>
                  <FormMessage />
                </FormItem>
              )}
            />
            <FormField
              control={form.control}
              name='model'
              render={({ field }) => (
                <FormItem>
                  <FormLabel>{t('Model name')}</FormLabel>
                  <FormControl>
                    <Input {...field} value={field.value ?? ''} />
                  </FormControl>
                  <FormMessage />
                </FormItem>
              )}
            />
            <FormField
              control={form.control}
              name='max_tool_rounds'
              render={({ field }) => (
                <FormItem>
                  <FormLabel>{t('Max tool rounds per question')}</FormLabel>
                  <FormControl>
                    <Input
                      type='number'
                      min={1}
                      max={10}
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
              name='session_rounds'
              render={({ field }) => (
                <FormItem>
                  <FormLabel>{t('Conversation history rounds')}</FormLabel>
                  <FormControl>
                    <Input
                      type='number'
                      min={1}
                      max={50}
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
              name='session_ttl_minutes'
              render={({ field }) => (
                <FormItem>
                  <FormLabel>{t('Conversation TTL (minutes)')}</FormLabel>
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
                      'Follow-up questions can reference earlier context within this window'
                    )}
                  </FormDescription>
                  <FormMessage />
                </FormItem>
              )}
            />
            <FormField
              control={form.control}
              name='allow_all_users'
              render={({ field }) => (
                <SettingsSwitchItem>
                  <SettingsSwitchContent>
                    <FormLabel>{t('Allow all WeCom users')}</FormLabel>
                    <FormDescription>
                      {t(
                        'When off, only whitelisted userids can talk to the bot'
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
              name='group_enabled'
              render={({ field }) => (
                <SettingsSwitchItem>
                  <SettingsSwitchContent>
                    <FormLabel>{t('Respond in group chats')}</FormLabel>
                    <FormDescription>
                      {t('The bot only sees group messages that @mention it')}
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

          <FormField
            control={form.control}
            name='user_whitelist'
            render={({ field }) => (
              <FormItem>
                <FormLabel>{t('User whitelist')}</FormLabel>
                <FormControl>
                  <Textarea
                    rows={2}
                    placeholder={t('Comma-separated WeCom userids')}
                    {...field}
                    value={field.value ?? ''}
                  />
                </FormControl>
                <FormDescription>
                  {t(
                    'Rejected userids are recorded in server logs and can be copied here'
                  )}
                </FormDescription>
                <FormMessage />
              </FormItem>
            )}
          />
          <FormField
            control={form.control}
            name='admin_userids'
            render={({ field }) => (
              <FormItem>
                <FormLabel>{t('Admin userids')}</FormLabel>
                <FormControl>
                  <Textarea
                    rows={2}
                    placeholder={t('Comma-separated WeCom userids')}
                    {...field}
                    value={field.value ?? ''}
                  />
                </FormControl>
                <FormDescription>
                  {t(
                    'Admins can use sensitive tools such as per-user usage and live balance refresh'
                  )}
                </FormDescription>
                <FormMessage />
              </FormItem>
            )}
          />
          <FormField
            control={form.control}
            name='report_enabled'
            render={({ field }) => (
              <SettingsSwitchItem>
                <SettingsSwitchContent>
                  <FormLabel>{t('Send daily report')}</FormLabel>
                  <FormDescription>
                    {t(
                      "Send yesterday's usage, spend and anomaly report to authorized targets"
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
            name='report_hour'
            render={({ field }) => (
              <FormItem>
                <FormLabel>
                  {t('Daily report hour (Beijing time, 0-23)')}
                </FormLabel>
                <FormControl>
                  <Input
                    type='number'
                    min={0}
                    max={23}
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
            name='report_targets'
            render={({ field }) => (
              <FormItem>
                <FormLabel>{t('Report push targets')}</FormLabel>
                <FormControl>
                  <Input
                    placeholder={t('Leave empty to push to admin userids only')}
                    {...field}
                    value={field.value ?? ''}
                  />
                </FormControl>
                <FormDescription>
                  {t(
                    'Comma-separated userids or group:chatid; single chats require access; empty = authorized admins only; no admins = no delivery'
                  )}
                </FormDescription>
                <FormMessage />
              </FormItem>
            )}
          />
          <FormField
            control={form.control}
            name='welcome_text'
            render={({ field }) => (
              <FormItem>
                <FormLabel>{t('Welcome message')}</FormLabel>
                <FormControl>
                  <Textarea
                    rows={2}
                    placeholder={t('Sent when a user opens the chat')}
                    {...field}
                    value={field.value ?? ''}
                  />
                </FormControl>
                <FormDescription>
                  {t('Leave blank to disable the welcome message')}
                </FormDescription>
                <FormMessage />
              </FormItem>
            )}
          />
        </SettingsForm>
      </Form>
    </SettingsSection>
  )
}
