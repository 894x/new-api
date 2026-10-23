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
import { handleServerError } from '@/lib/handle-server-error'

import {
  createWecomBot,
  deleteWecomBot,
  getWecomBots,
  updateWecomBot,
} from '../api'
import type { WecomBot } from '../types'

const wecomMentionTokenPattern = /^[A-Za-z0-9_@.-]{1,64}$/

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

  const handleAdd = () => {
    setEditingBot(null)
    form.reset({
      name: '',
      webhook_api_key: '',
      mention_userids: '',
      enabled: true,
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
              'Manage bots that receive channel alerts; alerts are broadcast to all enabled bots'
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
              <StaticRowActions
                editLabel={t('Edit')}
                deleteLabel={t('Delete')}
                menuLabel={t('Open menu')}
                onEdit={() => handleEdit(bot)}
                onDelete={() => setDeletingBot(bot)}
              />
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
