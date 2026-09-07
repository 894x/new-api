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

export function MatrixPagination(props: {
  axis: 'models' | 'channels'
  page: number
  pageSize: number
  total: number
  disabled: boolean
  onPageChange: (page: number) => void
}) {
  const { t } = useTranslation()
  const pages = Math.max(1, Math.ceil(props.total / props.pageSize))
  const label = props.axis === 'models' ? t('Models') : t('Channels')
  return (
    <nav aria-label={label} className='flex flex-wrap items-center gap-2'>
      <span className='text-muted-foreground text-xs'>
        {label} · {props.total}
      </span>
      <Button
        type='button'
        variant='outline'
        size='sm'
        disabled={props.disabled || props.page <= 1}
        aria-label={
          props.axis === 'models'
            ? t('Previous models')
            : t('Previous channels')
        }
        onClick={() => props.onPageChange(props.page - 1)}
      >
        {t('Previous')}
      </Button>
      <span className='min-w-10 text-center text-xs tabular-nums'>
        {props.page} / {pages}
      </span>
      <Button
        type='button'
        variant='outline'
        size='sm'
        disabled={props.disabled || props.page >= pages}
        aria-label={
          props.axis === 'models' ? t('Next models') : t('Next channels')
        }
        onClick={() => props.onPageChange(props.page + 1)}
      >
        {t('Next')}
      </Button>
    </nav>
  )
}
