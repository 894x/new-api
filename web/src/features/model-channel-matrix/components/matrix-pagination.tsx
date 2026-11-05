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

import { Button } from '@/components/ui/button'
import { toIntlLocale } from '@/i18n/languages'
import { formatNumber } from '@/lib/format'

export function MatrixPagination(props: {
  axis: 'models' | 'channels' | 'groups'
  page: number
  pageSize: number
  total: number
  disabled: boolean
  onPageChange: (page: number) => void
}) {
  const { t, i18n } = useTranslation()
  const locale = toIntlLocale(i18n.resolvedLanguage || i18n.language)
  const pages = Math.max(1, Math.ceil(props.total / props.pageSize))
  const labels = {
    models: t('Models'),
    channels: t('Channels'),
    groups: t('Groups'),
  }
  const previousLabels = {
    models: t('Previous models'),
    channels: t('Previous channels'),
    groups: t('Previous groups'),
  }
  const nextLabels = {
    models: t('Next models'),
    channels: t('Next channels'),
    groups: t('Next groups'),
  }
  const label = labels[props.axis]
  return (
    <nav aria-label={label} className='flex flex-wrap items-center gap-2'>
      <span className='text-muted-foreground text-xs'>
        {label} · {formatNumber(props.total, locale)}
      </span>
      <Button
        type='button'
        variant='outline'
        size='sm'
        disabled={props.disabled || props.page <= 1}
        aria-label={previousLabels[props.axis]}
        onClick={() => props.onPageChange(props.page - 1)}
      >
        {t('Previous')}
      </Button>
      <span className='min-w-10 text-center text-xs tabular-nums'>
        {formatNumber(props.page, locale)} / {formatNumber(pages, locale)}
      </span>
      <Button
        type='button'
        variant='outline'
        size='sm'
        disabled={props.disabled || props.page >= pages}
        aria-label={nextLabels[props.axis]}
        onClick={() => props.onPageChange(props.page + 1)}
      >
        {t('Next')}
      </Button>
    </nav>
  )
}
