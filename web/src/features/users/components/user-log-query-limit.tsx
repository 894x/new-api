import { useFormContext } from 'react-hook-form'
import { useTranslation } from 'react-i18next'

import { SideDrawerSection } from '@/components/drawer-layout'
import { Button } from '@/components/ui/button'
import {
  FormControl,
  FormDescription,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from '@/components/ui/form'
import { Input } from '@/components/ui/input'

import type { UserFormValues } from '../lib/user-form'

export function UserLogQueryLimit() {
  const { t } = useTranslation()
  const form = useFormContext<UserFormValues>()
  const policy = form.watch('log_query_rate_limit_policy')

  return (
    <SideDrawerSection>
      <FormField
        control={form.control}
        name='log_query_rate_limit'
        render={({ field, fieldState }) => (
          <FormItem>
            <FormLabel>{t('Log query limit')}</FormLabel>
            <div className='flex gap-2'>
              <FormControl>
                <Input
                  {...field}
                  type='number'
                  min={0}
                  max={policy?.max_limit ?? 60000}
                  step={1}
                  disabled={!policy}
                  value={field.value ?? 0}
                  onChange={(event) =>
                    field.onChange(
                      event.target.value === '' ? 0 : Number(event.target.value)
                    )
                  }
                />
              </FormControl>
              <Button
                type='button'
                variant='outline'
                disabled={!policy}
                onClick={() => field.onChange(0)}
              >
                {t('Use global default')}
              </Button>
            </div>
            {policy && (
              <FormDescription>
                {t('Global default: {{limit}} requests / {{seconds}} seconds', {
                  limit: policy.default_limit,
                  seconds: policy.window_seconds,
                })}
              </FormDescription>
            )}
            <FormDescription>
              {t(
                '0 inherits the global default. Token log queries share this limit across all API keys of this user. Global IP limits still apply.'
              )}
            </FormDescription>
            {policy && !policy.enabled && (
              <FormDescription>
                {t(
                  'Log query limiting is disabled globally; saved overrides will apply when enabled.'
                )}
              </FormDescription>
            )}
            {fieldState.error && (
              <FormMessage>
                {t('Enter a whole number from 0 to 60000')}
              </FormMessage>
            )}
          </FormItem>
        )}
      />
    </SideDrawerSection>
  )
}
