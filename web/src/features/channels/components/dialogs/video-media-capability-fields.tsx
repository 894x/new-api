import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'
import { Field, FieldDescription, FieldLabel } from '@/components/ui/field'
import { Input } from '@/components/ui/input'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'

import type { VideoMediaCapability } from '../../types'

function MediaBooleanField(props: {
  id: string
  label: string
  value?: boolean
  onChange: (value: boolean | undefined) => void
}) {
  const { t } = useTranslation()
  let value = 'inherit'
  if (props.value !== undefined) value = props.value ? 'enabled' : 'disabled'
  const items = [
    { value: 'inherit', label: t('Inherit') },
    { value: 'enabled', label: t('Enabled') },
    { value: 'disabled', label: t('Disabled') },
  ]
  return (
    <Field>
      <FieldLabel htmlFor={props.id}>{props.label}</FieldLabel>
      <Select
        items={items}
        value={value}
        onValueChange={(next) =>
          props.onChange(next === 'inherit' ? undefined : next === 'enabled')
        }
      >
        <SelectTrigger id={props.id} className='w-full'>
          <SelectValue />
        </SelectTrigger>
        <SelectContent alignItemWithTrigger={false}>
          {items.map((item) => (
            <SelectItem key={item.value} value={item.value}>
              {item.label}
            </SelectItem>
          ))}
        </SelectContent>
      </Select>
    </Field>
  )
}

export function VideoMediaCapabilityFields(props: {
  path: string
  value?: VideoMediaCapability
  effective?: VideoMediaCapability
  onChange: (media: VideoMediaCapability | undefined) => void
}) {
  const { t } = useTranslation()
  const media = props.value
  if (!media) {
    return (
      <Button
        type='button'
        variant='outline'
        onClick={() => props.onChange({ kind: 'video' })}
      >
        {t('Configure video delivery')}
      </Button>
    )
  }
  return (
    <fieldset className='grid grid-cols-1 gap-3 rounded-lg border p-3 sm:grid-cols-2'>
      <legend className='px-1 text-sm font-medium'>
        {t('Video delivery')}
      </legend>
      <FieldDescription className='sm:col-span-2'>
        {t(
          'Prefer the original format. Limits are decoded file bytes. URL sizes may be unknown. Base64 to URL requires object storage. This replaces legacy video conversion.'
        )}
      </FieldDescription>
      {(['url', 'base64'] as const).map((format) => (
        <div key={format} className='grid gap-3'>
          <MediaBooleanField
            id={`${props.path}-${format}-enabled`}
            label={
              format === 'url'
                ? t('Accept video URL')
                : t('Accept video Base64')
            }
            value={media.formats?.[format]?.supported}
            onChange={(supported) =>
              props.onChange({
                ...media,
                formats: {
                  ...media.formats,
                  [format]: { ...media.formats?.[format], supported },
                },
              })
            }
          />
          <Field>
            <FieldLabel htmlFor={`${props.path}-${format}-bytes`}>
              {format === 'url'
                ? t('URL maximum bytes')
                : t('Base64 maximum bytes')}
            </FieldLabel>
            <Input
              id={`${props.path}-${format}-bytes`}
              type='number'
              min={1}
              max={2 ** 40}
              step={1}
              value={media.formats?.[format]?.max_media_bytes ?? ''}
              placeholder={String(
                props.effective?.formats?.[format]?.max_media_bytes ?? ''
              )}
              onChange={(event) =>
                props.onChange({
                  ...media,
                  formats: {
                    ...media.formats,
                    [format]: {
                      ...media.formats?.[format],
                      max_media_bytes:
                        event.target.value === ''
                          ? undefined
                          : Number(event.target.value),
                    },
                  },
                })
              }
            />
          </Field>
        </div>
      ))}
      <MediaBooleanField
        id={`${props.path}-url-to-base64`}
        label={t('Video URL to Base64')}
        value={media.conversions?.url_to_base64}
        onChange={(enabled) =>
          props.onChange({
            ...media,
            conversions: { ...media.conversions, url_to_base64: enabled },
          })
        }
      />
      <MediaBooleanField
        id={`${props.path}-base64-to-url`}
        label={t('Video Base64 to URL')}
        value={media.conversions?.base64_to_url}
        onChange={(enabled) =>
          props.onChange({
            ...media,
            conversions: { ...media.conversions, base64_to_url: enabled },
          })
        }
      />
      <Button
        type='button'
        variant='ghost'
        className='sm:col-span-2'
        onClick={() => props.onChange(undefined)}
      >
        {t('Remove video delivery override')}
      </Button>
    </fieldset>
  )
}
