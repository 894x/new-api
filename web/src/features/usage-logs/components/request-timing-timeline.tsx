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

import { Clock3 } from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { Label } from '@/components/ui/label'
import { formatUseTime } from '@/lib/format'
import { cn } from '@/lib/utils'

import { buildRequestTimingSegments } from '../lib/request-timing'
import type { RequestTimingInfo } from '../types'

const segmentColorClasses = {
  client_upload: 'bg-slate-400 dark:bg-slate-500',
  gateway_processing: 'bg-violet-500 dark:bg-violet-400',
  upstream_upload: 'bg-cyan-500 dark:bg-cyan-400',
  upstream_wait: 'bg-amber-500 dark:bg-amber-400',
  first_response: 'bg-blue-500 dark:bg-blue-400',
  streaming: 'bg-emerald-500 dark:bg-emerald-400',
} as const

function formatMilliseconds(milliseconds: number): string {
  if (milliseconds < 1_000) return `${Math.round(milliseconds)} ms`
  return formatUseTime(milliseconds / 1_000)
}

export function RequestTimingTimeline(props: { timing: RequestTimingInfo }) {
  const { t } = useTranslation()
  const { segments, totalDurationMs, longestSegmentKey } =
    buildRequestTimingSegments(props.timing)
  if (segments.length === 0) return null

  const observedDurationMs = segments.reduce(
    (sum, segment) => sum + segment.durationMs,
    0
  )
  const widthDenominator = Math.max(observedDurationMs, 1)

  return (
    <section className='min-w-0 space-y-1.5' aria-label={t('Request Timeline')}>
      <div className='flex items-center justify-between gap-3'>
        <Label className='flex items-center gap-1.5 text-xs font-semibold'>
          <Clock3 className='size-3.5' aria-hidden='true' />
          {t('Request Timeline')}
        </Label>
        {totalDurationMs > 0 && (
          <span className='text-muted-foreground text-[11px] tabular-nums'>
            {t('Total')} {formatMilliseconds(totalDurationMs)}
          </span>
        )}
      </div>

      <div className='bg-muted/30 min-w-0 rounded-md border p-2.5'>
        <div
          className='bg-muted flex h-3 min-w-0 gap-0.5 overflow-hidden rounded-full p-0.5'
          role='img'
          aria-label={segments
            .map(
              (segment) =>
                `${t(segment.labelKey)} ${formatMilliseconds(segment.durationMs)}`
            )
            .join(', ')}
        >
          {segments.map((segment) => {
            const isLongest = segment.key === longestSegmentKey
            return (
              <span
                key={segment.key}
                className={cn(
                  'min-w-1 rounded-full transition-[flex-grow]',
                  segmentColorClasses[segment.key],
                  isLongest && 'bg-rose-500 dark:bg-rose-400'
                )}
                style={{
                  flexBasis: 0,
                  flexGrow: Math.max(
                    segment.durationMs / widthDenominator,
                    0.02
                  ),
                }}
                title={`${t(segment.labelKey)}: ${formatMilliseconds(segment.durationMs)}`}
              />
            )
          })}
        </div>

        <div className='mt-2.5 grid gap-x-4 gap-y-1.5 sm:grid-cols-2'>
          {segments.map((segment) => {
            const isLongest = segment.key === longestSegmentKey
            return (
              <div
                key={segment.key}
                className={cn(
                  'flex min-w-0 items-center justify-between gap-2 rounded px-1 py-0.5 text-[11px]',
                  isLongest && 'bg-rose-500/10 text-rose-600 dark:text-rose-400'
                )}
              >
                <span className='flex min-w-0 items-center gap-1.5'>
                  <span
                    className={cn(
                      'size-2 shrink-0 rounded-full',
                      segmentColorClasses[segment.key],
                      isLongest && 'bg-rose-500 dark:bg-rose-400'
                    )}
                    aria-hidden='true'
                  />
                  <span className='truncate'>{t(segment.labelKey)}</span>
                  {isLongest && (
                    <span className='sr-only'>({t('Longest Stage')})</span>
                  )}
                </span>
                <span className='shrink-0 font-mono tabular-nums'>
                  {formatMilliseconds(segment.durationMs)}
                </span>
              </div>
            )
          })}
        </div>
      </div>
    </section>
  )
}
