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
import { Plus } from 'lucide-react'
import { useState } from 'react'
import { useForm } from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import * as z from 'zod'

import { ConfirmDialog } from '@/components/confirm-dialog'
import { StaticDataTable } from '@/components/data-table/static/static-data-table'
import { StaticRowActions } from '@/components/data-table/static/static-row-actions'
import { Dialog } from '@/components/dialog'
import { Button } from '@/components/ui/button'
import { Checkbox } from '@/components/ui/checkbox'
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
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { Switch } from '@/components/ui/switch'
import { handleServerError } from '@/lib/handle-server-error'

import {
  createWecomBot,
  deleteWecomBot,
  getWecomBots,
  testWecomBot,
  updateWecomBot,
} from '../api'
import type { WecomBot } from '../types'

const wecomMentionTokenPattern = /^[A-Za-z0-9_@.-]{1,64}$/

// 告警类型目录：value 与后端 WecomBotCategory 常量一致
const WECOM_BOT_ALERT_CATEGORIES = [
  { value: 'channel_error', labelKey: 'Channel error alerts' },
  { value: 'channel_disabled', labelKey: 'Channel auto-disabled alerts' },
  { value: 'channel_enabled', labelKey: 'Channel re-enabled alerts' },
  { value: 'balance_patrol', labelKey: 'Balance patrol alerts' },
  { value: 'watchdog', labelKey: 'Channel watchdog alerts' },
] as const

const botSchema = (t: (key: string) => string) =>
  z.object({
    name: z
      .string()
      .min(1, t('Bot name is required'))
      .max(64, t('Bot name must be at most 64 characters')),
    webhook_api_key: z.string(),
    mention_userids: z.string().refine((value) => {
      const trimmed = value.trim()
      if (!trimmed) return true
      return trimmed
        .split(/[,，\s]+/)
        .every((token) => wecomMentionTokenPattern.test(token))
    }, t('Mention list accepts comma-separated WeCom userids or @all')),
    enabled: z.boolean(),
    subscriptions: z.array(z.string()),
    message_style: z.string(),
  })

type BotFormValues = z.infer<ReturnType<typeof botSchema>>

const BOT_FORM_ID = 'wecom-bot-form'

