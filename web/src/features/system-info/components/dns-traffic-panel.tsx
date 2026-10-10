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
import { RefreshCw } from 'lucide-react'
import { useState, type ReactNode } from 'react'
import { useForm, useWatch } from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { ConfirmDialog } from '@/components/confirm-dialog'
import { ErrorState } from '@/components/error-state'
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
import { Skeleton } from '@/components/ui/skeleton'
import {
  SecureVerificationDialog,
  useSecureVerification,
} from '@/features/auth/secure-verification'
import { formatNumber, formatPercent } from '@/lib/format'
import { AuthOperationError } from '@/lib/secure-verification'

import { getDNSTraffic, updateDNSTraffic } from '../api'
import { dnsTrafficSchema, type DNSTrafficForm } from '../lib/dns-traffic'
import type { DNSTrafficState, DNSTrafficUpdate } from '../types'
import { DNSPodConfigPanel } from './dns-pod-config-panel'

const queryKey = ['system-info', 'dns-traffic']
const failureMessages: Record<string, string> = {
  DNS_CONFIG_INVALID: 'DNSPod server configuration is incomplete or invalid.',
  DNS_CONFIG_CONFLICT:
    'DNSPod configuration changed. Refresh and review it before saving again.',
  DNS_REDIS_REQUIRED:
    'A shared Redis connection is required to apply DNS weights.',
  DNS_INSTANCE_COUNT_INVALID:
    'Configure 2 to 16 enabled records in DNSPod for this hostname, line and record type.',
  DNS_RECORDS_INVALID: 'DNS records must contain distinct valid IP addresses.',
  DNS_CONFLICT:
    'DNS records changed. Refresh and review the records before applying again.',
  DNS_BUSY:
    'Another DNS update is in progress. Refresh records and try again later.',
  DNS_INPUT_INVALID:
    'Enter an integer weight from 1 to 100 for each enabled IP.',
}

export function DNSTrafficPanel() {
  const { t } = useTranslation()
  const updating = useIsMutating({ mutationKey: queryKey }) > 0
  const query = useQuery({
    queryKey,
    queryFn: getDNSTraffic,
    staleTime: 30_000,
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
          failureMessages[AuthOperationError.from(query.error).code ?? ''] ??
            'Unable to read DNSPod records. Check server configuration and DNSPod permissions.'
        )}
        onRetry={() => void query.refetch()}
      />
    )
  } else if (query.data.configured) {
    content = (
      <DNSTrafficEditor
        key={query.data.revision}
        state={query.data}
        refreshing={query.isFetching}
      />
    )
  } else {
    content = (
      <p className='text-muted-foreground text-sm'>
        {t(
          'Configure and enable DNSPod below, then refresh DNS records to manage instance weights.'
        )}
      </p>
    )
  }

  return (
    <section
      className='space-y-4 rounded-xl border p-4'
      aria-label={t('DNS traffic allocation')}
    >
      <div className='flex items-center justify-between gap-3'>
        <h2 className='text-base font-semibold'>
          {t('DNS traffic allocation')}
        </h2>
        <Button
          variant='outline'
          size='sm'
          disabled={query.isFetching || updating}
          onClick={() => void query.refetch()}
        >
          <RefreshCw className='size-4' aria-hidden />
          {t('Refresh DNS records')}
        </Button>
      </div>
      <p className='text-muted-foreground text-sm'>
        {t(
          'Weights control DNS answers, not exact request or bandwidth percentages. Changes take effect after DNS caches expire; active connections keep their current instance.'
        )}
      </p>
      {content}
      <DNSPodConfigPanel />
    </section>
  )
}

