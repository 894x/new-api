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
import { Download02Icon, Loading03Icon } from '@hugeicons/core-free-icons'
import { HugeiconsIcon } from '@hugeicons/react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { Dialog } from '@/components/dialog'
import { Button } from '@/components/ui/button'
import { Checkbox } from '@/components/ui/checkbox'
import { Field, FieldLabel, FieldSet, FieldTitle } from '@/components/ui/field'
import { ScrollArea } from '@/components/ui/scroll-area'
import {
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { toIntlLocale } from '@/i18n/languages'
import { formatNumber } from '@/lib/format'
import { handleServerError } from '@/lib/handle-server-error'

import {
  buildLogExportRequest,
  getDefaultLogExportFields,
  getLogExportFields,
  type LogExportView,
} from '../lib/log-export'
import { createLogExport, getLogExportConfig } from '../log-export-api'
import type { GetLogsParams, LogExportRequest } from '../types'
import { LogExportHistory } from './log-export-history'

interface LogExportDialogProps {
  scope: 'all' | 'self'
  params: GetLogsParams
  exporter?: typeof createLogExport
}

export function LogExportDialog(props: LogExportDialogProps) {
  const { t, i18n } = useTranslation()
  const locale = toIntlLocale(i18n.resolvedLanguage || i18n.language)
  const queryClient = useQueryClient()
  const [open, setOpen] = useState(false)
  const [panel, setPanel] = useState('new')
  const config = useQuery({
    queryKey: ['log-export-config'],
    queryFn: getLogExportConfig,
    enabled: open,
    meta: { errorToast: false },
  })
  const [adminView, setAdminView] = useState<'upstream' | 'downstream'>(
    'upstream'
  )
  const view: LogExportView = props.scope === 'self' ? 'self' : adminView
  const [selectedByView, setSelectedByView] = useState<
    Record<LogExportView, string[]>
  >(() => ({
    upstream: getDefaultLogExportFields('upstream'),
    downstream: getDefaultLogExportFields('downstream'),
    self: getDefaultLogExportFields('self'),
  }))
  const exportMutation = useMutation({
    mutationFn: (request: LogExportRequest) =>
      (props.exporter ?? createLogExport)(request),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ['log-exports'] })
      setPanel('history')
      toast.success(
        t('Export queued. You can download it from export history when ready.')
      )
    },
    onError: (error) => handleServerError(error, t('Failed to export logs')),
  })
  const exporting = exportMutation.isPending

  const availableFields = useMemo(() => getLogExportFields(view), [view])
  const selectedFields = selectedByView[view]
  const selectedFieldSet = useMemo(
    () => new Set(selectedFields),
    [selectedFields]
  )
  const scopeItems = useMemo(
    () => [
      { value: 'upstream', label: t('Upstream channel attempts') },
      { value: 'downstream', label: t('Downstream final requests') },
    ],
    [t]
  )
  const scopeLabel =
    scopeItems.find((item) => item.value === view)?.label ??
    t('Upstream channel attempts')

  const toggleField = (key: string, checked: boolean) => {
    setSelectedByView((current) => {
      const selected = current[view]
      return {
        ...current,
        [view]: checked
          ? [...selected, key]
          : selected.filter((field) => field !== key),
      }
    })
  }

  const handleExport = () => {
    if (selectedFields.length === 0) return
    exportMutation.mutate(
      buildLogExportRequest(view, selectedFields, props.params)
    )
  }

  return (
    <Dialog
      open={open}
      onOpenChange={setOpen}
      contentClassName='sm:max-w-4xl'
      title={
        props.scope === 'self'
          ? t('Export my consumption data')
          : t('Export usage logs')
      }
      description={
        props.scope === 'self'
          ? t(
              'Exports your consumption, violation fees, and refunds as detail and per-model summary CSV files. Charges are positive and refunds negative. Historical USD amounts may be blank when conversion data is unavailable.'
            )
          : t(
              'Exports matching usage and error logs as detail and summary CSV files. Failed requests keep pricing empty.'
            )
      }
      trigger={
        <Button variant='outline' size='sm' data-icon='inline-start'>
          <HugeiconsIcon icon={Download02Icon} strokeWidth={2} />
          {t('Export')}
        </Button>
      }
      footer={
        <>
          <Button
            type='button'
            variant='outline'
            onClick={() => setOpen(false)}
            disabled={exporting}
          >
            {t('Cancel')}
          </Button>
          {panel === 'new' && (
            <Button
              type='button'
              onClick={handleExport}
              disabled={
                exporting || selectedFields.length === 0 || !config.data
              }
              data-icon='inline-start'
            >
              <HugeiconsIcon
                icon={exporting ? Loading03Icon : Download02Icon}
                strokeWidth={2}
                className={exporting ? 'animate-spin' : undefined}
              />
              {exporting ? t('Exporting...') : t('Export')}
            </Button>
          )}
        </>
      }
    >
      <Tabs value={panel} onValueChange={(value) => setPanel(String(value))}>
        <TabsList aria-label={t('Export panel')}>
          <TabsTrigger value='new'>{t('New export')}</TabsTrigger>
          <TabsTrigger value='history'>{t('Export history')}</TabsTrigger>
        </TabsList>
        <TabsContent value='new' className='space-y-4'>
          <div className='bg-muted/40 space-y-1 rounded-lg border p-3 text-sm'>
            <p>
              {t(
                'The export uses your current filters and runs in the background.'
              )}
            </p>
            {config.data && (
              <p className='text-muted-foreground'>
                {t(
                  'Maximum {{count}} records per export. Files are kept for {{days}} days.',
                  {
                    count: formatNumber(config.data.max_rows, locale),
                    days: config.data.retention_days,
                  }
                )}
              </p>
            )}
            {config.data && (
              <p className='text-muted-foreground'>
                {t('Maximum file size: {{size}} MiB.', {
                  size: formatNumber(
                    config.data.max_file_bytes / 1024 / 1024,
                    locale
                  ),
                })}
              </p>
            )}
            {config.isPending && (
              <p role='status'>{t('Loading export limit...')}</p>
            )}
            {config.isError && (
              <div role='alert' className='flex items-center gap-2'>
                <span>{t('Unable to load export limit')}</span>
                <Button
                  variant='outline'
                  size='sm'
                  onClick={() => void config.refetch()}
                >
                  {t('Retry')}
                </Button>
              </div>
            )}
          </div>
          <FieldSet>
            {props.scope === 'all' && (
              <Field>
                <FieldLabel id='log-export-view-label'>
                  {t('Reconciliation view')}
                </FieldLabel>
                <Select
                  items={scopeItems}
                  value={view}
                  onValueChange={(value) => {
                    if (value === 'upstream' || value === 'downstream') {
                      setAdminView(value)
                    }
                  }}
                >
                  <SelectTrigger aria-labelledby='log-export-view-label'>
                    <SelectValue>{scopeLabel}</SelectValue>
                  </SelectTrigger>
                  <SelectContent alignItemWithTrigger={false}>
                    <SelectGroup>
                      {scopeItems.map((item) => (
                        <SelectItem key={item.value} value={item.value}>
                          {item.label}
                        </SelectItem>
                      ))}
                    </SelectGroup>
                  </SelectContent>
                </Select>
                <p className='text-muted-foreground text-xs'>
                  {view === 'upstream'
                    ? t('One row per recorded channel attempt.')
                    : t(
                        'One final row per request. Channel and upstream fields are excluded.'
                      )}
                </p>
              </Field>
            )}

            <FieldSet>
              <div className='flex items-center justify-between gap-2'>
                <FieldTitle>{t('Detail columns')}</FieldTitle>
                <div className='flex gap-1'>
                  <Button
                    type='button'
                    variant='ghost'
                    size='xs'
                    onClick={() =>
                      setSelectedByView((current) => ({
                        ...current,
                        [view]: availableFields.map((field) => field.key),
                      }))
                    }
                  >
                    {t('Select all')}
                  </Button>
                  <Button
                    type='button'
                    variant='ghost'
                    size='xs'
                    onClick={() =>
                      setSelectedByView((current) => ({
                        ...current,
                        [view]: getDefaultLogExportFields(view),
                      }))
                    }
                  >
                    {t('Reset')}
                  </Button>
                </div>
              </div>
              <ScrollArea className='h-64 rounded-lg border'>
                <div className='grid gap-1 p-3 sm:grid-cols-2'>
                  {availableFields.map((field) => {
                    const id = `log-export-${view}-${field.key}`
                    return (
                      <Field key={field.key} orientation='horizontal'>
                        <Checkbox
                          id={id}
                          checked={selectedFieldSet.has(field.key)}
                          onCheckedChange={(checked) =>
                            toggleField(field.key, Boolean(checked))
                          }
                        />
                        <FieldLabel htmlFor={id}>{t(field.label)}</FieldLabel>
                      </Field>
                    )
                  })}
                </div>
              </ScrollArea>
            </FieldSet>
          </FieldSet>
        </TabsContent>
        <TabsContent value='history'>
          <LogExportHistory key={props.scope} scope={props.scope} />
        </TabsContent>
      </Tabs>
    </Dialog>
  )
}
