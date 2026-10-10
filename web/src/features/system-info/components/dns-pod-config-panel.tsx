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
import {
  useIsMutating,
  useMutation,
  useQuery,
  useQueryClient,
} from '@tanstack/react-query'
import { useState, type ReactNode } from 'react'
import { useForm } from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { ConfirmDialog } from '@/components/confirm-dialog'
import { ErrorState } from '@/components/error-state'
import { PasswordInput } from '@/components/password-input'
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
import { NativeSelect, NativeSelectOption } from '@/components/ui/native-select'
import { Skeleton } from '@/components/ui/skeleton'
import { Switch } from '@/components/ui/switch'
import {
  SecureVerificationDialog,
  useSecureVerification,
} from '@/features/auth/secure-verification'
import { AuthOperationError } from '@/lib/secure-verification'

import { getDNSPodConfig, updateDNSPodConfig } from '../api'
import {
  dnsPodConfigChangeHash,
  dnsPodConfigSchema,
  type DNSPodConfigForm,
} from '../lib/dns-traffic'
import type { DNSPodConfigState, DNSPodConfigUpdate } from '../types'

const queryKey = ['system-info', 'dns-traffic', 'config']
const mutationKey = ['system-info', 'dns-traffic']

export function DNSPodConfigPanel() {
  const { t } = useTranslation()
  const updating = useIsMutating({ mutationKey }) > 0
  const query = useQuery({
    queryKey,
    queryFn: getDNSPodConfig,
    retry: false,
    refetchOnWindowFocus: false,
    refetchOnReconnect: false,
  })
  let content: ReactNode
  if (query.isPending) {
    content = <Skeleton className='h-24 w-full' />
  } else if (query.isError) {
    content = (
      <ErrorState
        description={t(
          'Unable to load DNSPod configuration. Check the shared database and server encryption key.'
        )}
        onRetry={() => void query.refetch()}
      />
    )
  } else {
    content = (
      <DNSPodConfigEditor
        key={query.data.revision}
        state={query.data}
        refreshing={query.isFetching}
      />
    )
  }
  return (
    <div className='space-y-3 border-t pt-4'>
      <div className='flex items-center justify-between gap-3'>
        <h3 className='font-medium'>{t('DNSPod configuration')}</h3>
        <Button
          size='sm'
          variant='outline'
          disabled={query.isFetching || updating}
          onClick={() => void query.refetch()}
        >
          {t('Refresh configuration')}
        </Button>
      </div>
      {content}
    </div>
  )
}

