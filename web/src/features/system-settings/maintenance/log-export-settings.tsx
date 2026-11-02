import { zodResolver } from '@hookform/resolvers/zod'
import { useQueryClient } from '@tanstack/react-query'
import { useForm } from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import { z } from 'zod'

import { ErrorState } from '@/components/error-state'
import { LoadingState } from '@/components/loading-state'
import { Button } from '@/components/ui/button'
import {
  Form,
  FormControl,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from '@/components/ui/form'
import { Input } from '@/components/ui/input'

import { useSystemOptions } from '../hooks/use-system-options'
import { useUpdateOption } from '../hooks/use-update-option'

const validationMessage =
  'Enter a whole number from 1 to 10,000,000. Export files are kept for 7 days; history is retained.'
const schema = z.object({
  maxRows: z
    .number({ error: validationMessage })
    .int(validationMessage)
    .min(1, validationMessage)
    .max(10000000, validationMessage),
})

export function LogExportSettings() {
  const { t } = useTranslation()
  const options = useSystemOptions()
  if (options.isPending) return <LoadingState />
  if (options.isError) {
    return (
      <ErrorState
        title={t('Unable to load export settings')}
        onRetry={() => void options.refetch()}
      />
    )
  }
  const value = Number(
    options.data?.data?.find((option) => option.key === 'LogExportMaxRows')
      ?.value ?? 100000
  )
  return <LogExportSettingsForm key={value} initialValue={value} />
}

function LogExportSettingsForm(props: { initialValue: number }) {
  const { t } = useTranslation()
  const updateOption = useUpdateOption()
  const queryClient = useQueryClient()
  const form = useForm<z.infer<typeof schema>>({
    resolver: zodResolver(schema),
    defaultValues: { maxRows: props.initialValue },
  })
  return (
    <Form {...form}>
      <form
        noValidate
        className='space-y-4 rounded-md border p-4'
        onSubmit={form.handleSubmit((values) =>
          updateOption.mutate(
            {
              key: 'LogExportMaxRows',
              value: String(values.maxRows),
            },
            {
              onSuccess: () =>
                void queryClient.invalidateQueries({
                  queryKey: ['log-export-config'],
                }),
            }
          )
        )}
      >
        <h4 className='text-sm font-medium'>{t('Usage log exports')}</h4>
        <p className='text-muted-foreground text-sm'>
          {t(
            'Applies to new exports for all users. Existing jobs keep the limit recorded when created.'
          )}
        </p>
        <FormField
          control={form.control}
          name='maxRows'
          render={({ field }) => (
            <FormItem>
              <FormLabel htmlFor='log-export-max-rows'>
                {t('Maximum records per export')}
              </FormLabel>
              <FormControl>
                <Input
                  {...field}
                  value={Number.isNaN(field.value) ? '' : field.value}
                  id='log-export-max-rows'
                  type='number'
                  min={1}
                  max={10000000}
                  step={1}
                  disabled={updateOption.isPending}
                  onChange={(event) =>
                    field.onChange(
                      event.target.value === ''
                        ? Number.NaN
                        : Number(event.target.value)
                    )
                  }
                />
              </FormControl>
              <p className='text-muted-foreground text-xs'>
                {t(
                  'Enter a whole number from 1 to 10,000,000. Export files are kept for 7 days; history is retained.'
                )}
              </p>
              <FormMessage />
            </FormItem>
          )}
        />
        <Button
          type='submit'
          disabled={updateOption.isPending || !form.formState.isDirty}
        >
          {t('Save')}
        </Button>
      </form>
    </Form>
  )
}
