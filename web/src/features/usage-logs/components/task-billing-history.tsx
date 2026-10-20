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
import { useQuery } from '@tanstack/react-query'
import { useId, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { CopyButton } from '@/components/copy-button'
import { Dialog } from '@/components/dialog'
import { EmptyState } from '@/components/empty-state'
import { ErrorState } from '@/components/error-state'
import { LoadingState } from '@/components/loading-state'
import { StatusBadge } from '@/components/status-badge'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { formatLogQuota, formatTimestampToDate } from '@/lib/format'
import { useAuthStore } from '@/stores/auth-store'

import { getTaskBillingHistory } from '../api'
import { taskStatusMapper } from '../lib/mappers'
import type { TaskBillingHistory } from '../types'
import { DetailRow } from './dialogs/log-detail-layout'

type BillingLookup = { task_id?: string; request_id?: string }

function TaskBillingHistoryContent(props: { history: TaskBillingHistory }) {
  const { t } = useTranslation()
  const history = props.history
  const labels: Record<string, string> = {
    initial_charge: t('Initial charge'),
    additional_charge: t('Additional task charge'),
    refund: t('Refund'),
  }
  const totals: Array<[string, number]> = [
    [t('Total charged'), history.charged_quota],
    [t('Total refunded'), history.refunded_quota],
    [t('Net charge from logs'), history.net_quota],
  ]
  return (
    <div className='min-w-0 space-y-4'>
      <DetailRow label={t('Task ID')} value={history.task_id} mono />
      <DetailRow
        label={t('Status')}
        value={
          <StatusBadge
            label={t(taskStatusMapper.getLabel(history.status))}
            variant={taskStatusMapper.getVariant(history.status)}
            size='sm'
            copyable={false}
          />
        }
      />
      {history.submit_request_id && (
        <DetailRow
          label={t('Submit Request ID')}
          value={
            <span className='inline-flex max-w-full items-start gap-1'>
              <span className='min-w-0 break-all'>
                {history.submit_request_id}
              </span>
              <CopyButton
                value={history.submit_request_id}
                aria-label={t('Copy submit request ID')}
                className='size-6'
              />
            </span>
          }
          mono
        />
      )}
      <div className='grid grid-cols-1 gap-2 sm:grid-cols-3'>
        {totals.map(([label, quota]) => (
          <div key={label} className='min-w-0 rounded-md border p-3'>
            <p className='text-muted-foreground text-xs'>{label}</p>
            <p className='mt-1 font-mono text-sm break-all'>
              {formatLogQuota(quota)}
            </p>
          </div>
        ))}
      </div>
      <DetailRow
        label={t('Task charge')}
        value={formatLogQuota(history.task_quota)}
      />
      <p role='status' className='text-muted-foreground text-xs'>
        {history.reconciled
          ? t('Billing matches the task charge')
          : t('Billing has not been reconciled')}
        {history.reconciled &&
          history.charged_quota > 0 &&
          history.net_quota === 0 && (
            <span className='text-success ml-2'>{t('Fully refunded')}</span>
          )}
      </p>
      {history.truncated && (
        <p role='alert' className='text-destructive text-xs'>
          {t(
            'Billing history is incomplete. Totals only cover the displayed records.'
          )}
        </p>
      )}
      <p className='text-muted-foreground text-xs'>
        {t(
          'Only retained billing logs are shown. Missing logs do not prove that a charge or refund did not occur.'
        )}
      </p>
      {history.entries.length === 0 ? (
        <EmptyState
          title={t('No billing records found')}
          className='min-h-24'
        />
      ) : (
        <ol aria-label={t('Billing records')} className='space-y-2'>
          {history.entries.map((entry) => (
            <li
              key={`${entry.request_id}-${entry.created_at}-${entry.kind}-${entry.quota}`}
              className='min-w-0 space-y-2 rounded-md border p-3'
            >
              <div className='flex flex-wrap items-center justify-between gap-2 text-sm'>
                <span>{labels[entry.kind] ?? t('Unknown')}</span>
                <span className='font-mono'>
                  {entry.kind === 'refund' ? '+' : '-'}
                  {formatLogQuota(entry.quota)}
                </span>
              </div>
              <p className='text-muted-foreground text-xs'>
                {formatTimestampToDate(entry.created_at, 'seconds')}
              </p>
              <div className='flex min-w-0 items-start gap-1'>
                <span className='min-w-0 font-mono text-xs break-all'>
                  {entry.request_id || '-'}
                </span>
                {entry.request_id && (
                  <CopyButton
                    value={entry.request_id}
                    aria-label={t('Copy request ID')}
                    className='size-6'
                  />
                )}
              </div>
            </li>
          ))}
        </ol>
      )}
    </div>
  )
}

export function TaskBillingHistoryButton(props: {
  taskId?: string
  requestId?: string
  userId?: number
  isAdmin: boolean
}) {
  const { t } = useTranslation()
  const inputId = useId()
  const viewerId = useAuthStore((state) => state.auth.user?.id)
  const [open, setOpen] = useState(false)
  const [kind, setKind] = useState<'task_id' | 'request_id'>('task_id')
  const [value, setValue] = useState('')
  const [lookup, setLookup] = useState<BillingLookup>()
  const ownerId = props.userId ?? (props.isAdmin ? undefined : viewerId)
  const query = useQuery({
    queryKey: ['task-billing-history', viewerId, ownerId, lookup],
    queryFn: ({ signal }) =>
      getTaskBillingHistory({ ...lookup, user_id: ownerId }, signal),
    enabled: open && !!lookup,
    staleTime: 0,
    retry: false,
    meta: { errorToast: false },
  })

  function changeOpen(next: boolean) {
    if (next) {
      const nextKind = props.taskId ? 'task_id' : 'request_id'
      const nextValue = props.taskId || props.requestId || ''
      setKind(nextKind)
      setValue(nextValue)
      setLookup(nextValue ? { [nextKind]: nextValue } : undefined)
    }
    setOpen(next)
  }

  return (
    <>
      <Button variant='outline' size='sm' onClick={() => changeOpen(true)}>
        {t('Related billing')}
      </Button>
      <Dialog
        open={open}
        onOpenChange={changeOpen}
        title={t('Related billing')}
        description={t('Find all charges and refunds by task ID or request ID')}
        contentClassName='sm:max-w-2xl'
        contentHeight='min(70dvh, 680px)'
      >
        <form
          className='mb-4 flex flex-wrap items-end gap-2'
          onSubmit={(event) => {
            event.preventDefault()
            if (value.trim()) {
              const next = { [kind]: value.trim() }
              if (lookup?.[kind] === value.trim()) void query.refetch()
              else setLookup(next)
            }
          }}
        >
          <div className='space-y-1'>
            <Label htmlFor={`${inputId}-kind`}>{t('Search by')}</Label>
            <Select
              value={kind}
              onValueChange={(next) => {
                if (next === 'task_id' || next === 'request_id') setKind(next)
              }}
            >
              <SelectTrigger id={`${inputId}-kind`} aria-label={t('Search by')}>
                <SelectValue>
                  {kind === 'task_id' ? t('Task ID') : t('Request ID')}
                </SelectValue>
              </SelectTrigger>
              <SelectContent>
                <SelectItem value='task_id'>{t('Task ID')}</SelectItem>
                <SelectItem value='request_id'>{t('Request ID')}</SelectItem>
              </SelectContent>
            </Select>
          </div>
          <div className='min-w-0 flex-1 basis-48 space-y-1'>
            <Label htmlFor={inputId}>
              {kind === 'task_id' ? t('Task ID') : t('Request ID')}
            </Label>
            <Input
              id={inputId}
              value={value}
              maxLength={kind === 'task_id' ? 191 : 64}
              onChange={(event) => setValue(event.target.value)}
              className='font-mono text-xs'
            />
          </div>
          <Button
            type='submit'
            size='sm'
            disabled={!value.trim() || query.isFetching}
          >
            {t('Search')}
          </Button>
        </form>
        {open && lookup && (
          <div aria-busy={query.isFetching}>
            {query.isPending && <LoadingState size='sm' />}
            {query.isError && (
              <ErrorState
                title={t('Failed to load related billing')}
                description={t(
                  'Check the ID and access permissions. Historical task IDs may require a user-specific entry.'
                )}
                onRetry={() => void query.refetch()}
              />
            )}
            {query.data && !query.isError && (
              <TaskBillingHistoryContent history={query.data} />
            )}
            {query.data && (
              <Button
                variant='outline'
                size='sm'
                className='mt-3'
                disabled={query.isFetching}
                onClick={() => void query.refetch()}
              >
                {t('Refresh')}
              </Button>
            )}
          </div>
        )}
      </Dialog>
    </>
  )
}
