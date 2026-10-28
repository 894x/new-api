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
import { Link2, Pause, Play, Radio, RotateCcw } from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'

export function PerformanceChartToolbar(props: {
  live: boolean
  onPause: () => void
  onResume: () => void
  onReset: () => void
  updatedAt: number
  formatTime: (ts: number) => string
}) {
  const { t } = useTranslation()
  return (
    <div className='bg-card flex flex-wrap items-center gap-x-4 gap-y-2 rounded-xl border px-3 py-2 text-xs'>
      <span className='flex items-center gap-2 font-medium' role='status'>
        <Radio
          className={
            props.live
              ? 'size-3.5 text-emerald-500'
              : 'text-muted-foreground size-3.5'
          }
        />
        {props.live
          ? t('Live · every 15 seconds')
          : t('Paused · exploring history')}
      </span>
      <span className='text-muted-foreground flex items-center gap-1.5'>
        <Link2 className='size-3.5' />
        {t('Linked crosshair and zoom')}
      </span>
      <span className='text-muted-foreground hidden xl:inline'>
        {t('Drag to pan · Ctrl + scroll to zoom · keyboard + / −')}
      </span>
      <div className='ml-auto flex flex-wrap items-center gap-1'>
        <Button variant='ghost' size='sm' onClick={props.onReset}>
          <RotateCcw />
          {t('Reset zoom')}
        </Button>
        <Button
          variant='outline'
          size='sm'
          onClick={props.live ? props.onPause : props.onResume}
        >
          {props.live ? <Pause /> : <Play />}
          {props.live ? t('Pause live updates') : t('Back to latest')}
        </Button>
      </div>
      {props.updatedAt > 0 && (
        <span className='text-muted-foreground w-full sm:w-auto'>
          {t('Last updated')}: {props.formatTime(props.updatedAt / 1000)}
        </span>
      )}
    </div>
  )
}
