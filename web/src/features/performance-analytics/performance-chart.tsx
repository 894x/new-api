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
import type { EChartsType } from 'echarts/core'
import { Activity } from 'lucide-react'
import { useContext, useEffect, useMemo, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { ErrorState } from '@/components/error-state'
import { useTheme } from '@/context/theme-provider'

import { PerformanceChartContext } from './chart-context'
import { buildPerformanceChartOption } from './chart-options'
import type { PerformanceMetricChartDatum } from './types'

export function PerformanceChart(props: {
  title: string
  values: PerformanceMetricChartDatum[]
  timestamps: number[]
  formatTime: (ts: number) => string
  formatValue: (value: number) => string
  formatTooltipValue?: (value: number) => string
}) {
  const { t } = useTranslation()
  const { resolvedTheme } = useTheme()
  const context = useContext(PerformanceChartContext)
  const host = useRef<HTMLDivElement>(null)
  const instance = useRef<EChartsType | null>(null)
  const explore = useRef(context.onExplore)
  explore.current = context.onExplore
  const [failed, setFailed] = useState(false)
  const [attempt, setAttempt] = useState(0)
  const [ready, setReady] = useState(false)
  const option = useMemo(
    () =>
      buildPerformanceChartOption({
        ...props,
        start: context.start,
        end: context.end,
        dark: resolvedTheme === 'dark',
      }),
    [props, context.start, context.end, resolvedTheme]
  )
  const latestOption = useRef(option)
  latestOption.current = option

  useEffect(() => {
    let cancelled = false
    let observer: ResizeObserver | undefined
    let leave: (() => void) | undefined
    setFailed(false)
    setReady(false)
    import('./echarts-runtime')
      .then((runtime) => {
        if (cancelled || !host.current) return
        const chart = runtime.init(host.current)
        instance.current = chart
        chart.group = context.group
        chart.setOption(latestOption.current)
        chart.on('datazoom', () => explore.current())
        leave = runtime.joinChartGroup(context.group)
        observer = new ResizeObserver(() => chart.resize())
        observer.observe(host.current)
        setReady(true)
      })
      .catch(() => {
        if (!cancelled) setFailed(true)
      })
    return () => {
      cancelled = true
      observer?.disconnect()
      leave?.()
      instance.current?.dispose()
      instance.current = null
    }
  }, [context.group, attempt])

  useEffect(() => {
    // Patch data and theme without resetting the user's legend or zoom choices.
    // No selected/start/end fields are supplied, so ECharts retains interaction state.
    instance.current?.setOption(option, { replaceMerge: ['series'] })
  }, [option])

  useEffect(() => {
    instance.current?.dispatchAction(
      { type: 'dataZoom', start: 0, end: 100 },
      { silent: true }
    )
  }, [context.reset])

  return (
    <div className='relative h-56 min-w-0'>
      <div
        ref={host}
        role='img'
        aria-label={props.title}
        tabIndex={0}
        className='focus-visible:outline-ring h-full w-full rounded-md focus-visible:outline-2'
        onKeyDown={(event) => {
          const chart = instance.current
          if (!chart) {
            return
          }
          if (
            !['+', '=', '-', 'ArrowLeft', 'ArrowRight', 'Home'].includes(
              event.key
            )
          ) {
            return
          }
          event.preventDefault()
          const zoom = (
            chart.getOption().dataZoom as Array<{ start: number; end: number }>
          )[0]
          let start = zoom.start
          let end = zoom.end
          const step = (end - start) * 0.1
          if (event.key === '+' || event.key === '=') {
            start += step
            end -= step
          }
          if (event.key === '-') {
            start = Math.max(0, start - step)
            end = Math.min(100, end + step)
          }
          if (event.key === 'ArrowLeft') {
            const shift = Math.min(step, start)
            start -= shift
            end -= shift
          }
          if (event.key === 'ArrowRight') {
            const shift = Math.min(step, 100 - end)
            start += shift
            end += shift
          }
          if (event.key === 'Home') {
            start = 0
            end = 100
          }
          chart.dispatchAction({ type: 'dataZoom', start, end })
        }}
      />
      {failed && (
        <div className='bg-card absolute inset-0'>
          <ErrorState
            title={t('Failed to load performance data')}
            onRetry={() => setAttempt((value) => value + 1)}
          />
        </div>
      )}
      {!ready && !failed && (
        <div
          role='status'
          className='text-muted-foreground pointer-events-none absolute inset-0 flex items-center justify-center gap-2 text-sm'
        >
          <Activity className='size-4 animate-pulse' />
          {t('Loading...')}
        </div>
      )}
    </div>
  )
}
