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
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { AlertCircle, Library, Loader2, RefreshCw, Trash2 } from 'lucide-react'
import { useEffect, useState } from 'react'
import { useForm } from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { ConfirmDialog } from '@/components/confirm-dialog'
import {
  SideDrawerSection,
  SideDrawerSectionHeader,
  sideDrawerSwitchItemClassName,
} from '@/components/drawer-layout'
import { PasswordInput } from '@/components/password-input'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
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
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { Switch } from '@/components/ui/switch'
import {
  deleteChannelAssetLibraryConfig,
  getChannelAssetLibraryConfig,
  saveChannelAssetLibraryConfig,
  syncChannelAssetLibrary,
} from '@/features/asset-library/api'
import {
  assetLibraryQueryKeys,
  channelAssetConfigDestinationChanged,
  channelAssetConfigFormSchema,
  getAssetLibraryErrorMessage,
  getChannelAssetConfigFormValues,
  getChannelAssetConfigPayload,
  type ChannelAssetConfigFormValues,
} from '@/features/asset-library/lib'

export function ChannelAssetLibrarySection(props: {
  channelId: number | null
  disabled?: boolean
}) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const [removeConfirmOpen, setRemoveConfirmOpen] = useState(false)
  const queryKey = props.channelId
    ? assetLibraryQueryKeys.channelConfig(props.channelId)
    : assetLibraryQueryKeys.all
  const { data: config, isLoading } = useQuery({
    queryKey,
    queryFn: () => getChannelAssetLibraryConfig(props.channelId || 0),
    enabled: !!props.channelId,
  })
  const form = useForm<ChannelAssetConfigFormValues>({
    resolver: zodResolver(channelAssetConfigFormSchema),
    defaultValues: getChannelAssetConfigFormValues(null),
  })
  const authType = form.watch('authType')
  const baseUrl = form.watch('baseUrl')
  const enabled = form.watch('enabled')
  const canReuseStoredCredential = !channelAssetConfigDestinationChanged(
    { authType, baseUrl },
    config ?? null
  )
  const hasStoredCredential = Boolean(
    config?.has_access_key || config?.has_secret_key || config?.has_api_key
  )
  const hasConfiguration = Boolean(
    hasStoredCredential || config?.enabled || (config?.replica_count ?? 0) > 0
  )

  useEffect(() => {
    if (!props.channelId || isLoading) return
    form.reset(getChannelAssetConfigFormValues(config ?? null))
  }, [config, form, isLoading, props.channelId])

  const saveMutation = useMutation({
    mutationFn: (values: ChannelAssetConfigFormValues) =>
      saveChannelAssetLibraryConfig(
        props.channelId || 0,
        getChannelAssetConfigPayload(values)
      ),
    onSuccess: async (savedConfig) => {
      queryClient.setQueryData(queryKey, savedConfig)
      form.reset(getChannelAssetConfigFormValues(savedConfig))
      await queryClient.invalidateQueries({ queryKey })
      toast.success(t('Asset library configuration saved'))
    },
    onError: (error) =>
      toast.error(
        getAssetLibraryErrorMessage(
          error,
          t('Failed to save asset library configuration')
        )
      ),
  })
  const syncMutation = useMutation({
    mutationFn: () => syncChannelAssetLibrary(props.channelId || 0),
    onSuccess: async () => {
      await queryClient.invalidateQueries({
        queryKey: assetLibraryQueryKeys.all,
      })
      toast.success(t('Asset library synchronization started'))
    },
    onError: (error) =>
      toast.error(
        getAssetLibraryErrorMessage(
          error,
          t('Failed to synchronize asset library')
        )
      ),
  })
  const deleteMutation = useMutation({
    mutationFn: () => deleteChannelAssetLibraryConfig(props.channelId || 0),
    onSuccess: async () => {
      form.reset(getChannelAssetConfigFormValues(null))
      await queryClient.invalidateQueries({ queryKey })
      setRemoveConfirmOpen(false)
      toast.success(t('Asset library configuration removed'))
    },
    onError: (error) =>
      toast.error(
        getAssetLibraryErrorMessage(
          error,
          t('Failed to remove asset library configuration')
        )
      ),
  })

  let content
  if (!props.channelId) {
    content = (
      <Alert>
        <AlertCircle />
        <AlertDescription>
          {t('Save the channel before configuring its asset library.')}
        </AlertDescription>
      </Alert>
    )
  } else if (isLoading) {
    content = (
      <div className='text-muted-foreground flex items-center gap-2 py-6 text-sm'>
        <Loader2 className='size-4 animate-spin' />
        {t('Loading asset library configuration...')}
      </div>
    )
  } else {
    content = (
      <Form {...form}>
        <div className='space-y-5'>
          {props.disabled && (
            <Alert>
              <AlertDescription>
                {t(
                  'Sensitive channel settings are read-only for your account.'
                )}
              </AlertDescription>
            </Alert>
          )}

          <fieldset
            disabled={props.disabled || saveMutation.isPending}
            className='space-y-5 disabled:opacity-60'
          >
            <FormField
              control={form.control}
              name='enabled'
              render={({ field }) => (
                <FormItem className={sideDrawerSwitchItemClassName()}>
                  <div className='space-y-0.5'>
                    <FormLabel>{t('Enable asset library')}</FormLabel>
                    <FormDescription>
                      {t(
                        'New and existing user assets are synchronized to this channel automatically.'
                      )}
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

            <div className='grid gap-4 sm:grid-cols-2'>
              <FormField
                control={form.control}
                name='baseUrl'
                render={({ field }) => (
                  <FormItem className='sm:col-span-2'>
                    <FormLabel>{t('Asset service base URL')}</FormLabel>
                    <FormControl>
                      <Input {...field} inputMode='url' />
                    </FormControl>
                    <FormDescription>
                      {t(
                        'Use the official Volcengine endpoint or a compatible upstream service.'
                      )}
                    </FormDescription>
                    <FormMessage />
                  </FormItem>
                )}
              />
              <FormField
                control={form.control}
                name='authType'
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>{t('Authentication type')}</FormLabel>
                    <Select
                      items={[
                        { value: 'aksk', label: t('Volcengine AK/SK') },
                        { value: 'bearer', label: t('Single API key') },
                      ]}
                      value={field.value}
                      onValueChange={(value) => value && field.onChange(value)}
                    >
                      <FormControl>
                        <SelectTrigger className='w-full'>
                          <SelectValue />
                        </SelectTrigger>
                      </FormControl>
                      <SelectContent alignItemWithTrigger={false}>
                        <SelectGroup>
                          <SelectItem value='aksk'>
                            {t('Volcengine AK/SK')}
                          </SelectItem>
                          <SelectItem value='bearer'>
                            {t('Single API key')}
                          </SelectItem>
                        </SelectGroup>
                      </SelectContent>
                    </Select>
                    <FormMessage />
                  </FormItem>
                )}
              />
              <FormField
                control={form.control}
                name='projectName'
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>{t('Project')}</FormLabel>
                    <FormControl>
                      <Input {...field} placeholder='default' />
                    </FormControl>
                    <FormMessage />
                  </FormItem>
                )}
              />

              {authType === 'aksk' ? (
                <>
                  <FormField
                    control={form.control}
                    name='accessKey'
                    render={({ field }) => (
                      <FormItem>
                        <FormLabel>{t('Access Key (AK)')}</FormLabel>
                        <FormControl>
                          <PasswordInput
                            {...field}
                            autoComplete='new-password'
                            placeholder={
                              canReuseStoredCredential && config?.has_access_key
                                ? t('Stored; leave blank to keep')
                                : t('Enter Access Key')
                            }
                          />
                        </FormControl>
                        <FormMessage />
                      </FormItem>
                    )}
                  />
                  <FormField
                    control={form.control}
                    name='secretKey'
                    render={({ field }) => (
                      <FormItem>
                        <FormLabel>{t('Secret Key (SK)')}</FormLabel>
                        <FormControl>
                          <PasswordInput
                            {...field}
                            autoComplete='new-password'
                            placeholder={
                              canReuseStoredCredential && config?.has_secret_key
                                ? t('Stored; leave blank to keep')
                                : t('Enter Secret Key')
                            }
                          />
                        </FormControl>
                        <FormMessage />
                      </FormItem>
                    )}
                  />
                  <FormField
                    control={form.control}
                    name='region'
                    render={({ field }) => (
                      <FormItem>
                        <FormLabel>{t('Region')}</FormLabel>
                        <FormControl>
                          <Input {...field} placeholder='cn-beijing' />
                        </FormControl>
                        <FormMessage />
                      </FormItem>
                    )}
                  />
                </>
              ) : (
                <FormField
                  control={form.control}
                  name='apiKey'
                  render={({ field }) => (
                    <FormItem className='sm:col-span-2'>
                      <FormLabel>{t('API Key')}</FormLabel>
                      <FormControl>
                        <PasswordInput
                          {...field}
                          autoComplete='new-password'
                          placeholder={
                            canReuseStoredCredential && config?.has_api_key
                              ? t('Stored; leave blank to keep')
                              : t('Enter API key')
                          }
                        />
                      </FormControl>
                      <FormDescription>
                        {t(
                          'For compatible upstreams that authenticate with one key.'
                        )}
                      </FormDescription>
                      <FormMessage />
                    </FormItem>
                  )}
                />
              )}
            </div>
          </fieldset>

          <div className='flex flex-wrap justify-end gap-2'>
            {hasConfiguration && (
              <Button
                type='button'
                variant='outline'
                disabled={props.disabled}
                onClick={() => setRemoveConfirmOpen(true)}
              >
                <Trash2 />
                {t('Remove configuration')}
              </Button>
            )}
            <Button
              type='button'
              variant='outline'
              disabled={props.disabled || !enabled || syncMutation.isPending}
              onClick={() => syncMutation.mutate()}
            >
              {syncMutation.isPending ? (
                <Loader2 className='animate-spin' />
              ) : (
                <RefreshCw />
              )}
              {t('Synchronize now')}
            </Button>
            <Button
              type='button'
              disabled={props.disabled || saveMutation.isPending}
              onClick={form.handleSubmit((values) => {
                if (
                  channelAssetConfigDestinationChanged(
                    values,
                    config ?? null
                  ) &&
                  values.enabled
                ) {
                  if (values.authType === 'aksk') {
                    if (!values.accessKey.trim()) {
                      form.setError('accessKey', {
                        message: t('Access Key is required'),
                      })
                    }
                    if (!values.secretKey.trim()) {
                      form.setError('secretKey', {
                        message: t('Secret Key is required'),
                      })
                    }
                    if (!values.accessKey.trim() || !values.secretKey.trim()) {
                      return
                    }
                  } else if (!values.apiKey.trim()) {
                    form.setError('apiKey', {
                      message: t('API key is required'),
                    })
                    return
                  }
                }
                saveMutation.mutate(values)
              })}
            >
              {saveMutation.isPending && <Loader2 className='animate-spin' />}
              {t('Save asset library')}
            </Button>
          </div>

          <ConfirmDialog
            open={removeConfirmOpen}
            onOpenChange={setRemoveConfirmOpen}
            title={t('Remove asset library configuration?')}
            desc={t(
              "This removes the credentials and this channel's asset replicas. The user asset library remains available on other channels."
            )}
            confirmText={t('Remove')}
            destructive
            isLoading={deleteMutation.isPending}
            handleConfirm={() => deleteMutation.mutate()}
          />
        </div>
      </Form>
    )
  }

  return (
    <SideDrawerSection>
      <SideDrawerSectionHeader
        title={t('Asset Library')}
        description={t(
          'Configure the upstream asset service used by this video channel.'
        )}
        icon={<Library className='h-4 w-4' aria-hidden='true' />}
        iconTone='chart-4'
      />
      {content}
    </SideDrawerSection>
  )
}
