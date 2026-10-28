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
import { toIntlLocale } from '@/i18n/languages'
import { formatNumber } from '@/lib/format'

import type {
  PerformanceAnalyticsPoint,
  PerformanceChartDatum,
  PerformanceMetric,
  PerformanceMetricChartDatum,
} from './types'

export const PERFORMANCE_CURVE_LABELS = [
  'P50',
  'P90',
  'P95',
  'P99',
  'MAX',
] as const
export const PERFORMANCE_CURVE_COLORS = [
  '#3b82f6',
  '#10b981',
  '#f59e0b',
  '#8b5cf6',
  '#ef4444',
]

const PERCENTILE_FIELDS = [
  ['P50', 'p50_ms'],
  ['P90', 'p90_ms'],
  ['P95', 'p95_ms'],
  ['P99', 'p99_ms'],
  ['MAX', 'max_ms'],
] as const

export function formatRequestBodySize(
  bytes: number,
  locale?: Intl.LocalesArgument
): string {
  for (const [unit, scale] of [
    ['GiB', 1024 ** 3],
    ['MiB', 1024 ** 2],
    ['KiB', 1024],
  ] as const) {
    if (bytes >= scale) return `${formatNumber(bytes / scale, locale)} ${unit}`
  }
  return `${formatNumber(bytes, locale)} B`
}

export function getPerformanceFilterLabel(
  value: string | null,
  options: Array<{ value: string; label: string }>
): string | undefined {
  return value == null
    ? undefined
    : options.find((option) => option.value === value)?.label
}

export function formatPerformanceTimestamp(
  timestamp: number,
  language?: string
): string {
  return new Intl.DateTimeFormat(toIntlLocale(language), {
    month: '2-digit',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit',
  }).format(new Date(timestamp * 1000))
}

export function formatTtftSeconds(
  ms: number,
  locale?: Intl.LocalesArgument
): string {
  if (!Number.isFinite(ms) || ms < 0) return '—'
  // Seconds need three fractional digits to preserve collected millisecond precision.
  return `${Intl.NumberFormat(locale, { maximumFractionDigits: 3 }).format(ms / 1_000)} s`
}

export function formatPerformanceLatency(
  ms: number,
  locale?: Intl.LocalesArgument
): string {
  if (!Number.isFinite(ms) || ms < 0) return '—'
  if (ms >= 1_000) return formatTtftSeconds(ms, locale)
  return `${Intl.NumberFormat(locale, { maximumFractionDigits: 3 }).format(ms)} ms`
}

export function buildPerformanceTimeAxis(
  formatTime: (ts: number) => string,
  textColor: string,
  timestamps: number[] = []
) {
  const finiteTimes = timestamps.filter(Number.isFinite)
  let min: number | undefined
  let max: number | undefined
  if (finiteTimes.length > 0) {
    min = Math.min(...finiteTimes)
    max = Math.max(...finiteTimes)
    if (min === max) {
      min -= 150
      max += 150
    }
  }
  return {
    orient: 'bottom' as const,
    type: 'linear' as const,
    zero: false,
    min,
    max,
    label: {
      formatMethod: (value: number | string) => formatTime(Number(value)),
      style: { fill: textColor, fontSize: 10 },
      autoHide: true,
      autoLimit: true,
    },
    tick: { visible: false },
  }
}

export function buildPerformanceSeries(
  points: PerformanceAnalyticsPoint[],
  metric: 'ttft' | 'tpot',
  formatTime: (ts: number) => string
): PerformanceChartDatum[] {
  return points.flatMap((point) => {
    const percentiles = point[metric]
    if (percentiles.sample_count <= 0) return []

    return PERCENTILE_FIELDS.flatMap(([percentile, field]) => {
      const value = percentiles[field]
      if (typeof value !== 'number' || !Number.isFinite(value)) return []
      return [
        {
          ts: point.ts,
          time: point.ts,
          label: formatTime(point.ts),
          percentile,
          value,
        },
      ]
    })
  })
}

export function buildPerformanceMetricSeries(
  points: PerformanceAnalyticsPoint[],
  metric: PerformanceMetric,
  formatTime: (ts: number) => string
): PerformanceMetricChartDatum[] {
  const hasDistributions = points.some(
    (point) => point.metric_percentiles?.[metric] !== undefined
  )
  return points.flatMap((point) => {
    if (
      metric === 'rpm' ||
      metric === 'tpm' ||
      metric === 'error_count' ||
      !hasDistributions
    ) {
      const value = point[metric]
      if (typeof value !== 'number' || !Number.isFinite(value)) return []
      if (metric === 'cache_write_tokens' && !point.cache_write_samples) {
        return []
      }
      if (metric === 'cache_hit_rate' && point.cache_read_samples === 0) {
        return []
      }
      if (
        metric === 'request_body_avg_bytes' &&
        point.request_body_samples === 0
      ) {
        return []
      }
      return [
        { ts: point.ts, time: point.ts, label: formatTime(point.ts), value },
      ]
    }
    const percentiles = point.metric_percentiles?.[metric]
    if (!percentiles || percentiles.sample_count <= 0) return []
    return PERFORMANCE_CURVE_LABELS.flatMap((percentile) => {
      const field = percentile.toLowerCase() as
        | 'p50'
        | 'p90'
        | 'p95'
        | 'p99'
        | 'max'
      const value = percentiles[field]
      if (typeof value !== 'number' || !Number.isFinite(value)) return []
      return [
        {
          ts: point.ts,
          time: point.ts,
          label: formatTime(point.ts),
          percentile,
          value,
        },
      ]
    })
  })
}