export function WecomBotList() {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const [showDialog, setShowDialog] = useState(false)
  const [editingBot, setEditingBot] = useState<WecomBot | null>(null)
  const [deletingBot, setDeletingBot] = useState<WecomBot | null>(null)

  const botsQuery = useQuery({
    queryKey: ['wecom-bots'],
    queryFn: getWecomBots,
  })
  const bots = botsQuery.data?.data ?? []

  const form = useForm<BotFormValues>({
    resolver: zodResolver(botSchema(t)),
    defaultValues: {
      name: '',
      webhook_api_key: '',
      mention_userids: '',
      enabled: true,
      subscriptions: [],
      message_style: 'markdown',
    },
  })

  const invalidateBots = () =>
    queryClient.invalidateQueries({ queryKey: ['wecom-bots'] })
  const notifySuccess = () => toast.success(t('Setting updated successfully'))
  const notifyError = (error: Error) =>
    handleServerError(error, t('Failed to update setting'))

  const saveMutation = useMutation({
    mutationFn: (values: BotFormValues) => {
      const request = {
        name: values.name.trim(),
        webhook_api_key: values.webhook_api_key.trim(),
        mention_userids: values.mention_userids.trim(),
        enabled: values.enabled,
        subscriptions: values.subscriptions,
        message_style: values.message_style,
      }
      if (editingBot) {
        return updateWecomBot(editingBot.id, request)
      }
      return createWecomBot(request)
    },
    onSuccess: () => {
      invalidateBots()
      notifySuccess()
      setShowDialog(false)
    },
    onError: notifyError,
  })

  const toggleMutation = useMutation({
    mutationFn: (bot: WecomBot) =>
      updateWecomBot(bot.id, {
        name: bot.name,
        // 留空表示保留现有凭据
        webhook_api_key: '',
        mention_userids: bot.mention_userids,
        enabled: !bot.enabled,
        subscriptions: bot.subscriptions ?? [],
        message_style: bot.message_style || 'markdown',
      }),
    onSuccess: () => {
      invalidateBots()
      notifySuccess()
    },
    onError: notifyError,
  })

  const deleteMutation = useMutation({
    mutationFn: (id: string) => deleteWecomBot(id),
    onSuccess: () => {
      invalidateBots()
      setDeletingBot(null)
      notifySuccess()
    },
    onError: notifyError,
  })

  const testMutation = useMutation({
    mutationFn: (id: string) => testWecomBot(id),
    onSuccess: () => {
      toast.success(t('Test message sent, please check the WeCom group'))
    },
    onError: (error) => handleServerError(error),
  })

  const handleAdd = () => {
    setEditingBot(null)
    form.reset({
      name: '',
      webhook_api_key: '',
      mention_userids: '',
      enabled: true,
      subscriptions: [],
      message_style: 'markdown',
    })
    setShowDialog(true)
  }

  const handleEdit = (bot: WecomBot) => {
    setEditingBot(bot)
    form.reset({
      name: bot.name,
      webhook_api_key: '',
      mention_userids: bot.mention_userids,
      enabled: bot.enabled,
      subscriptions: bot.subscriptions ?? [],
      message_style: bot.message_style || 'markdown',
    })
    setShowDialog(true)
  }

  const handleSubmit = (values: BotFormValues) => {
    if (!editingBot && !values.webhook_api_key.trim()) {
      form.setError('webhook_api_key', {
        message: t('Bot webhook key is required'),
      })
      return
    }
    saveMutation.mutate(values)
  }

  return (
    <div className='space-y-4'>
      <div className='flex flex-wrap items-center justify-between gap-2'>
        <div>
          <h4 className='font-medium'>{t('WeCom bot list')}</h4>
          <p className='text-muted-foreground mt-1 text-xs'>
            {t(
              'Manage bots that receive channel alerts; each bot can subscribe to specific alert types'
            )}
          </p>
        </div>
        <Button onClick={handleAdd} size='sm'>
          <Plus className='mr-2 h-4 w-4' />
          {t('Add bot')}
        </Button>
      </div>

      <StaticDataTable
        data={bots}
        getRowKey={(bot) => bot.id}
        emptyContent={t('No WeCom bots yet. Click "Add bot" to create one.')}
        columns={[
          {
            id: 'name',
            header: t('Bot name'),
            cell: (bot) => bot.name,
          },
          {
            id: 'webhook',
            header: t('Webhook'),
            cellClassName: 'max-w-xs truncate',
            cell: (bot) => bot.webhook_masked || '-',
          },
          {
            id: 'mentions',
            header: t('Mention members'),
            cellClassName: 'max-w-xs truncate',
            cell: (bot) => bot.mention_userids || '-',
          },
          {
            id: 'subscriptions',
            header: t('Alert subscriptions'),
            cellClassName: 'max-w-xs truncate',
            cell: (bot) => {
              const subscriptions = bot.subscriptions ?? []
              if (subscriptions.length === 0) {
                return <span className='text-muted-foreground'>{t('All')}</span>
              }
              return subscriptions
                .map(
                  (value) =>
                    WECOM_BOT_ALERT_CATEGORIES.find(
                      (category) => category.value === value
                    )?.labelKey ?? value
                )
                .map((labelKey) => t(labelKey))
                .join(', ')
            },
          },
          {
            id: 'enabled',
            header: t('Enabled'),
            cell: (bot) => (
              <Switch
                checked={bot.enabled}
                onCheckedChange={() => toggleMutation.mutate(bot)}
                disabled={toggleMutation.isPending}
                aria-label={t('Enabled')}
              />
            ),
          },
          {
            id: 'actions',
            header: t('Actions'),
            cell: (bot) => (
              <div className='flex items-center gap-2'>
                <Button
                  variant='outline'
                  size='sm'
                  onClick={() => testMutation.mutate(bot.id)}
                  disabled={testMutation.isPending}
                  aria-busy={testMutation.isPending}
                >
                  {t('Send test message')}
                </Button>
                <StaticRowActions
                  editLabel={t('Edit')}
                  deleteLabel={t('Delete')}
                  menuLabel={t('Open menu')}
                  onEdit={() => handleEdit(bot)}
                  onDelete={() => setDeletingBot(bot)}
                />
              </div>
            ),
          },
        ]}
      />

      <Dialog
        open={showDialog}
        onOpenChange={setShowDialog}
        title={editingBot ? t('Edit bot') : t('Add bot')}
        description={t(
          'Paste the robot key or the full webhook URL copied from WeCom'
        )}
        contentHeight='auto'
        bodyClassName='space-y-4'
        footer={
          <>
            <Button
              type='button'
              variant='outline'
              onClick={() => setShowDialog(false)}
            >
              {t('Cancel')}
            </Button>
            <Button
              type='submit'
              form={BOT_FORM_ID}
              disabled={saveMutation.isPending}
            >
              {editingBot ? t('Update') : t('Add')}
            </Button>
          </>
        }
      >
        <Form {...form}>
          <form
            id={BOT_FORM_ID}
            onSubmit={form.handleSubmit(handleSubmit)}
            className='space-y-4'
          >
            <FormField
              control={form.control}
              name='name'
              render={({ field }) => (
                <FormItem>
                  <FormLabel>{t('Bot name')}</FormLabel>
                  <FormControl>
                    <Input
                      autoComplete='off'
                      placeholder={t('Ops alert group')}
                      {...field}
                      onChange={(event) => field.onChange(event.target.value)}
                    />
                  </FormControl>
                  <FormMessage />
                </FormItem>
              )}
            />
            <FormField
              control={form.control}
              name='webhook_api_key'
              render={({ field }) => (
                <FormItem>
                  <FormLabel>{t('Bot webhook key')}</FormLabel>
                  <FormControl>
                    <Input
                      autoComplete='off'
                      type='password'
                      placeholder={t('Enter new key to update')}
                      {...field}
                      onChange={(event) => field.onChange(event.target.value)}
                    />
                  </FormControl>
                  {editingBot && (
                    <FormDescription>
                      {t('Leave blank to keep the existing credential')}
                    </FormDescription>
                  )}
                  <FormMessage />
                </FormItem>
              )}
            />
            <FormField
              control={form.control}
              name='mention_userids'
              render={({ field }) => (
                <FormItem>
                  <FormLabel>{t('Mention members (userids)')}</FormLabel>
                  <FormControl>
                    <Input
                      autoComplete='off'
                      placeholder='zhangsan,lisi'
                      {...field}
                      onChange={(event) => field.onChange(event.target.value)}
                    />
                  </FormControl>
                  <FormDescription>
                    {t('Comma-separated WeCom userids to @mention, or @all')}
                  </FormDescription>
                  <FormMessage />
                </FormItem>
              )}
            />
            <FormField
              control={form.control}
              name='message_style'
              render={({ field }) => (
                <FormItem>
                  <FormLabel>{t('Message style')}</FormLabel>
                  <Select value={field.value} onValueChange={field.onChange}>
                    <SelectTrigger className='w-full'>
                      <SelectValue />
                    </SelectTrigger>
                    <SelectContent>
                      <SelectItem value='markdown'>{t('Markdown')}</SelectItem>
                      <SelectItem value='card'>
                        {t('Template card')}
                      </SelectItem>
                    </SelectContent>
                  </Select>
                  <FormDescription>
                    {t(
                      'Template card style does not support mentions and falls back to Markdown when a mention list is configured'
                    )}
                  </FormDescription>
                  <FormMessage />
                </FormItem>
              )}
            />
            <FormField
              control={form.control}
              name='subscriptions'
              render={({ field }) => (
                <FormItem>
                  <FormLabel>{t('Alert subscriptions')}</FormLabel>
                  <FormDescription>
                    {t('Leave empty to receive all alert types')}
                  </FormDescription>
                  <div className='grid grid-cols-1 gap-2 sm:grid-cols-2'>
                    {WECOM_BOT_ALERT_CATEGORIES.map((category) => {
                      const checked = field.value.includes(category.value)
                      return (
                        <label
                          key={category.value}
                          className='flex cursor-pointer items-center gap-2 text-sm font-normal'
                        >
                          <Checkbox
                            checked={checked}
                            onCheckedChange={(next) =>
                              field.onChange(
                                next
                                  ? [...field.value, category.value]
                                  : field.value.filter(
                                      (value) => value !== category.value
                                    )
                              )
                            }
                          />
                          {t(category.labelKey)}
                        </label>
                      )
                    })}
                  </div>
                  <FormMessage />
                </FormItem>
              )}
            />
            <FormField
              control={form.control}
              name='enabled'
              render={({ field }) => (
                <FormItem className='flex items-center justify-between rounded-lg border p-3'>
                  <FormLabel className='font-normal'>{t('Enabled')}</FormLabel>
                  <FormControl>
                    <Switch
                      checked={field.value}
                      onCheckedChange={field.onChange}
                    />
                  </FormControl>
                </FormItem>
              )}
            />
          </form>
        </Form>
      </Dialog>

      <ConfirmDialog
        open={deletingBot !== null}
        onOpenChange={(open) => {
          if (!open) {
            setDeletingBot(null)
          }
        }}
        title={t('Delete bot')}
        desc={t('This WeCom bot will be removed from the alert list.')}
        destructive
        handleConfirm={() => {
          if (deletingBot) {
            deleteMutation.mutate(deletingBot.id)
          }
        }}
        isLoading={deleteMutation.isPending}
      />
    </div>
  )
}