function DNSPodConfigEditor({
  state,
  refreshing,
}: {
  state: DNSPodConfigState
  refreshing: boolean
}) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const verification = useSecureVerification()
  const updating = useIsMutating({ mutationKey }) > 0
  const [confirming, setConfirming] = useState(false)
  const [failure, setFailure] = useState<string | null>(null)
  const defaults: DNSPodConfigUpdate = {
    revision: state.revision,
    enabled: state.enabled,
    domain: state.domain,
    subdomain: state.subdomain,
    record_line: state.record_line,
    record_type: state.record_type,
    secret_id: '',
    secret_key: '',
    session_token: '',
    clear_session_token: false,
  }
  const form = useForm<DNSPodConfigForm>({
    resolver: zodResolver(dnsPodConfigSchema(t, state)),
    defaultValues: defaults,
  })
  const credentialFields = [
    {
      name: 'secret_id',
      label: t('SecretId'),
      configured: state.secret_id_set,
    },
    {
      name: 'secret_key',
      label: t('SecretKey'),
      configured: state.secret_key_set,
    },
    {
      name: 'session_token',
      label: t('DNSPod session token (optional)'),
      configured: state.session_token_set,
    },
  ] as const
  const save = useMutation({
    mutationKey,
    gcTime: 0,
    retry: false,
    mutationFn: async (update: DNSPodConfigUpdate) => {
      const changeHash = await dnsPodConfigChangeHash(update)
      const proof = await verification.requestVerification({
        scope: 'dns.config.update',
        context: { revision: update.revision, change_hash: changeHash },
        title: t('Verify DNSPod configuration changes'),
      })
      if (!proof) return null
      return updateDNSPodConfig(update, proof.proof_token)
    },
    onSuccess: (result) => {
      if (!result) return
      queryClient.setQueryData(queryKey, result)
      void queryClient.invalidateQueries({ queryKey: mutationKey, exact: true })
      toast.success(
        t('DNSPod configuration saved. DNS records were not changed.')
      )
    },
    onError: (error) => {
      const code = AuthOperationError.from(error).code
      let message = t(
        'Unable to save DNSPod configuration. Refresh configuration and review server settings before trying again.'
      )
      if (code === 'DNS_CONFIG_CONFLICT') {
        message = t(
          'DNSPod configuration changed. Refresh and review it before saving again.'
        )
      } else if (code === 'DNS_CONFIG_INPUT_INVALID') {
        message = t(
          'DNSPod configuration is invalid or credentials are missing.'
        )
      } else if (code === 'DNS_BUSY') {
        message = t(
          'Another DNS update is in progress. Refresh records and try again later.'
        )
      }
      setFailure(message)
    },
    onSettled: () => {
      form.setValue('secret_id', '')
      form.setValue('secret_key', '')
      form.setValue('session_token', '')
      // Remove credentials from the mutation cache after every attempt.
      save.reset()
    },
  })
  const locked = updating || confirming || refreshing
  return (
    <>
      <p className='text-muted-foreground text-sm'>
        {t(
          'Configuration is shared through the database. Saved credentials are encrypted and never displayed; leave credential fields blank to keep them. Saving configuration does not change DNS records.'
        )}
      </p>
      {state.source === 'environment' && state.enabled && (
        <p className='text-muted-foreground text-sm'>
          {t(
            'Using server environment configuration until the first dashboard save.'
          )}
        </p>
      )}
      {!state.can_save && (
        <p role='alert' className='text-destructive text-sm'>
          {t(
            state.blocked_reason === 'DNS_ENCRYPTION_KEY_REQUIRED'
              ? 'Set the same stable CRYPTO_SECRET or SESSION_SECRET of at least 32 characters on every instance before saving DNSPod credentials.'
              : 'A shared Redis connection is required to apply DNS weights.'
          )}
        </p>
      )}
      {failure && (
        <p role='alert' className='text-destructive text-sm'>
          {failure}
        </p>
      )}
      <Form {...form}>
        <form
          className='space-y-4'
          onSubmit={form.handleSubmit(() => {
            setFailure(null)
            setConfirming(true)
          })}
        >
          <fieldset disabled={locked} className='space-y-4 disabled:opacity-60'>
            <FormField
              control={form.control}
              name='enabled'
              render={({ field }) => (
                <FormItem className='flex items-center justify-between'>
                  <FormLabel>{t('Enable DNSPod management')}</FormLabel>
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
                name='domain'
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>{t('DNSPod domain')}</FormLabel>
                    <FormControl>
                      <Input
                        {...field}
                        placeholder='xunlitec.com'
                        autoComplete='off'
                      />
                    </FormControl>
                    <FormMessage />
                  </FormItem>
                )}
              />
              <FormField
                control={form.control}
                name='subdomain'
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>{t('DNSPod hostname')}</FormLabel>
                    <FormControl>
                      <Input
                        {...field}
                        placeholder='newapi'
                        autoComplete='off'
                      />
                    </FormControl>
                    <FormDescription>
                      {t('Use @ for the root domain.')}
                    </FormDescription>
                    <FormMessage />
                  </FormItem>
                )}
              />
              <FormField
                control={form.control}
                name='record_line'
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>{t('DNSPod record line')}</FormLabel>
                    <FormControl>
                      <Input {...field} autoComplete='off' />
                    </FormControl>
                    <FormMessage />
                  </FormItem>
                )}
              />
              <FormField
                control={form.control}
                name='record_type'
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>{t('DNSPod record type')}</FormLabel>
                    <FormControl>
                      <NativeSelect {...field}>
                        <NativeSelectOption value='A'>
                          A (IPv4)
                        </NativeSelectOption>
                        <NativeSelectOption value='AAAA'>
                          AAAA (IPv6)
                        </NativeSelectOption>
                      </NativeSelect>
                    </FormControl>
                    <FormMessage />
                  </FormItem>
                )}
              />
              {credentialFields.map(({ name, label, configured }) => {
                return (
                  <FormField
                    key={name}
                    control={form.control}
                    name={name}
                    render={({ field }) => (
                      <FormItem>
                        <FormLabel>{label}</FormLabel>
                        <FormControl>
                          <PasswordInput
                            {...field}
                            autoComplete='new-password'
                            placeholder={
                              configured
                                ? t('Already set; leave blank to keep')
                                : t('Not set')
                            }
                          />
                        </FormControl>
                        <FormMessage />
                      </FormItem>
                    )}
                  />
                )
              })}
              {state.session_token_set && (
                <FormField
                  control={form.control}
                  name='clear_session_token'
                  render={({ field }) => (
                    <FormItem className='flex items-center justify-between'>
                      <FormLabel>
                        {t('Clear saved DNSPod session token')}
                      </FormLabel>
                      <FormControl>
                        <Switch
                          checked={field.value}
                          onCheckedChange={field.onChange}
                        />
                      </FormControl>
                      <FormMessage />
                    </FormItem>
                  )}
                />
              )}
            </div>
          </fieldset>
          <Button
            type='submit'
            disabled={locked || !state.can_save || !form.formState.isDirty}
          >
            {t('Save DNSPod configuration')}
          </Button>
        </form>
      </Form>
      <ConfirmDialog
        open={confirming}
        onOpenChange={setConfirming}
        title={t('Save DNSPod configuration')}
        desc={
          <>
            <p className='font-mono'>
              {form.getValues('subdomain') === '@'
                ? form.getValues('domain')
                : `${form.getValues('subdomain')}.${form.getValues('domain')}`}{' '}
              · {form.getValues('record_type')} ·{' '}
              {form.getValues('record_line')}
            </p>
            <p>
              {t(
                'The new credentials and DNS scope will be used by all instances. Disabling management leaves existing DNS records unchanged.'
              )}
            </p>
          </>
        }
        confirmText={t('Continue')}
        handleConfirm={() => {
          setConfirming(false)
          save.mutate(form.getValues())
        }}
      />
      <SecureVerificationDialog {...verification.dialogProps} />
    </>
  )
}
