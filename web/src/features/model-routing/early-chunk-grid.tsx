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
import { useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { Dialog } from '@/components/dialog'
import { LearnMore } from '@/components/learn-more'
import { Button } from '@/components/ui/button'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { toIntlLocale } from '@/i18n/languages'
import { formatNumber } from '@/lib/format'
import { cn } from '@/lib/utils'

import {
  EarlyChunkCellDetails,
  EarlyChunkMetricsDisplay,
} from './early-chunk-cell-details'
import type {
  EarlyChunkBand,
  EarlyChunkCell,
  EarlyChunkGrid,
} from './early-chunk-monitor-api'

export function EarlyChunkGridView(props: { grid: EarlyChunkGrid }) {
  const { t, i18n } = useTranslation()
  const locale = toIntlLocale(i18n.resolvedLanguage || i18n.language)
  const [selected, setSelected] = useState<number | null>(null)
  const visible = useMemo(() => {
    const cells = new Map<number, EarlyChunkCell>()
    const tokenBands = new Set<number>()
    const cacheBands = new Set<number>()
    for (const cell of props.grid.cells) {
      if (cell.metrics.samples <= 0) continue
      cells.set(cell.id, cell)
      tokenBands.add(cell.token_band)
      cacheBands.add(cell.cache_band)
    }
    return {
      cells,
      tokenBands: props.grid.token_bands.filter((band) =>
        tokenBands.has(band.index)
      ),
      cacheBands: props.grid.cache_bands.filter((band) =>
        cacheBands.has(band.index)
      ),
    }
  }, [props.grid.cells, props.grid.token_bands, props.grid.cache_bands])
  const cell = selected == null ? undefined : visible.cells.get(selected)
  const tokenLabel = (band: EarlyChunkBand): string => {
    if (band.unknown) return t('Unknown')
    if (band.max == null) return '> 512k'
    return `${formatNumber(band.min / 1024, locale)}–${formatNumber(band.max / 1024, locale)}k`
  }
  const cacheLabel = (band: EarlyChunkBand): string => {
    if (band.unknown) return t('Unknown')
    return `${formatNumber(band.min / 100, locale)}–${formatNumber((band.max ?? 10000) / 100, locale)}%`
  }
  const time = (ms: number | null): string =>
    ms == null ? '—' : `${formatNumber(ms / 1000, locale)}s`
  return (
    <div className='bg-card flex min-h-0 flex-1 flex-col gap-3 overflow-y-auto rounded-xl border p-3'>
      <div className='flex shrink-0 items-start justify-between gap-3'>
        <details className='min-w-0 flex-1'>
          <summary className='cursor-pointer text-sm font-medium'>
            {t('Overview')} · {t('Samples')}:{' '}
            {formatNumber(props.grid.summary.samples, locale)} · p50{' '}
            {time(props.grid.summary.p50_ms)} · p90{' '}
            {time(props.grid.summary.p90_ms)}
          </summary>
          <div className='mt-3 space-y-3'>
            <EarlyChunkMetricsDisplay
              metrics={props.grid.summary}
              className='gap-x-4 gap-y-2 lg:grid-cols-6 2xl:grid-cols-8'
            />
            <p className='text-muted-foreground text-xs'>
              {t(
                'Storage: {{backend}} · {{active}}/{{limit}} active cells globally · {{size}} KiB table payload',
                {
                  backend: props.grid.backend,
                  active: formatNumber(props.grid.tracked_buckets, locale),
                  limit: formatNumber(props.grid.max_buckets, locale),
                  size: formatNumber(props.grid.payload_bytes / 1024, locale),
                }
              )}
            </p>
            <p className='text-muted-foreground text-xs'>
              {t(
                'This node: {{reads}} read errors · {{writes}} write errors · {{evictions}} evictions',
                {
                  reads: formatNumber(props.grid.node_read_errors, locale),
                  writes: formatNumber(props.grid.node_write_errors, locale),
                  evictions: formatNumber(props.grid.node_evictions, locale),
                }
              )}
            </p>
          </div>
        </details>
        <LearnMore
          triggerProps={{ className: 'shrink-0' }}
          contentProps={{
            side: 'bottom',
            align: 'end',
            className:
              'max-h-[70dvh] w-96 max-w-[calc(100vw-2rem)] overflow-y-auto',
          }}
        >
          <div className='text-muted-foreground space-y-1 text-xs'>
            <p>
              {t(
                'Each sample is one channel attempt. First content includes text, reasoning and tool calls.'
              )}
            </p>
            <p>
              {t(
                'Advance {{ratio}}% of the TTFT estimate, bounded to {{min}}–{{max}}s; earliest pre-emission {{earliest}}s. Gate: {{samples}} pending samples and {{probability}}%.',
                {
                  ratio: formatNumber(
                    props.grid.config.lead_ratio * 100,
                    locale
                  ),
                  min: formatNumber(
                    props.grid.config.min_lead_ms / 1000,
                    locale
                  ),
                  max: formatNumber(
                    props.grid.config.max_lead_ms / 1000,
                    locale
                  ),
                  earliest: formatNumber(
                    props.grid.config.earliest_ms / 1000,
                    locale
                  ),
                  samples: formatNumber(props.grid.config.min_samples, locale),
                  probability: formatNumber(
                    props.grid.config.min_probability * 100,
                    locale
                  ),
                }
              )}
            </p>
            <p>
              {props.grid.probe_ms < 0
                ? t(
                    'Each bucket is evaluated at its candidate time. Enter elapsed time to inspect a request that is still waiting.'
                  )
                : t(
                    'Evaluating after {{elapsed}}s of waiting. Forecasts and advance budgets update with elapsed time.',
                    {
                      elapsed: formatNumber(props.grid.probe_ms / 1000, locale),
                    }
                  )}
            </p>
            <p>
              {t(
                'Cache ratio is a local prefix-byte reuse estimate, not provider-reported cache hits. Quantiles are histogram upper bounds. Finished requests only; refreshes every 10s.'
              )}
            </p>
            <p>
              {t(
                'Three rotating time slices, up to {{seconds}}s retention. Green cells pass the prediction gate; amber cells lack samples. Click a populated cell for details.',
                {
                  seconds: formatNumber(
                    props.grid.config.max_age_seconds,
                    locale
                  ),
                }
              )}
            </p>
            <p>
              {t(
                'Prediction eligibility does not mean pre-emission is enabled. The pool and channel mode still applies.'
              )}
            </p>
          </div>
        </LearnMore>
      </div>
      {visible.cells.size === 0 && (
        <p role='status' className='bg-muted rounded-lg p-3 text-sm'>
          {t(
            'No observations yet. Off mode also collects samples without sending empty chunks.'
          )}
        </p>
      )}
      {visible.cells.size > 0 && (
        <div
          role='region'
          aria-label={t('Input tokens by prefix reuse ratio')}
          tabIndex={0}
          className='min-h-48 flex-1 overflow-auto rounded-lg border max-xl:max-h-[55vh]'
        >
          <Table
            withContainer={false}
            className='w-max min-w-full text-xs [&_td]:p-1 [&_th]:text-xs'
          >
            <TableHeader className='bg-card sticky top-0 z-20'>
              <TableRow>
                <TableHead className='bg-card sticky left-0 z-30 min-w-28'>
                  {t('Input tokens')}
                </TableHead>
                {visible.cacheBands.map((band) => (
                  <TableHead
                    key={band.index}
                    scope='col'
                    className='min-w-28 text-center'
                  >
                    {cacheLabel(band)}
                  </TableHead>
                ))}
              </TableRow>
            </TableHeader>
            <TableBody>
              {visible.tokenBands.map((band) => (
                <TableRow key={band.index}>
                  <TableHead
                    scope='row'
                    className='bg-card sticky left-0 z-10 whitespace-nowrap'
                  >
                    {tokenLabel(band)}
                  </TableHead>
                  {visible.cacheBands.map((cache) => {
                    const id = band.index * 21 + cache.index
                    const entry = visible.cells.get(id)
                    const label = `${tokenLabel(band)} / ${cacheLabel(cache)}`
                    if (!entry) {
                      return (
                        <TableCell
                          key={id}
                          className='text-muted-foreground text-center'
                          aria-label={`${label}: ${t('No Data')}`}
                        >
                          —
                        </TableCell>
                      )
                    }
                    return (
                      <TableCell key={id}>
                        <Button
                          variant='ghost'
                          aria-label={label}
                          aria-pressed={selected === id}
                          onClick={() => setSelected(id)}
                          className={cn(
                            'h-auto min-h-16 w-full flex-col items-start gap-0 rounded-md px-2 py-1 text-left tabular-nums',
                            entry.prediction.eligible &&
                              'bg-emerald-500/15 hover:bg-emerald-500/25',
                            entry.prediction.pending <
                              props.grid.config.min_samples &&
                              'bg-amber-500/10 hover:bg-amber-500/20',
                            selected === id && 'ring-2 ring-primary'
                          )}
                        >
                          <span>
                            n={formatNumber(entry.metrics.samples, locale)}
                          </span>
                          <span>p50 {time(entry.metrics.p50_ms)}</span>
                          <span>p90 {time(entry.metrics.p90_ms)}</span>
                          <span>
                            {t('Advance {{time}}', {
                              time: time(entry.prediction.lead_ms),
                            })}
                          </span>
                          <span>
                            {t('Candidate {{time}}', {
                              time: time(entry.prediction.candidate_ms),
                            })}
                          </span>
                          <span>
                            {formatNumber(
                              entry.prediction.probability * 100,
                              locale
                            )}
                            %
                          </span>
                        </Button>
                      </TableCell>
                    )
                  })}
                </TableRow>
              ))}
            </TableBody>
          </Table>
        </div>
      )}
      <Dialog
        open={!!cell}
        onOpenChange={(open) => {
          if (!open) setSelected(null)
        }}
        title={t('Bucket details')}
      >
        {cell && (
          <EarlyChunkCellDetails
            cell={cell}
            grid={props.grid}
            label={`${tokenLabel(props.grid.token_bands[cell.token_band])} / ${cacheLabel(props.grid.cache_bands[cell.cache_band])}`}
          />
        )}
      </Dialog>
      {!cell && visible.cells.size > 0 && (
        <p className='text-muted-foreground shrink-0 text-sm'>
          {t('Select a populated cell to inspect its measurements.')}
        </p>
      )}
    </div>
  )
}
