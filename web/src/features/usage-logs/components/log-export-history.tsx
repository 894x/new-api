import { useMutation, useQuery } from '@tanstack/react-query'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import { StaticDataTable } from '@/components/data-table'
import { EmptyState } from '@/components/empty-state'
import { ErrorState } from '@/components/error-state'
import { LoadingState } from '@/components/loading-state'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { toIntlLocale } from '@/i18n/languages'
import { formatNumber, formatTimestampToDate } from '@/lib/format'
import { handleServerError } from '@/lib/handle-server-error'
import { useAuthStore } from '@/stores/auth-store'

import {
  getLogExportHistory,
  downloadLogExport,
  type LogExportRecord,
  type LogExportFileDestination,
} from '../log-export-api'

interface LogExportHistoryProps {
  scope: 'all' | 'self'
}

export function LogExportHistory(props: LogExportHistoryProps) {
  const { t, i18n } = useTranslation()
  const locale = toIntlLocale(i18n.resolvedLanguage || i18n.language)
  const ownerID = useAuthStore((state) => state.auth.user?.id)
  const [offset, setOffset] = useState(0)
  const download = useMutation({
    mutationFn: (request: {
      record: LogExportRecord
      destination?: Promise<LogExportFileDestination>
    }) => downloadLogExport(request.record, props.scope, request.destination),
    onSuccess: (result) => {
      if ('saved' in result) return
      const url = URL.createObjectURL(result.blob)
      const anchor = document.createElement('a')
      anchor.href = url
      anchor.download = result.filename
      anchor.click()
      URL.revokeObjectURL(url)
    },
    onError: (error) =>
      handleServerError(error, t('Failed to download export')),
  })
  const history = useQuery({
    queryKey: ['log-exports', ownerID, props.scope, offset],
    queryFn: () => getLogExportHistory(props.scope, offset),
    refetchInterval: (query) =>
      query.state.data?.data.some(
        (record) => record.status === 'pending' || record.status === 'running'
      )
        ? 2000
        : false,
    meta: { errorToast: false },
  })

  const labels = {
    pending: t('Queued'),
    running: t('Generating'),
    succeeded: t('Completed'),
    failed: t('Failed'),
  }
  const viewLabels = {
    self: t('My consumption'),
    upstream: t('Upstream channel attempts'),
    downstream: t('Downstream final requests'),
  }

  if (history.isPending) return <LoadingState />
  if (history.isError && !history.data) {
    return (
      <ErrorState
        title={t('Unable to load export history')}
        onRetry={() => void history.refetch()}
      />
    )
  }
  const records = history.data?.data ?? []

  return (
    <div className='space-y-3'>
      <div className='flex items-center justify-between gap-2'>
        <p className='text-muted-foreground text-sm'>
          {t('Exports continue in the background after you close this panel.')}
        </p>
        <Button
          variant='outline'
          size='sm'
          onClick={() => void history.refetch()}
          disabled={history.isFetching}
        >
          {t('Refresh')}
        </Button>
      </div>
      {history.isError && (
        <p role='alert' className='text-destructive text-sm'>
          {t('Unable to refresh export history')}
        </p>
      )}
      <StaticDataTable<LogExportRecord>
        tableClassName='min-w-[42rem]'
        tableProps={{ 'aria-label': t('Export history') }}
        data={records}
        getRowKey={(record) => record.task_id}
        emptyContent={
          <EmptyState title={t('No export records yet')} className='min-h-36' />
        }
        columns={[
          {
            id: 'export',
            header: t('Export'),
            cell: (record) => (
              <div className='max-w-64 space-y-1 break-words whitespace-normal'>
                <p className='font-medium'>{viewLabels[record.view]}</p>
                <p className='text-muted-foreground text-xs'>
                  {formatTimestampToDate(record.created_at)}
                </p>
                <p className='text-muted-foreground text-xs'>
                  {record.filters?.start_timestamp
                    ? formatTimestampToDate(record.filters.start_timestamp)
                    : t('All time')}
                  {' — '}
                  {record.filters?.end_timestamp
                    ? formatTimestampToDate(record.filters.end_timestamp)
                    : '—'}
                </p>
                {record.filters?.model_name && (
                  <p className='text-xs'>
                    {t('Model Name')}: {record.filters.model_name}
                  </p>
                )}
                {record.filters?.token_name && (
                  <p className='text-xs'>
                    {t('Token Name')}: {record.filters.token_name}
                  </p>
                )}
              </div>
            ),
          },
          {
            id: 'status',
            header: t('Status'),
            cell: (record) => (
              <div
                className='max-w-64 space-y-1 break-words whitespace-normal'
                aria-live='polite'
              >
                <Badge
                  variant={
                    record.status === 'failed' ? 'destructive' : 'secondary'
                  }
                >
                  {labels[record.status]}
                </Badge>
                {record.error && (
                  <p className='text-destructive text-xs'>
                    {/^export exceeds (\d+) records; narrow the filters$/.test(
                      record.error
                    )
                      ? t(
                          'Export exceeds {{count}} records. Narrow the filters and try again.',
                          {
                            count: formatNumber(
                              Number(record.error.match(/\d+/)?.[0]),
                              locale
                            ),
                          }
                        )
                      : t(record.error)}
                  </p>
                )}
              </div>
            ),
          },
          {
            id: 'rows',
            header: t('Records'),
            cell: (record) =>
              record.status === 'succeeded'
                ? formatNumber(record.row_count, locale)
                : '—',
          },
          {
            id: 'download',
            header: t('File'),
            cell: (record) => {
              if (record.status !== 'succeeded') return '—'
              if (
                record.expires_at * 1000 <= Date.now() ||
                record.file_size <= 0
              ) {
                return (
                  <span className='text-muted-foreground'>{t('Expired')}</span>
                )
              }
              return (
                <div className='space-y-1'>
                  <Button
                    variant='outline'
                    size='sm'
                    disabled={download.isPending}
                    onClick={() => {
                      const picker = (
                        window as Window & {
                          showSaveFilePicker?: (options: {
                            suggestedName: string
                          }) => Promise<LogExportFileDestination>
                        }
                      ).showSaveFilePicker
                      const destination = picker?.call(window, {
                        suggestedName: record.filename,
                      })
                      download.mutate({ record, destination })
                    }}
                  >
                    {t('Download')}
                  </Button>
                  <p className='text-muted-foreground text-xs'>
                    {t('Expires')}: {formatTimestampToDate(record.expires_at)}
                  </p>
                </div>
              )
            },
          },
        ]}
      />
      <div className='flex items-center justify-between gap-2'>
        <span className='text-muted-foreground text-sm'>
          {t('Total')}: {formatNumber(history.data?.total ?? 0, locale)}
        </span>
        <div className='flex gap-2'>
          <Button
            variant='outline'
            size='sm'
            disabled={offset === 0}
            onClick={() => setOffset((current) => Math.max(0, current - 20))}
          >
            {t('Previous')}
          </Button>
          <Button
            variant='outline'
            size='sm'
            disabled={offset + 20 >= (history.data?.total ?? 0)}
            onClick={() => setOffset((current) => current + 20)}
          >
            {t('Next')}
          </Button>
        </div>
      </div>
    </div>
  )
}
