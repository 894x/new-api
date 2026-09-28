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
import { Loader2, Plus, Trash2 } from 'lucide-react'
import { useEffect, useMemo } from 'react'
import { useFieldArray, useForm } from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { z } from 'zod'

import {
  SideDrawerSection,
  SideDrawerSectionHeader,
  sideDrawerContentClassName,
  sideDrawerFooterClassName,
  sideDrawerFormClassName,
  sideDrawerHeaderClassName,
} from '@/components/drawer-layout'
import { MultiSelect } from '@/components/multi-select'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { Combobox } from '@/components/ui/combobox'
import {
  Form,
  FormControl,
  FormDescription,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from '@/components/ui/form'
import { Input } from '@/components/ui/input'
import {
  Sheet,
  SheetClose,
  SheetContent,
  SheetDescription,
  SheetFooter,
  SheetHeader,
  SheetTitle,
} from '@/components/ui/sheet'
import { Switch } from '@/components/ui/switch'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { RateLimitModelRulesEditor } from '@/features/system-settings/request-limits/rate-limit-model-rules-editor'
import { MAX_REQUEST_RATE_LIMIT } from '@/features/system-settings/request-limits/rate-limit-validation'
import { handleServerError } from '@/lib/handle-server-error'
import { requireServerSuccess } from '@/lib/server-error-message'

import { getUserGroupConfig, updateUserGroupConfig } from '../api'
import type { User, UserGroupConfig } from '../types'
import { useUsers } from './users-provider'

const createSchema = (t: (key: string) => string) =>
  z
    .object({
      discounts: z.array(
        z.object({
          targetGroup: z.string().min(1, t('Value is required')),
          ratio: z.number().min(0, t('Must be ≥ 0')),
        })
      ),
      rateLimitEnabled: z.boolean(),
      maxRequests: z.number().int().min(0).max(MAX_REQUEST_RATE_LIMIT),
      maxSuccess: z.number().int().min(1).max(MAX_REQUEST_RATE_LIMIT),
      maxTPM: z.number().int().min(0).max(2147483647),
      models: z.array(
        z.object({
          modelName: z.string().trim().min(1, t('Value is required')),
          rpm: z.number().int().min(0).max(MAX_REQUEST_RATE_LIMIT).nullable(),
          tpm: z.number().int().min(0).max(2147483647).nullable(),
        })
      ),
      channelRules: z.array(
        z.object({
          modelName: z.string().trim().min(1, t('Value is required')),
          groups: z.array(z.string()),
        })
      ),
    })
    .superRefine((values, context) => {
      const discountGroups = new Set<string>()
      values.discounts.forEach((entry, index) => {
        if (discountGroups.has(entry.targetGroup)) {
          context.addIssue({
            code: 'custom',
            path: ['discounts', index, 'targetGroup'],
            message: t('Values must be unique'),
          })
        }
        discountGroups.add(entry.targetGroup)
      })
      const rateLimitModels = new Set<string>()
      values.models.forEach((entry, index) => {
        if (rateLimitModels.has(entry.modelName)) {
          context.addIssue({
            code: 'custom',
            path: ['models', index, 'modelName'],
            message: t('Values must be unique'),
          })
        }
        rateLimitModels.add(entry.modelName)
      })
      const channelModels = new Set<string>()
      values.channelRules.forEach((entry, index) => {
        if (channelModels.has(entry.modelName)) {
          context.addIssue({
            code: 'custom',
            path: ['channelRules', index, 'modelName'],
            message: t('Values must be unique'),
          })
        }
        channelModels.add(entry.modelName)
      })
    })

type FormValues = z.infer<ReturnType<typeof createSchema>>

const emptyValues: FormValues = {
  discounts: [],
  rateLimitEnabled: false,
  maxRequests: 0,
  maxSuccess: 1,
  maxTPM: 0,
  models: [],
  channelRules: [],
}

function toFormValues(config: UserGroupConfig): FormValues {
  return {
    discounts: Object.entries(config.discounts).map(([targetGroup, ratio]) => ({
      targetGroup,
      ratio,
    })),
    rateLimitEnabled: config.rate_limit_enabled,
    maxRequests: config.rate_limit.limits[0],
    maxSuccess: config.rate_limit.limits[1],
    maxTPM: config.rate_limit.limits[2],
    models: Object.entries(config.rate_limit.models).map(
      ([modelName, limits]) => ({
        modelName,
        rpm: limits.rpm ?? null,
        tpm: limits.tpm ?? null,
      })
    ),
    channelRules: Object.entries(config.model_channel_groups).map(
      ([modelName, groups]) => ({ modelName, groups })
    ),
  }
}

type Props = {
  open: boolean
  onOpenChange: (open: boolean) => void
  user?: User
}

export function UserGroupConfigDrawer({ open, onOpenChange, user }: Props) {
  const { t } = useTranslation()
  const { triggerRefresh } = useUsers()
  const form = useForm<FormValues>({
    resolver: zodResolver(createSchema(t)),
    defaultValues: emptyValues,
  })
  const discounts = useFieldArray({ control: form.control, name: 'discounts' })
  const channelRules = useFieldArray({
    control: form.control,
    name: 'channelRules',
  })

  const query = useQuery({
    queryKey: ['user-group-config', user?.id],
    queryFn: async () => {
      if (!user) throw new Error('User is required')
      return requireServerSuccess(await getUserGroupConfig(user.id))
    },
    enabled: open && !!user,
  })
  const config = query.data?.data

  useEffect(() => {
    if (open && config) form.reset(toFormValues(config))
    if (!open) form.reset(emptyValues)
  }, [config, form, open])

  const groupOptions = useMemo(() => {
    const names = new Set(Object.keys(config?.available_group_ratios ?? {}))
    for (const item of form.getValues('discounts')) names.add(item.targetGroup)
    return [...names]
      .sort((left, right) => left.localeCompare(right))
      .map((name) => ({ value: name, label: name }))
  }, [config, form])

  const save = useMutation({
    mutationFn: async (values: FormValues) => {
      if (!user) throw new Error('User is required')
      if (!config) throw new Error('Configuration is required')
      const result = await updateUserGroupConfig(user.id, {
        group: config.group,
        discounts: Object.fromEntries(
          values.discounts.map((item) => [item.targetGroup, item.ratio])
        ),
        rate_limit_enabled: values.rateLimitEnabled,
        rate_limit: {
          limits: [values.maxRequests, values.maxSuccess, values.maxTPM],
          models: Object.fromEntries(
            values.models.map((item) => [
              item.modelName,
              {
                ...(item.rpm === null ? {} : { rpm: item.rpm }),
                ...(item.tpm === null ? {} : { tpm: item.tpm }),
              },
            ])
          ),
        },
        model_channel_groups: Object.fromEntries(
          values.channelRules.map((item) => [item.modelName, item.groups])
        ),
      })
      return requireServerSuccess(result)
    },
    onSuccess: () => {
      toast.success(t('Customer group configuration saved'))
      triggerRefresh()
      onOpenChange(false)
    },
    onError: (error) => handleServerError(error, t('Failed to save')),
  })

  return (
    <Sheet open={open} onOpenChange={onOpenChange}>
      <SheetContent className={sideDrawerContentClassName('sm:max-w-[760px]')}>
        <SheetHeader className={sideDrawerHeaderClassName()}>
          <SheetTitle>{t('Customer group configuration')}</SheetTitle>
          <SheetDescription>
            {config
              ? t(
                  'Configure policies for {{username}} using user group {{group}}.',
                  {
                    username: config.username,
                    group: config.group,
                  }
                )
              : t('Configure discounts, rate limits, and model channel pools.')}
          </SheetDescription>
        </SheetHeader>

        {query.isPending && (
          <div className='flex min-h-0 flex-1 items-center justify-center'>
            <Loader2 className='text-muted-foreground size-6 animate-spin' />
          </div>
        )}
        {query.isError && !query.isPending && (
          <div className='flex min-h-0 flex-1 flex-col items-center justify-center gap-3 p-6'>
            <p className='text-destructive text-sm'>{t('Failed to load')}</p>
            <Button variant='outline' onClick={() => query.refetch()}>
              {t('Retry')}
            </Button>
          </div>
        )}
        {!query.isPending && !query.isError && (
          <Form {...form}>
            <form
              id='user-group-config-form'
              className={sideDrawerFormClassName()}
              onSubmit={form.handleSubmit((values) => save.mutate(values))}
            >
              <Alert>
                <AlertDescription>
                  {t(
                    'These policies apply to every user in group {{group}}. Assign an exclusive group before using this page for one customer.',
                    { group: config?.group ?? user?.group ?? '' }
                  )}
                </AlertDescription>
              </Alert>
              <Tabs defaultValue='discounts'>
                <TabsList className='grid w-full grid-cols-3'>
                  <TabsTrigger value='discounts'>{t('Discounts')}</TabsTrigger>
                  <TabsTrigger value='rate-limits'>
                    {t('Rate limits')}
                  </TabsTrigger>
                  <TabsTrigger value='channels'>{t('Channels')}</TabsTrigger>
                </TabsList>

                <TabsContent value='discounts' className='pt-5'>
                  <SideDrawerSection>
                    <SideDrawerSectionHeader
                      title={t('Specified group discounts')}
                      description={t(
                        'Set the ratio charged when this user group uses each billing group. Unconfigured billing groups keep their base ratio.'
                      )}
                    />
                    {discounts.fields.map((item, index) => (
                      <div
                        key={item.id}
                        className='grid gap-3 rounded-lg border p-3 sm:grid-cols-[1fr_10rem_auto] sm:items-end'
                      >
                        <FormField
                          control={form.control}
                          name={`discounts.${index}.targetGroup`}
                          render={({ field }) => (
                            <FormItem>
                              <FormLabel>{t('Billing group')}</FormLabel>
                              <FormControl>
                                <Combobox
                                  options={groupOptions}
                                  value={field.value || null}
                                  onValueChange={(value) =>
                                    field.onChange(value ?? '')
                                  }
                                  placeholder={t('Select a group')}
                                />
                              </FormControl>
                              <FormMessage />
                            </FormItem>
                          )}
                        />
                        <FormField
                          control={form.control}
                          name={`discounts.${index}.ratio`}
                          render={({ field }) => (
                            <FormItem>
                              <FormLabel>{t('Ratio')}</FormLabel>
                              <FormControl>
                                <Input
                                  type='number'
                                  min={0}
                                  step={0.0001}
                                  {...field}
                                  onChange={(event) =>
                                    field.onChange(Number(event.target.value))
                                  }
                                />
                              </FormControl>
                              <FormMessage />
                            </FormItem>
                          )}
                        />
                        <Button
                          type='button'
                          variant='ghost'
                          size='icon'
                          aria-label={t('Remove')}
                          onClick={() => discounts.remove(index)}
                        >
                          <Trash2 />
                        </Button>
                      </div>
                    ))}
                    <Button
                      type='button'
                      variant='outline'
                      onClick={() =>
                        discounts.append({ targetGroup: '', ratio: 1 })
                      }
                    >
                      <Plus /> {t('Add ratio override')}
                    </Button>
                  </SideDrawerSection>
                </TabsContent>

                <TabsContent value='rate-limits' className='pt-5'>
                  <SideDrawerSection>
                    <SideDrawerSectionHeader
                      title={t('Group-based rate limits')}
                      description={t(
                        'These limits match the user group, even when an API key selects a different channel group.'
                      )}
                    />
                    {config && !config.global_rate_limit_enabled && (
                      <Alert>
                        <AlertDescription>
                          {t(
                            'Global model request rate limiting is disabled. These rules will take effect after it is enabled in system settings.'
                          )}
                        </AlertDescription>
                      </Alert>
                    )}
                    <FormField
                      control={form.control}
                      name='rateLimitEnabled'
                      render={({ field }) => (
                        <FormItem className='flex items-center justify-between gap-4 rounded-lg border p-3'>
                          <div>
                            <FormLabel>
                              {t('Enable group rate limit')}
                            </FormLabel>
                            <FormDescription>
                              {t('Turn off to inherit the global rate limits.')}
                            </FormDescription>
                          </div>
                          <FormControl>
                            <Switch
                              checked={field.value}
                              onCheckedChange={field.onChange}
                            />
                          </FormControl>
                        </FormItem>
                      )}
                    />
                    <div className='grid gap-4 sm:grid-cols-3'>
                      {(
                        [
                          ['maxRequests', 'Max requests per period'],
                          ['maxSuccess', 'Max successful requests'],
                          ['maxTPM', 'Tokens per minute'],
                        ] as const
                      ).map(([name, label]) => (
                        <FormField
                          key={name}
                          control={form.control}
                          name={name}
                          render={({ field }) => (
                            <FormItem>
                              <FormLabel>{t(label)}</FormLabel>
                              <FormControl>
                                <Input
                                  type='number'
                                  min={name === 'maxSuccess' ? 1 : 0}
                                  step={1}
                                  disabled={!form.watch('rateLimitEnabled')}
                                  {...field}
                                  onChange={(event) =>
                                    field.onChange(Number(event.target.value))
                                  }
                                />
                              </FormControl>
                              <FormMessage />
                            </FormItem>
                          )}
                        />
                      ))}
                    </div>
                    <fieldset disabled={!form.watch('rateLimitEnabled')}>
                      <RateLimitModelRulesEditor />
                    </fieldset>
                  </SideDrawerSection>
                </TabsContent>

                <TabsContent value='channels' className='pt-5'>
                  <SideDrawerSection>
                    <SideDrawerSectionHeader
                      title={t('Model channel pools')}
                      description={t(
                        'Choose channel groups for each model. No rule keeps existing routing; an empty selection blocks that model.'
                      )}
                    />
                    {channelRules.fields.map((item, index) => (
                      <div
                        key={item.id}
                        className='space-y-3 rounded-lg border p-3'
                      >
                        <div className='flex items-start gap-2'>
                          <FormField
                            control={form.control}
                            name={`channelRules.${index}.modelName`}
                            render={({ field }) => (
                              <FormItem className='flex-1'>
                                <FormLabel>{t('Model')}</FormLabel>
                                <FormControl>
                                  <Input {...field} placeholder='gpt-5' />
                                </FormControl>
                                <FormMessage />
                              </FormItem>
                            )}
                          />
                          <Button
                            type='button'
                            variant='ghost'
                            size='icon'
                            className='mt-6'
                            aria-label={t('Remove')}
                            onClick={() => channelRules.remove(index)}
                          >
                            <Trash2 />
                          </Button>
                        </div>
                        <FormField
                          control={form.control}
                          name={`channelRules.${index}.groups`}
                          render={({ field }) => (
                            <FormItem>
                              <FormLabel>{t('Channel groups')}</FormLabel>
                              <FormControl>
                                <MultiSelect
                                  options={groupOptions}
                                  selected={field.value}
                                  onChange={field.onChange}
                                  allowCreate
                                  placeholder={t('Select channel groups')}
                                />
                              </FormControl>
                              <FormMessage />
                            </FormItem>
                          )}
                        />
                      </div>
                    ))}
                    <Button
                      type='button'
                      variant='outline'
                      onClick={() =>
                        channelRules.append({ modelName: '', groups: [] })
                      }
                    >
                      <Plus /> {t('Add rule')}
                    </Button>
                  </SideDrawerSection>
                </TabsContent>
              </Tabs>
            </form>
          </Form>
        )}

        <SheetFooter className={sideDrawerFooterClassName()}>
          <SheetClose render={<Button variant='outline' />}>
            {t('Close')}
          </SheetClose>
          <Button
            type='submit'
            form='user-group-config-form'
            disabled={!config || save.isPending}
          >
            {save.isPending ? t('Saving...') : t('Save changes')}
          </Button>
        </SheetFooter>
      </SheetContent>
    </Sheet>
  )
}