function DNSTrafficEditor({
  state,
  refreshing,
}: {
  state: DNSTrafficState
  refreshing: boolean
}) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const verification = useSecureVerification()
  const updating = useIsMutating({ mutationKey: queryKey }) > 0
  const [pending, setPending] = useState<DNSTrafficUpdate | null>(null)
  const [failure, setFailure] = useState<string | null>(null)
  const enabled = state.records.filter((record) => record.enabled)
  const form = useForm<DNSTrafficForm>({
    resolver: zodResolver(dnsTrafficSchema(t)),
    defaultValues: {
      revision: state.revision,
      weights: enabled.map((record) => ({
        id: record.id,
        weight: record.weight || 1,
      })),
    },
  })
  const weights = useWatch({ control: form.control, name: 'weights' })
  const total = weights.reduce(
    (sum, record) => sum + (Number.isFinite(record.weight) ? record.weight : 0),
    0
  )
  const currentEqual = enabled.some((record) => record.weight === 0)
  const currentTotal = enabled.reduce((sum, record) => sum + record.weight, 0)
  const changed = weights.some(
    (record, index) => record.weight !== enabled[index]?.weight
  )
  const apply = useMutation({
    mutationKey: queryKey,
    retry: false,
    mutationFn: async (update: DNSTrafficUpdate) => {
      const proof = await verification.requestVerification({
        scope: 'dns.traffic.update',
        context: update,
        title: t('Verify DNS traffic changes'),
      })
      if (!proof) return null
      return updateDNSTraffic(update, proof.proof_token)
    },
    onSuccess: (result) => {
      if (!result) return
      queryClient.setQueryData(queryKey, result)
      form.reset({
        revision: result.revision,
        weights: result.records
          .filter((record) => record.enabled)
          .map((record) => ({ id: record.id, weight: record.weight })),
      })
      toast.success(
        t(
          'DNS weights applied and verified. DNS caches may still use the previous allocation.'
        )
      )
    },
    onError: (error) => {
      const code = AuthOperationError.from(error).code ?? ''
      setFailure(
        t(
          failureMessages[code] ??
            'DNS update could not be verified and some records may have changed. Refresh and review DNSPod records before trying again; uncertain updates are locked for up to 150 seconds.'
        )
      )
    },
  })
  const locked = updating || pending !== null

  return (
    <>
      <p className='text-sm'>
        <span className='font-mono'>
          {state.subdomain === '@'
            ? state.domain
            : `${state.subdomain}.${state.domain}`}
        </span>{' '}
        · {state.record_type} · {state.record_line}
      </p>
      {!state.can_apply && (
        <p role='status' className='text-muted-foreground text-sm'>
          {t(
            failureMessages[state.blocked_reason ?? ''] ??
              'DNS traffic allocation is currently unavailable.'
          )}
        </p>
      )}
      {currentEqual && (
        <p className='text-muted-foreground text-sm'>
          {t(
            'A current weight of 0 disables weighting for the entire group; it does not stop traffic. Applying positive weights enables weighted DNS answers.'
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
          noValidate
          onSubmit={form.handleSubmit((values) => {
            setFailure(null)
            setPending(values)
          })}
          className='space-y-4'
        >
          <fieldset
            disabled={locked || !state.can_apply}
            className='grid min-w-0 gap-4 sm:grid-cols-2'
          >
            {enabled.map((record, index) => (
              <FormField
                key={record.id}
                control={form.control}
                name={`weights.${index}.weight`}
                render={({ field }) => (
                  <FormItem className='min-w-0 rounded-lg border p-3'>
                    <FormLabel className='break-all'>
                      {t('DNS weight for {{ip}}', { ip: record.ip })}
                    </FormLabel>
                    {record.remark && (
                      <p className='text-muted-foreground text-xs break-words'>
                        {record.remark}
                      </p>
                    )}
                    <FormControl>
                      <Input
                        {...field}
                        type='number'
                        min={1}
                        max={100}
                        step={1}
                        value={Number.isFinite(field.value) ? field.value : ''}
                        onChange={(event) =>
                          field.onChange(
                            event.target.value === ''
                              ? Number.NaN
                              : event.target.valueAsNumber
                          )
                        }
                        disabled={locked || !state.can_apply}
                      />
                    </FormControl>
                    <FormMessage />
                    <p className='text-muted-foreground text-xs'>
                      {t(
                        'Current DNS share: {{share}} · TTL: {{ttl}} seconds',
                        {
                          share: formatPercent(
                            currentEqual
                              ? 100 / enabled.length
                              : (record.weight * 100) / currentTotal
                          ),
                          ttl: formatNumber(record.ttl),
                        }
                      )}
                    </p>
                    <p className='text-xs'>
                      {t('Planned DNS share: {{share}}', {
                        share: formatPercent(
                          total > 0 && field.value >= 1 && field.value <= 100
                            ? (field.value * 100) / total
                            : null
                        ),
                      })}
                    </p>
                  </FormItem>
                )}
              />
            ))}
          </fieldset>
          {state.records
            .filter((record) => !record.enabled)
            .map((record) => (
              <p key={record.id} className='text-muted-foreground text-sm'>
                {t('Disabled DNS record: {{ip}}', { ip: record.ip })}
              </p>
            ))}
          <Button
            type='submit'
            disabled={!state.can_apply || locked || refreshing || !changed}
          >
            {apply.isPending
              ? t('Applying DNS weights...')
              : t('Apply DNS weights')}
          </Button>
        </form>
      </Form>
      <ConfirmDialog
        open={pending !== null}
        onOpenChange={(open) => {
          if (!open) setPending(null)
        }}
        title={t('Apply DNS weights')}
        desc={t(
          'This updates the existing DNSPod records for this hostname and line. Records are updated one at a time. Review the planned DNS shares before confirming.'
        )}
        confirmText={t('Confirm DNS changes')}
        handleConfirm={() => {
          if (!pending) return
          const update = pending
          setPending(null)
          apply.mutate(update)
        }}
      />
      <SecureVerificationDialog {...verification.dialogProps} />
    </>
  )
}
