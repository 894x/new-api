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
import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'
import {
  Popover,
  PopoverContent,
  PopoverDescription,
  PopoverTitle,
  PopoverTrigger,
} from '@/components/ui/popover'
import { cn } from '@/lib/utils'

export type MonitorLayout = { rows: number; columns: number }

export function MonitorLayoutPicker(props: {
  value?: MonitorLayout
  disabled?: boolean
  onChange: (layout: MonitorLayout) => void
}) {
  const { t } = useTranslation()
  const [open, setOpen] = useState(false)
  const [preview, setPreview] = useState<MonitorLayout>(
    props.value ?? { rows: 2, columns: 2 }
  )

  return (
    <Popover
      open={open}
      onOpenChange={(next) => {
        setOpen(next)
        if (next) setPreview(props.value ?? { rows: 2, columns: 2 })
      }}
    >
      <PopoverTrigger
        disabled={props.disabled}
        render={<Button variant='outline' disabled={props.disabled} />}
      >
        {t('Multi-panel mode')}
      </PopoverTrigger>
      <PopoverContent align='end' className='w-64'>
        <PopoverTitle>{t('Monitor layout')}</PopoverTitle>
        <PopoverDescription>
          {t('Move over the grid to choose rows and columns, then click.')}
        </PopoverDescription>
        <div
          role='group'
          aria-label={t('Monitor layout')}
          className='grid grid-cols-4 gap-2'
        >
          {Array.from({ length: 16 }, (_, index) => {
            const rows = Math.floor(index / 4) + 1
            const columns = (index % 4) + 1
            const highlighted =
              rows <= preview.rows && columns <= preview.columns
            return (
              <Button
                key={index}
                variant='outline'
                className={cn(
                  'h-9 w-full rounded-md p-0',
                  highlighted &&
                    'border-primary bg-primary/20 text-primary hover:bg-primary/25 dark:border-primary dark:bg-primary/25 dark:hover:bg-primary/30'
                )}
                aria-label={t('{{rows}} rows × {{columns}} columns', {
                  rows,
                  columns,
                })}
                aria-pressed={
                  props.value?.rows === rows && props.value.columns === columns
                }
                onPointerEnter={() => setPreview({ rows, columns })}
                onFocus={() => setPreview({ rows, columns })}
                onClick={() => {
                  props.onChange({ rows, columns })
                  setOpen(false)
                }}
              >
                <span
                  aria-hidden='true'
                  className='size-2 rounded-sm bg-current opacity-40'
                />
              </Button>
            )
          })}
        </div>
        <p
          role='status'
          aria-live='polite'
          className='text-center text-sm tabular-nums'
        >
          {t('{{rows}} rows × {{columns}} columns', preview)}
        </p>
      </PopoverContent>
    </Popover>
  )
}
