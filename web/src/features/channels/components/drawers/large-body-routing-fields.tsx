import { useWatch, type Control } from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import {
  FormControl,
  FormDescription,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from '@/components/ui/form'
import { Input } from '@/components/ui/input'
import { Switch } from '@/components/ui/switch'
import type { ChannelFormValues } from '../../lib/channel-form'

export function LargeBodyRoutingFields({
  control,
}: {
  control: Control<ChannelFormValues>
}) {
  const { t } = useTranslation()
  const [protocol, enabled] = useWatch({
    control,
    name: ['http_protocol', 'http1_large_body_enabled'],
  })
  return (
    <div className='space-y-4'>
      <FormField
        control={control}
        name='http1_large_body_enabled'
        render={({ field }) => (
          <FormItem>
            <div className='flex items-center justify-between gap-4'>
              <FormLabel>
                {t('Use HTTP/1.1 for large request bodies')}
              </FormLabel>
              <FormControl>
                <Switch
                  checked={field.value ?? false}
                  disabled={protocol === 'http1'}
                  onCheckedChange={field.onChange}
                />
              </FormControl>
            </div>
            <FormDescription>
              {t(
                'Known upstream bodies at or above the threshold use a separate HTTP/1.1 keep-alive pool. Smaller or unknown-length bodies retain the original policy. No extra buffering or retries.'
              )}
            </FormDescription>
            <FormMessage />
          </FormItem>
        )}
      />
      {enabled && protocol !== 'http1' && (
        <FormField
          control={control}
          name='http1_large_body_threshold_kib'
          render={({ field }) => (
            <FormItem>
              <FormLabel>{t('Request body threshold (KiB)')}</FormLabel>
              <FormControl>
                <Input
                  type='number'
                  min={1}
                  max={1048576}
                  step={1}
                  value={field.value ?? ''}
                  onBlur={field.onBlur}
                  name={field.name}
                  ref={field.ref}
                  onChange={(event) =>
                    field.onChange(
                      event.target.value === ''
                        ? undefined
                        : Number(event.target.value)
                    )
                  }
                />
              </FormControl>
              <FormDescription>
                {t(
                  'Measured from the final upstream body, not input tokens. 1024 KiB = 1 MiB; start with a single channel and compare latency and errors.'
                )}
              </FormDescription>
              <FormMessage />
            </FormItem>
          )}
        />
      )}
    </div>
  )
}
