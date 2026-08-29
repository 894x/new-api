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
import { useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { Button } from '@/components/ui/button'
import { Checkbox } from '@/components/ui/checkbox'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from '@/components/ui/dialog'
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

import { exportLogs } from '../api'
import {
  buildLogExportRequest,
  getDefaultLogExportFields,
  getLogExportFields,
  type LogExportView,
} from '../lib/log-export'
import type { GetLogsParams } from '../types'

interface LogExportDialogProps {
  params: GetLogsParams
  exporter?: typeof exportLogs
}

export function LogExportDialog(props: LogExportDialogProps) {
  const { t } = useTranslation()
  const [open, setOpen] = useState(false)
  const [view, setView] = useState<LogExportView>('upstream')
  const [selectedByView, setSelectedByView] = useState<
    Record<LogExportView, string[]>
  >(() => ({
    upstream: getDefaultLogExportFields('upstream'),
    downstream: getDefaultLogExportFields('downstream'),
  }))
  const [exporting, setExporting] = useState(false)

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

  const handleExport = async () => {
    if (selectedFields.length === 0) return
    setExporting(true)
    try {
      const result = await (props.exporter ?? exportLogs)(
        buildLogExportRequest(view, selectedFields, props.params)
      )
      const url = URL.createObjectURL(result.blob)
      const anchor = document.createElement('a')
      anchor.href = url
      anchor.download = result.filename
      anchor.click()
      URL.revokeObjectURL(url)
      toast.success(t('Usage log export created'))
      setOpen(false)
    } catch (error) {
      toast.error(
        error instanceof Error ? error.message : t('Failed to export logs')
      )
    } finally {
      setExporting(false)
    }
  }

  return (
    <Dialog open={open} onOpenChange={setOpen}>
      <DialogTrigger
        render={<Button variant='outline' size='sm' data-icon='inline-start' />}
      >
        <HugeiconsIcon icon={Download02Icon} strokeWidth={2} />
        {t('Export')}
      </DialogTrigger>
      <DialogContent className='sm:max-w-2xl'>
        <DialogHeader>
          <DialogTitle>{t('Export usage logs')}</DialogTitle>
          <DialogDescription>
            {t(
              'Exports matching usage and error logs as detail and summary CSV files. Failed requests keep pricing empty.'
            )}
          </DialogDescription>
        </DialogHeader>

        <FieldSet>
          <Field>
            <FieldLabel id='log-export-view-label'>
              {t('Reconciliation view')}
            </FieldLabel>
            <Select
              items={scopeItems}
              value={view}
              onValueChange={(value) => {
                if (value === 'upstream' || value === 'downstream') {
                  setView(value)
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

        <DialogFooter>
          <Button
            type='button'
            variant='outline'
            onClick={() => setOpen(false)}
            disabled={exporting}
          >
            {t('Cancel')}
          </Button>
          <Button
            type='button'
            onClick={handleExport}
            disabled={exporting || selectedFields.length === 0}
            data-icon='inline-start'
          >
            <HugeiconsIcon
              icon={exporting ? Loading03Icon : Download02Icon}
              strokeWidth={2}
              className={exporting ? 'animate-spin' : undefined}
            />
            {exporting ? t('Exporting...') : t('Export')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
