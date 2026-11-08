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

import { Badge } from '@/components/ui/badge'
import { toIntlLocale } from '@/i18n/languages'
import { formatNumber } from '@/lib/format'

import type {
  EarlyChunkCell,
  EarlyChunkGrid,
  EarlyChunkMetrics,
} from './early-chunk-monitor-api'

export function EarlyChunkMetricsDisplay(props: {
  metrics: EarlyChunkMetrics
}) {
  const { t, i18n } = useTranslation()
  const locale = toIntlLocale(i18n.resolvedLanguage || i18n.language)
  const m = props.metrics
  const values = [
    [t('Samples'), formatNumber(m.samples, locale)],
    [t('Content observations'), formatNumber(m.content_samples, locale)],
    [
      t('Average TTFT'),
      m.average_ms == null
        ? '—'
        : `${formatNumber(m.average_ms / 1000, locale)} s`,
    ],
    [
      'p50',
      m.p50_ms == null ? '—' : `${formatNumber(m.p50_ms / 1000, locale)} s`,
    ],
    [
      'p90',
      m.p90_ms == null ? '—' : `${formatNumber(m.p90_ms / 1000, locale)} s`,
    ],
    [
      'p95',
      m.p95_ms == null ? '—' : `${formatNumber(m.p95_ms / 1000, locale)} s`,
    ],
    [t('Completed'), formatNumber(m.completed, locale)],
    [t('Failed'), formatNumber(m.failed, locale)],
    [t('Cancelled'), formatNumber(m.cancelled, locale)],
    [t('Prediction decisions'), formatNumber(m.decisions, locale)],
    [t('Pre-emitted'), formatNumber(m.sent, locale)],
    [t('Gap target met'), formatNumber(m.met, locale)],
    [
      t('Prediction target rate'),
      m.decisions
        ? `${formatNumber((m.met / m.decisions) * 100, locale)}%`
        : '—',
    ],
    [t('Pre-emitted target met'), formatNumber(m.sent_met, locale)],
    [
      t('Pre-emitted target rate'),
      m.sent ? `${formatNumber((m.sent_met / m.sent) * 100, locale)}%` : '—',
    ],
    [t('Failed after pre-emission'), formatNumber(m.sent_failed, locale)],
    [
      t('Average content gap'),
      m.average_gap_ms == null
        ? '—'
        : `${formatNumber(m.average_gap_ms / 1000, locale)} s`,
    ],
    [t('TTFT over 300s'), formatNumber(m.overflow, locale)],
  ]
  return (
    <dl className='grid grid-cols-2 gap-3 sm:grid-cols-4'>
      {values.map(([label, value]) => (
        <div key={label} className='min-w-0'>
          <dt className='text-muted-foreground text-xs'>{label}</dt>
          <dd className='font-medium tabular-nums'>{value}</dd>
        </div>
      ))}
    </dl>
  )
}

export function EarlyChunkCellDetails(props: {
  cell: EarlyChunkCell
  grid: EarlyChunkGrid
  label: string
}) {
  const { t, i18n } = useTranslation()
  const locale = toIntlLocale(i18n.resolvedLanguage || i18n.language)
  const p = props.cell.prediction
  const time = (ms: number | null): string =>
    ms == null ? '—' : `${formatNumber(ms / 1000, locale)} s`
  const reasons: Record<string, string> = {
    eligible: t('Prediction gate passed'),
    insufficient_samples: t('Insufficient pending samples'),
    no_content_estimate: t('No finite first-content estimate'),
    before_candidate: t('Waiting for candidate time'),
    low_probability: t('Conditional probability below threshold'),
    unknown_estimate: t('Unknown token or prefix estimate'),
  }
  return (
    <section
      aria-label={t('Bucket details')}
      className='space-y-4 rounded-xl border p-4'
    >
      <div className='flex flex-wrap items-center gap-2'>
        <h3 className='font-medium'>{props.label}</h3>
        <Badge variant={p.eligible ? 'default' : 'secondary'}>
          {p.eligible
            ? t('Prediction gate passed')
            : t('Prediction gate not passed')}
        </Badge>
      </div>
      <EarlyChunkMetricsDisplay metrics={props.cell.metrics} />
      <dl className='grid grid-cols-2 gap-3 sm:grid-cols-3'>
        {[
          [t('Evaluation time'), time(p.at_ms)],
          [t('Estimated first content'), time(p.estimated_ttft_ms)],
          [t('Suggested advance'), time(p.lead_ms)],
          [t('Candidate pre-emission time'), time(p.candidate_ms)],
          [t('Remaining p50'), time(p.remaining_p50_ms)],
          [t('Remaining p90'), time(p.remaining_p90_ms)],
        ].map(([label, value]) => (
          <div key={label}>
            <dt className='text-muted-foreground text-xs'>{label}</dt>
            <dd className='font-medium tabular-nums'>{value}</dd>
          </div>
        ))}
      </dl>
      <p className='text-muted-foreground text-sm'>
        {reasons[p.reason] ?? t('Unknown')}
      </p>
      <p className='text-muted-foreground text-sm'>
        {t(
          'Conditional prediction: {{probability}}% · {{pending}} pending samples · {{cells}} merged cells',
          {
            probability: formatNumber(p.probability * 100, locale),
            pending: formatNumber(p.pending, locale),
            cells: formatNumber(p.merged_cells, locale),
          }
        )}
      </p>
      <p className='text-muted-foreground text-xs'>
        {t('Last sample')}:{' '}
        {new Date(props.cell.metrics.updated_at_ms).toLocaleString(locale)}
      </p>
      <details>
        <summary className='cursor-pointer text-sm'>
          {t('Content TTFT distribution')}
        </summary>
        <dl className='mt-3 grid grid-cols-2 gap-2 sm:grid-cols-4'>
          {props.cell.histogram.map((n, i) => {
            if (!n) return null
            const lower = i === 0 ? 0 : props.grid.latency_bounds_ms[i - 1]
            const upper = props.grid.latency_bounds_ms[i]
            let label = `> ${formatNumber(lower / 1000, locale)} s`
            if (upper != null) {
              label = `${formatNumber(lower / 1000, locale)}–${formatNumber(upper / 1000, locale)} s`
            }
            return (
              <div key={lower}>
                <dt className='text-muted-foreground text-xs'>{label}</dt>
                <dd>{formatNumber(n, locale)}</dd>
              </div>
            )
          })}
        </dl>
      </details>
    </section>
  )
}
