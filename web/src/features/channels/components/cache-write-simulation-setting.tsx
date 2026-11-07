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
import { useFormContext, useWatch } from 'react-hook-form'
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
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { Switch } from '@/components/ui/switch'

import type { ChannelFormValues } from '../lib/channel-form'

export function CacheWriteSimulationSetting(props: { disabled?: boolean }) {
  const { t } = useTranslation()
  const form = useFormContext<ChannelFormValues>()
  const enabled = useWatch({
    control: form.control,
    name: 'cache_write_simulation_enabled',
  })
  const ttlItems = [
    { value: '5m', label: t('5 minutes') },
    { value: '1h', label: t('1 hour') },
  ]

  return (
    <div className='space-y-4 rounded-lg border p-4'>
      <FormField
        control={form.control}
        name='cache_write_simulation_enabled'
        render={({ field }) => (
          <FormItem className='flex items-center justify-between gap-4'>
            <div className='space-y-1'>
              <FormLabel>{t('Simulate cache writes')}</FormLabel>
              <FormDescription>
                {t(
                  'For Chat Completions, Responses and Messages, reclassify uncached input when the upstream reports no cache writes. Responses, usage logs and billing use the configured TTL price. No upstream cache is created.'
                )}
              </FormDescription>
            </div>
            <FormControl>
              <Switch
                checked={field.value === true}
                onCheckedChange={field.onChange}
                onBlur={field.onBlur}
                name={field.name}
                ref={field.ref}
                disabled={props.disabled}
              />
            </FormControl>
          </FormItem>
        )}
      />
      {enabled && (
        <fieldset
          disabled={props.disabled}
          className='grid gap-4 sm:grid-cols-2'
        >
          <FormField
            control={form.control}
            name='cache_write_simulation_threshold'
            render={({ field }) => (
              <FormItem>
                <FormLabel>
                  {t('Cache write input threshold (tokens)')}
                </FormLabel>
                <FormControl>
                  <Input
                    type='number'
                    min={0}
                    max={2147483647}
                    step={1}
                    {...field}
                    value={field.value ?? ''}
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
                    'Activate only when total input tokens strictly exceed this threshold, including cache reads.'
                  )}
                </FormDescription>
                <FormMessage />
              </FormItem>
            )}
          />
          <FormField
            control={form.control}
            name='cache_write_simulation_percentage'
            render={({ field }) => (
              <FormItem>
                <FormLabel>
                  {t('Uncached input converted to cache writes (%)')}
                </FormLabel>
                <FormControl>
                  <Input
                    type='number'
                    min={0}
                    max={100}
                    step='any'
                    {...field}
                    value={field.value ?? ''}
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
                    'Round down the converted token count. Existing cache reads and writes are preserved.'
                  )}
                </FormDescription>
                <FormMessage />
              </FormItem>
            )}
          />
          <FormField
            control={form.control}
            name='cache_write_simulation_ttl'
            render={({ field }) => (
              <FormItem>
                <FormLabel>{t('Cache write TTL')}</FormLabel>
                <Select
                  items={ttlItems}
                  value={field.value ?? '5m'}
                  onValueChange={field.onChange}
                  disabled={props.disabled}
                >
                  <FormControl>
                    <SelectTrigger className='w-full'>
                      <SelectValue />
                    </SelectTrigger>
                  </FormControl>
                  <SelectContent>
                    {ttlItems.map((item) => (
                      <SelectItem key={item.value} value={item.value}>
                        {item.label}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
                <FormDescription>
                  {t(
                    'Uses the model cache write price for this TTL. Default: 5 minutes.'
                  )}
                </FormDescription>
                <FormMessage />
              </FormItem>
            )}
          />
        </fieldset>
      )}
    </div>
  )
}
