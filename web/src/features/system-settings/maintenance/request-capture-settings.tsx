import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { z } from 'zod'

import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'

import { useSystemOptions } from '../hooks/use-system-options'
import { useUpdateOption } from '../hooks/use-update-option'

const captureSettingsSchema = z.object({
  retention_days: z.number().int().min(1).max(365),
  max_gib: z.number().int().min(1).max(365),
})
type CaptureSettings = z.infer<typeof captureSettingsSchema>

export function RequestCaptureSettings() {
  const { t } = useTranslation()
  const options = useSystemOptions()
  if (options.isPending) return <p role='status'>{t('Loading...')}</p>
  const raw = options.data?.data?.find(
    (option) => option.key === 'RequestCaptureStorage'
  )?.value
  let settings: CaptureSettings | undefined
  if (!options.isError && options.data?.success) {
    try {
      settings = captureSettingsSchema.parse(
        JSON.parse(raw ?? '{"retention_days":3,"max_gib":10}')
      )
    } catch {
      /* Invalid server data must not become editable defaults. */
    }
  }
  if (!settings) {
    return (
      <div role='alert'>
        <p>{t('Failed to load capture settings')}</p>
        <Button
          type='button'
          variant='outline'
          onClick={() => void options.refetch()}
        >
          {t('Retry')}
        </Button>
      </div>
    )
  }
  return (
    <RequestCaptureSettingsForm
      key={raw ?? 'default'}
      initialValue={settings}
    />
  )
}

function RequestCaptureSettingsForm({
  initialValue,
}: {
  initialValue: CaptureSettings
}) {
  const { t } = useTranslation()
  const updateOption = useUpdateOption()
  const [days, setDays] = useState(String(initialValue.retention_days))
  const [capacity, setCapacity] = useState(String(initialValue.max_gib))
  const [invalid, setInvalid] = useState(false)
  const errorId = 'capture-storage-validation'
  return (
    <form
      noValidate
      className='space-y-4 rounded-md border p-4'
      onSubmit={(event) => {
        event.preventDefault()
        const candidate = captureSettingsSchema.safeParse({
          retention_days: Number(days),
          max_gib: Number(capacity),
        })
        setInvalid(!candidate.success)
        if (!candidate.success) return
        updateOption.mutate({
          key: 'RequestCaptureStorage',
          value: JSON.stringify(candidate.data),
        })
      }}
    >
      <h4 className='text-sm font-medium'>{t('Request capture storage')}</h4>
      <p className='text-muted-foreground text-sm'>
        {t(
          'Retention applies to new captures. Capacity changes take effect without restarting; old files are cleaned up in the background. Existing expiry dates stay unchanged.'
        )}
      </p>
      <div className='grid min-w-0 gap-4 sm:grid-cols-2'>
        <div className='min-w-0 space-y-2'>
          <Label htmlFor='capture-retention-days'>
            {t('Capture retention (days)')}
          </Label>
          <Input
            id='capture-retention-days'
            type='number'
            min={1}
            max={365}
            step={1}
            value={days}
            disabled={updateOption.isPending}
            aria-invalid={invalid}
            aria-describedby={invalid ? errorId : undefined}
            onChange={(event) => setDays(event.target.value)}
          />
        </div>
        <div className='min-w-0 space-y-2'>
          <Label htmlFor='capture-max-gib'>
            {t('Capture capacity per instance (GiB)')}
          </Label>
          <Input
            id='capture-max-gib'
            type='number'
            min={1}
            max={365}
            step={1}
            value={capacity}
            disabled={updateOption.isPending}
            aria-invalid={invalid}
            aria-describedby={invalid ? errorId : undefined}
            onChange={(event) => setCapacity(event.target.value)}
          />
        </div>
      </div>
      {invalid && (
        <p id={errorId} role='alert' className='text-destructive text-sm'>
          {t('Enter whole numbers between 1 and 365.')}
        </p>
      )}
      {(updateOption.isError || updateOption.data?.success === false) && (
        <p role='alert' className='text-destructive text-sm'>
          {t('Failed to save capture settings')}
        </p>
      )}
      <Button type='submit' disabled={updateOption.isPending}>
        {updateOption.isPending ? t('Saving...') : t('Save capture settings')}
      </Button>
    </form>
  )
}
