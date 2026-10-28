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
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { useId, useState } from 'react'
import { useForm } from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { ConfirmDialog } from '@/components/confirm-dialog'
import {
  Form,
  FormControl,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from '@/components/ui/form'
import { Input } from '@/components/ui/input'
import { toIntlLocale } from '@/i18n/languages'
import { handleServerError } from '@/lib/handle-server-error'
import {
  getServerErrorMessage,
  requireServerSuccess,
} from '@/lib/server-error-message'

import {
  createChannelResourceGroup,
  type ResourceGroupConflict,
} from '../../api'
import { channelsQueryKeys } from '../../lib/channel-actions'
import {
  createChannelResourceGroupSchema,
  type ChannelResourceGroupValues,
} from '../../lib/channel-resource-group'

type ChannelResourceGroupDialogProps = {
  channelId: number
  channelName: string
  onOpenChange: (open: boolean) => void
  onApplied?: (groupName: string) => void
}

export function ChannelResourceGroupDialog(
  props: ChannelResourceGroupDialogProps
) {
  const { t, i18n } = useTranslation()
  const locale = toIntlLocale(i18n.resolvedLanguage || i18n.language)
  const inputId = useId()
  const queryClient = useQueryClient()
  const [conflict, setConflict] = useState<ResourceGroupConflict | null>(null)
  const form = useForm<ChannelResourceGroupValues>({
    resolver: zodResolver(createChannelResourceGroupSchema(t)),
    defaultValues: { name: `资源-${props.channelName.trim()}` },
  })
  const mutation = useMutation({
    mutationFn: async (values: ChannelResourceGroupValues) => {
      const response = await createChannelResourceGroup(props.channelId, {
        group_name: values.name,
        reuse_existing: conflict?.group_name === values.name,
      })
      if (!response.success && response.code === 'RESOURCE_GROUP_EXISTS') {
        setConflict(response.data)
        return null
      }
      requireServerSuccess(response)
      return response.success ? response.data : null
    },
    onError: (error) =>
      handleServerError(error, undefined, {
        title: t(getServerErrorMessage(error)),
      }),
    onSuccess: (result) => {
      if (!result) return
      props.onApplied?.(result.group_name)
      void queryClient.invalidateQueries({ queryKey: channelsQueryKeys.all })
      void queryClient.invalidateQueries({ queryKey: ['groups'] })
      void queryClient.invalidateQueries({ queryKey: ['system-options'] })
      toast.success(t('Resource group associated with channel'))
      props.onOpenChange(false)
    },
  })

  return (
    <ConfirmDialog
      open
      onOpenChange={(open) => {
        if (!mutation.isPending) props.onOpenChange(open)
      }}
      title={t('Create resource group')}
      desc={
        <p className='break-words'>
          {t(
            'Create a private group and append it to channel "{{name}}" without replacing its current groups.',
            { name: props.channelName }
          )}
        </p>
      }
      confirmText={
        conflict ? t('Reuse and associate') : t('Create and associate')
      }
      isLoading={mutation.isPending}
      handleConfirm={form.handleSubmit((values) => mutation.mutate(values))}
    >
      <Form {...form}>
        <form
          onSubmit={(event) => {
            event.stopPropagation()
            void form.handleSubmit((values) => mutation.mutate(values))(event)
          }}
          className='min-w-0 space-y-4'
        >
          <FormField
            control={form.control}
            name='name'
            render={({ field }) => (
              <FormItem>
                <FormLabel htmlFor={inputId}>{t('Group name')}</FormLabel>
                <FormControl>
                  <Input
                    {...field}
                    id={inputId}
                    disabled={mutation.isPending}
                    onChange={(event) => {
                      field.onChange(event)
                      setConflict(null)
                    }}
                  />
                </FormControl>
                <FormMessage />
              </FormItem>
            )}
          />
          <p className='text-muted-foreground text-sm'>
            {t(
              'New groups use ratio 1 and are not added to public groups or automatic routing. Existing group ratios are preserved when reused.'
            )}
          </p>
          {conflict && (
            <div
              role='status'
              className='bg-muted/50 min-w-0 space-y-2 rounded-md border p-3 text-sm'
            >
              <p>
                {t(
                  'This private group already exists. Reuse it or change the name.'
                )}
              </p>
              <p>
                {t('Existing ratio: {{ratio}}', {
                  ratio: new Intl.NumberFormat(locale, {
                    maximumFractionDigits: 20,
                  }).format(conflict.ratio),
                })}
              </p>
              <p className='break-words'>
                {t('Associated channels: {{channels}}', {
                  channels:
                    conflict.channels
                      .map((channel) => `${channel.name} (#${channel.id})`)
                      .join(', ') || t('None'),
                })}
              </p>
            </div>
          )}
        </form>
      </Form>
    </ConfirmDialog>
  )
}
