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
import { useFieldArray, useFormContext } from 'react-hook-form'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'
import {
  FormControl,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from '@/components/ui/form'
import { Input } from '@/components/ui/input'

import type { RateLimitDialogFormValues } from './rate-limit-dialog'

export function RateLimitModelRulesEditor() {
  const { t } = useTranslation()
  const form = useFormContext<RateLimitDialogFormValues>()
  const rules = useFieldArray({ control: form.control, name: 'models' })

  return (
    <section className='space-y-3 border-t pt-4' aria-label={t('Model rules')}>
      <div className='flex items-center justify-between gap-2'>
        <h3 className='text-sm font-medium'>{t('Model rules')}</h3>
        <Button
          type='button'
          variant='outline'
          size='sm'
          onClick={() => rules.append({ modelName: '', rpm: null, tpm: null })}
        >
          {t('Add model rule')}
        </Button>
      </div>
      <p className='text-muted-foreground text-xs'>
        {t(
          'Each user and model is counted separately. Empty fields inherit the group limits; 0 = unlimited.'
        )}
      </p>
      <p className='text-muted-foreground text-xs'>
        {t(
          'Use exact request model names. RPM overrides count all requests per minute; inherited request limits keep the group period and success limit.'
        )}
      </p>
      {rules.fields.map((rule, index) => (
        <div key={rule.id} className='space-y-2 rounded-lg border p-3'>
          <div className='grid gap-3 sm:grid-cols-3'>
            <FormField
              control={form.control}
              name={`models.${index}.modelName`}
              render={({ field }) => (
                <FormItem>
                  <FormLabel>
                    {t('Model')} {index + 1}
                  </FormLabel>
                  <FormControl>
                    <Input {...field} placeholder='gpt-5' />
                  </FormControl>
                  <FormMessage />
                </FormItem>
              )}
            />
            <FormField
              control={form.control}
              name={`models.${index}.rpm`}
              render={({ field }) => (
                <FormItem>
                  <FormLabel>
                    {t('RPM')} {index + 1}
                  </FormLabel>
                  <FormControl>
                    <Input
                      {...field}
                      type='number'
                      min={0}
                      max={2147483647}
                      step={1}
                      placeholder={t('Inherit group')}
                      value={field.value ?? ''}
                      onChange={(event) =>
                        field.onChange(
                          event.target.value === ''
                            ? null
                            : Number(event.target.value)
                        )
                      }
                    />
                  </FormControl>
                  <FormMessage />
                </FormItem>
              )}
            />
            <FormField
              control={form.control}
              name={`models.${index}.tpm`}
              render={({ field }) => (
                <FormItem>
                  <FormLabel>
                    {t('TPM')} {index + 1}
                  </FormLabel>
                  <FormControl>
                    <Input
                      {...field}
                      type='number'
                      min={0}
                      max={2147483647}
                      step={1}
                      placeholder={t('Inherit group')}
                      value={field.value ?? ''}
                      onChange={(event) =>
                        field.onChange(
                          event.target.value === ''
                            ? null
                            : Number(event.target.value)
                        )
                      }
                    />
                  </FormControl>
                  <FormMessage />
                </FormItem>
              )}
            />
          </div>
          <Button
            type='button'
            variant='ghost'
            size='sm'
            aria-label={t('Remove model rule {{index}}', { index: index + 1 })}
            onClick={() => rules.remove(index)}
          >
            {t('Remove')}
          </Button>
        </div>
      ))}
    </section>
  )
}
