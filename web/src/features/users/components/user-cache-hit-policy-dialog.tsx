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
import { useMutation, useQuery } from '@tanstack/react-query'
import { useState } from 'react'
import { useForm } from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { z } from 'zod'

import { Dialog } from '@/components/dialog'
import { ErrorState } from '@/components/error-state'
import { LoadingState } from '@/components/loading-state'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { Combobox } from '@/components/ui/combobox'
import {
  Form,
  FormControl,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from '@/components/ui/form'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Switch } from '@/components/ui/switch'
import { toIntlLocale } from '@/i18n/languages'
import { formatNumber } from '@/lib/format'
import { handleServerError } from '@/lib/handle-server-error'

import {
  getUserCacheHitPolicy,
  getUserChannelRouting,
  putUserCacheHitPolicy,
} from '../api'
import type { UserCacheHitPolicyConfig } from '../types'

export function UserCacheHitPolicyDialog(props: {
  userId: number
  username: string
  open: boolean
  onOpenChange: (open: boolean) => void
}) {
  const { t, i18n } = useTranslation()
  const locale = toIntlLocale(i18n.resolvedLanguage || i18n.language)
  const [model, setModel] = useState('')
  const models = useQuery({
    queryKey: ['user-cache-policy-models', props.userId],
    queryFn: () => getUserChannelRouting(props.userId),
    enabled: props.open,
  })
  const query = useQuery({
    queryKey: ['user-cache-hit-policy', props.userId, model],
    queryFn: () => getUserCacheHitPolicy(props.userId, model),
    enabled: props.open,
    refetchOnWindowFocus: false,
  })
  const config = query.data?.data
  const options = [
    ...new Set([
      ...(models.data?.data?.models ?? []),
      ...Object.keys(config?.policies ?? {}),
    ]),
  ]
    .sort()
    .map((value) => ({ value, label: value }))
  const daily = config?.daily

  return (
    <Dialog
      open={props.open}
      onOpenChange={props.onOpenChange}
      title={t('Cache billing policy')}
      description={props.username}
    >
      <div className='space-y-4'>
        <p className='text-muted-foreground text-sm'>
          {t(
            'Random daily cache ceiling; lower real hits stay unchanged. Removed cache reads are billed as ordinary input. Cache writes stay unchanged.'
          )}
        </p>
        <p className='text-muted-foreground text-sm'>
          {t(
            'Applies to text usage in Chat Completions, Messages and Responses. Multimodal cache breakdowns retain real usage.'
          )}
        </p>
        <Label htmlFor='cache-policy-model'>{t('Model')}</Label>
        <Combobox
          id='cache-policy-model'
          aria-label={t('Model')}
          options={options}
          value={model}
          onValueChange={(value) => setModel(value ?? '')}
          allowCustomValue
          placeholder={t('Select model')}
        />
        {query.isPending && <LoadingState />}
        {(query.isError || models.isError) && (
          <ErrorState
            onRetry={() => {
              void query.refetch()
              void models.refetch()
            }}
          />
        )}
        {config?.statistics_error && (
          <Alert>
            <AlertDescription>
              {t(
                'Daily statistics unavailable; cache billing uses real usage when Redis is unavailable'
              )}
            </AlertDescription>
          </Alert>
        )}
        {config && model && (
          <>
            <CacheHitPolicyForm
              key={`${model}:${config.revision}`}
              userId={props.userId}
              model={model}
              config={config}
              onSaved={() => void query.refetch()}
            />
            {daily && !config.statistics_error && (
              <dl className='grid grid-cols-2 gap-2 text-sm'>
                <dt>{t('Today (Beijing time)')}</dt>
                <dd>{daily.day}</dd>
                <dt>{t('Policy input tokens today')}</dt>
                <dd>{formatNumber(daily.input_tokens, locale)}</dd>
                <dt>{t('Real cache hit rate')}</dt>
                <dd>
                  {daily.input_tokens > 0
                    ? `${formatNumber((100 * daily.real_cache_tokens) / daily.input_tokens, locale)}%`
                    : '—'}
                </dd>
                <dt>{t('Billed cache hit rate')}</dt>
                <dd>
                  {daily.input_tokens > 0
                    ? `${formatNumber((100 * daily.bill_cache_tokens) / daily.input_tokens, locale)}%`
                    : '—'}
                </dd>
                <dt>{t('Cache reads converted to ordinary input')}</dt>
                <dd>
                  {formatNumber(
                    daily.real_cache_tokens - daily.bill_cache_tokens,
                    locale
                  )}
                </dd>
              </dl>
            )}
            <Button
              variant='outline'
              onClick={() => void query.refetch()}
              disabled={query.isFetching}
            >
              {t('Refresh')}
            </Button>
          </>
        )}
      </div>
    </Dialog>
  )
}

function CacheHitPolicyForm(props: {
  userId: number
  model: string
  config: UserCacheHitPolicyConfig
  onSaved: () => void
}) {
  const { t } = useTranslation()
  const policy = props.config.policies[props.model]
  const schema = z
    .object({
      enabled: z.boolean(),
      minimum: z.number().min(0).max(100),
      maximum: z.number().min(0).max(100),
    })
    .refine((value) => value.minimum <= value.maximum, {
      path: ['maximum'],
      message: t('Minimum must not exceed maximum'),
    })
  const form = useForm<z.infer<typeof schema>>({
    resolver: zodResolver(schema),
    defaultValues: {
      enabled: policy?.enabled ?? false,
      minimum: (policy?.min_bps ?? 4000) / 100,
      maximum: (policy?.max_bps ?? 6000) / 100,
    },
  })
  const save = useMutation({
    mutationFn: (values: z.infer<typeof schema>) =>
      putUserCacheHitPolicy(props.userId, {
        model: props.model,
        revision: props.config.revision,
        enabled: values.enabled,
        min_bps: Math.round(values.minimum * 100),
        max_bps: Math.round(values.maximum * 100),
      }),
    onSuccess: () => {
      toast.success(t('Saved successfully'))
      props.onSaved()
    },
    onError: (error) => handleServerError(error, t('Failed to save')),
  })
  return (
    <Form {...form}>
      <form
        onSubmit={form.handleSubmit((values) => save.mutate(values))}
        className='space-y-4'
      >
        <FormField
          control={form.control}
          name='enabled'
          render={({ field }) => (
            <FormItem className='flex items-center justify-between'>
              <FormLabel>{t('Enable cache billing policy')}</FormLabel>
              <FormControl>
                <Switch
                  checked={field.value}
                  onCheckedChange={field.onChange}
                  disabled={save.isPending}
                />
              </FormControl>
              <FormMessage />
            </FormItem>
          )}
        />
        <div className='grid grid-cols-2 gap-4'>
          {(['minimum', 'maximum'] as const).map((name) => (
            <FormField
              key={name}
              control={form.control}
              name={name}
              render={({ field }) => (
                <FormItem>
                  <FormLabel>
                    {name === 'minimum'
                      ? t('Random target minimum (%)')
                      : t('Random target maximum (%)')}
                  </FormLabel>
                  <FormControl>
                    <Input
                      type='number'
                      min={0}
                      max={100}
                      step={0.01}
                      {...field}
                      onChange={(event) =>
                        field.onChange(
                          event.target.value === ''
                            ? Number.NaN
                            : Number(event.target.value)
                        )
                      }
                      disabled={save.isPending}
                    />
                  </FormControl>
                  <FormMessage />
                </FormItem>
              )}
            />
          ))}
        </div>
        <Button type='submit' disabled={save.isPending}>
          {t('Save')}
        </Button>
      </form>
    </Form>
  )
}
