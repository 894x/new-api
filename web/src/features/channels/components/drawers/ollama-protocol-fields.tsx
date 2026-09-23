import { useWatch, type Control } from 'react-hook-form'
import { useTranslation } from 'react-i18next'

import {
  FormControl,
  FormDescription,
  FormField,
  FormItem,
  FormLabel,
} from '@/components/ui/form'
import { Switch } from '@/components/ui/switch'

import type { ChannelFormValues } from '../../lib/channel-form'

export function OllamaProtocolFields(props: {
  control: Control<ChannelFormValues>
}) {
  const { t } = useTranslation()
  const channelType = useWatch({ control: props.control, name: 'type' })
  if (channelType !== 4) return null

  return (
    <FormField
      control={props.control}
      name='ollama_native_claude'
      render={({ field }) => (
        <FormItem className='flex items-center justify-between gap-4 px-4 py-3'>
          <div className='min-w-0 space-y-0.5'>
            <FormLabel>{t('Use native Claude API')}</FormLabel>
            <FormDescription>
              {t(
                'Send Claude requests to Ollama /v1/messages. Disabled by default to keep conversion to /api/chat. Requires native Claude support upstream.'
              )}
            </FormDescription>
          </div>
          <FormControl>
            <Switch
              checked={field.value ?? false}
              onCheckedChange={field.onChange}
            />
          </FormControl>
        </FormItem>
      )}
    />
  )
}
