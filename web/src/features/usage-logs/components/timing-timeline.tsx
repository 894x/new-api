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
import { cn } from '@/lib/utils'

import { formatTimingMilliseconds } from '../lib/request-timing'

export type TimingSegment = {
  key: string
  label: string
  durationMs: number
  colorClass: string
}

export function TimingTimeline(props: {
  title: string
  segments: TimingSegment[]
  totalDurationMs: number
}) {
  const { t } = useTranslation()
  const { segments, totalDurationMs } = props
  const longestSegmentKey = segments.reduce<TimingSegment | undefined>(
    (longest, segment) =>
      !longest || segment.durationMs > longest.durationMs ? segment : longest,
    undefined
  )?.key
  if (segments.length === 0) return null

  const observedDurationMs = segments.reduce(
    (sum, segment) => sum + segment.durationMs,
    0
  )
  const widthDenominator = Math.max(observedDurationMs, 1)

  return (
    <section className='min-w-0 space-y-1.5' aria-label={props.title}>
      <div className='flex items-center justify-between gap-3'>
        <Label className='flex items-center gap-1.5 text-xs font-semibold'>
          <Clock3 className='size-3.5' aria-hidden='true' />
          {props.title}
        </Label>
        {totalDurationMs > 0 && (
          <span className='text-muted-foreground text-[11px] tabular-nums'>
            {t('Total')} {formatTimingMilliseconds(totalDurationMs)}
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
                `${segment.label} ${formatTimingMilliseconds(segment.durationMs)}`
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
                  segment.colorClass,
                  isLongest && 'bg-rose-500 dark:bg-rose-400'
                )}
                style={{
                  flexBasis: 0,
                  flexGrow: Math.max(
                    segment.durationMs / widthDenominator,
                    0.02
                  ),
                }}
                title={`${segment.label}: ${formatTimingMilliseconds(segment.durationMs)}`}
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
                      segment.colorClass,
                      isLongest && 'bg-rose-500 dark:bg-rose-400'
                    )}
                    aria-hidden='true'
                  />
                  <span className='truncate'>{segment.label}</span>
                  {isLongest && (
                    <span className='sr-only'>({t('Longest Stage')})</span>
                  )}
                </span>
                <span className='shrink-0 font-mono tabular-nums'>
                  {formatTimingMilliseconds(segment.durationMs)}
                </span>
              </div>
            )
          })}
        </div>
      </div>
    </section>
  )
}
