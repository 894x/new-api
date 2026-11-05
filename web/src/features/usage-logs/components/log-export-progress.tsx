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
import { useTranslation } from 'react-i18next'

import { Progress } from '@/components/ui/progress'
import { toIntlLocale } from '@/i18n/languages'
import { formatNumber } from '@/lib/format'

import type { LogExportRecord } from '../log-export-api'

export function LogExportProgress(props: { record: LogExportRecord }) {
  const { t, i18n } = useTranslation()
  const locale = toIntlLocale(i18n.resolvedLanguage || i18n.language)
  const record = props.record
  let percent: number | null = null
  let label = ''
  if (record.progress?.stage && Number.isFinite(record.progress.percent)) {
    percent = Math.min(99, Math.max(0, record.progress.percent))
  }
  switch (record.status) {
    case 'pending':
      percent = 0
      break
    case 'succeeded':
      percent = 100
      break
    case 'running':
      if (record.progress?.stage === 'counting') label = t('Counting records')
      if (record.progress?.stage === 'exporting') label = t('Exporting records')
      if (record.progress?.stage === 'storing') label = t('Saving export file')
  }

  return (
    <div className='max-w-64 min-w-44 space-y-1.5'>
      <div className='flex items-center justify-between gap-3 text-xs'>
        <span>{label}</span>
        <span className='shrink-0 tabular-nums'>
          {percent === null ? '—' : `${formatNumber(percent, locale)}%`}
        </span>
      </div>
      <Progress value={percent} aria-label={t('Export progress')} />
      {record.status === 'running' &&
        record.progress &&
        record.progress.total > 0 && (
          <p className='text-muted-foreground text-xs tabular-nums'>
            {t('Processed {{processed}} / {{total}} records', {
              processed: formatNumber(record.progress.processed, locale),
              total: formatNumber(record.progress.total, locale),
            })}
          </p>
        )}
    </div>
  )
}
