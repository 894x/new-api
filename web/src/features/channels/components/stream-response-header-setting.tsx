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
import { useFormContext } from 'react-hook-form'
import { useTranslation } from 'react-i18next'

import {
  FormControl,
  FormDescription,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from '@/components/ui/form'
import { Textarea } from '@/components/ui/textarea'

import type { ChannelFormValues } from '../lib/channel-form'

export function StreamResponseHeaderSetting(props: { disabled?: boolean }) {
  const { t } = useTranslation()
  const form = useFormContext<ChannelFormValues>()

  return (
    <FormField
      control={form.control}
      name='stream_response_headers'
      render={({ field }) => (
        <FormItem className='space-y-3 border-t pt-4'>
          <FormLabel>{t('Stream response headers')}</FormLabel>
          <FormDescription>
            {t(
              'Forward additional upstream headers to clients in streaming responses. Enter one exact header name per line. Default Codex headers remain enabled; global blocks and gateway-managed headers take priority.'
            )}
          </FormDescription>
          <FormControl>
            <Textarea
              {...field}
              value={field.value ?? ''}
              rows={4}
              className='font-mono text-xs'
              placeholder={
                'Msh-Usage-Cache-Write-Tokens-5m\nMsh-Usage-Cache-Write-Tokens-1h'
              }
              disabled={props.disabled}
            />
          </FormControl>
          <FormMessage />
        </FormItem>
      )}
    />
  )
}
