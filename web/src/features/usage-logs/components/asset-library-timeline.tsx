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

import type { AssetLibraryTiming } from '../asset-library-timing'
import { formatTimingMilliseconds } from '../lib/request-timing'
import { TimingTimeline, type TimingSegment } from './timing-timeline'

export function AssetLibraryTimeline(props: { timing: AssetLibraryTiming }) {
  const { t } = useTranslation()
  const timing = props.timing
  if (timing.version !== 1) return null
  const definitions = [
    {
      key: 'source_download',
      label: t('Source Download'),
      colorClass: 'bg-cyan-500 dark:bg-cyan-400',
    },
    {
      key: 'media_inspection',
      label: t('Media Inspection'),
      colorClass: 'bg-violet-500 dark:bg-violet-400',
    },
    {
      key: 'local_persistence',
      label: t('Local Persistence'),
      colorClass: 'bg-slate-500 dark:bg-slate-400',
    },
    {
      key: 'channel_lock',
      label: t('Channel Lock Wait'),
      colorClass: 'bg-amber-500 dark:bg-amber-400',
    },
    {
      key: 'upstream_request',
      label: t('Asset Upstream Requests'),
      colorClass: 'bg-blue-500 dark:bg-blue-400',
    },
  ]
  const stages = Array.isArray(timing.stages)
    ? timing.stages.filter(
        (stage) =>
          Number.isFinite(stage.duration_ms) &&
          stage.duration_ms >= 0 &&
          Number.isFinite(stage.started_at_ms) &&
          stage.started_at_ms > 0
      )
    : []
  const segments: TimingSegment[] = definitions.flatMap((definition) => {
    const matching = stages.filter((stage) => stage.stage === definition.key)
    if (!matching.length) return []
    return [
      {
        ...definition,
        durationMs: matching.reduce(
          (total, stage) => total + stage.duration_ms,
          0
        ),
      },
    ]
  })
  const totalDurationMs = Number.isFinite(timing.duration_ms)
    ? Math.max(0, timing.duration_ms)
    : 0
  const measuredMs = segments.reduce(
    (sum, segment) => sum + segment.durationMs,
    0
  )
  if (totalDurationMs > measuredMs) {
    segments.push({
      key: 'gateway_processing',
      label: t('Other Gateway Processing'),
      colorClass: 'bg-slate-400',
      durationMs: totalDurationMs - measuredMs,
    })
  }
  return (
    <div className='min-w-0 space-y-3'>
      <TimingTimeline
        title={t('Request Timeline')}
        totalDurationMs={totalDurationMs}
        segments={segments}
      />
      <div className='text-muted-foreground flex flex-wrap gap-x-3 gap-y-1 text-xs'>
        <span>
          {t('Request ID')}:{' '}
          <span className='font-mono break-all'>{timing.request_id}</span>
        </span>
        <span>
          {t('Status')}:{' '}
          {timing.outcome === 'failed' ? t('Failed') : t('Succeeded')}
        </span>
        {timing.asset_id && (
          <span className='break-all'>{timing.asset_id}</span>
        )}
      </div>
      {(timing.dropped_stages ?? 0) > 0 && (
        <p className='text-xs text-amber-600'>
          {t(
            'Stage details truncated; see server logs for the complete trace.'
          )}
        </p>
      )}
      {stages.length > 0 && (
        <details className='min-w-0 rounded-md border p-2.5'>
          <summary className='cursor-pointer text-xs font-medium'>
            {t('Stage Details')}
          </summary>
          <ol className='mt-2 space-y-2 text-xs'>
            {stages.map((stage) => (
              <li
                key={
                  stage.id ??
                  `${stage.started_at_ms}-${stage.stage}-${stage.channel_id}`
                }
                className='min-w-0 border-t pt-2'
              >
                <div className='flex flex-wrap justify-between gap-2'>
                  <span>
                    {definitions.find((item) => item.key === stage.stage)
                      ?.label ?? t('Other Gateway Processing')}{' '}
                    {stage.operation}
                  </span>
                  <span className='font-mono'>
                    {formatTimingMilliseconds(stage.duration_ms)} ·{' '}
                    {stage.outcome === 'failed' ? t('Failed') : t('Succeeded')}
                  </span>
                </div>
                <div className='text-muted-foreground mt-1 flex flex-wrap gap-x-3 gap-y-1 break-all'>
                  <span>
                    +
                    {formatTimingMilliseconds(
                      Math.max(0, stage.started_at_ms - timing.started_at_ms)
                    )}
                  </span>
                  {!!stage.channel_id && (
                    <span>
                      {t('Channel')}: {stage.channel_id}
                    </span>
                  )}
                  {stage.asset_id && <span>{stage.asset_id}</span>}
                  {!!stage.http_status && (
                    <span>
                      HTTP <span>{stage.http_status}</span>
                    </span>
                  )}
                  {stage.error_code && (
                    <span>
                      {t('Error Code')}: {stage.error_code}
                    </span>
                  )}
                  {stage.error_kind && (
                    <span>
                      {t('Error')}:{' '}
                      {stage.error_kind === 'timeout' && t('Request timed out')}
                      {stage.error_kind === 'canceled' && t('Request canceled')}
                      {stage.error_kind !== 'timeout' &&
                        stage.error_kind !== 'canceled' &&
                        t('Operation failed')}
                    </span>
                  )}
                  {stage.upstream_request_id && (
                    <span>
                      {t('Upstream Request ID')}:{' '}
                      <span>{stage.upstream_request_id}</span>
                    </span>
                  )}
                  {!!stage.held_ms && (
                    <span>
                      {t('Channel Lock Held')}:{' '}
                      {formatTimingMilliseconds(stage.held_ms)}
                    </span>
                  )}
                  {stage.backend && <span>{stage.backend}</span>}
                  {!!stage.bytes && <span>{stage.bytes} B</span>}
                </div>
              </li>
            ))}
          </ol>
        </details>
      )}
      {(timing.replicas ?? []).map((replica) => {
        const submitted = replica.submitted_at_ms ?? 0
        const observed = replica.first_active_at_ms ?? 0
        const end =
          observed || replica.last_polled_at_ms || timing.completed_at_ms
        const lifecycle: TimingSegment[] = []
        const upload = replica.upload_started_at_ms ?? 0
        if (upload > 0 && submitted >= upload) {
          lifecycle.push({
            key: 'submission',
            label: t('Upload to Upstream Acceptance'),
            durationMs: submitted - upload,
            colorClass: 'bg-cyan-500',
          })
        }
        if (submitted > 0 && end >= submitted) {
          lifecycle.push({
            key: 'activation',
            label: observed
              ? t('First Active Observation')
              : t('Waiting for Active Observation'),
            durationMs: end - submitted,
            colorClass: 'bg-amber-500',
          })
        }
        return (
          <div
            key={`${replica.asset_id}-${replica.channel_id}`}
            className='min-w-0 space-y-1'
          >
            <p className='text-xs break-all'>
              {t('Channel')} {replica.channel_id} · {replica.asset_id} ·{' '}
              {t('Status')}: {replica.status === 'ready' && t('Active')}
              {replica.status === 'failed' && t('Failed')}
              {replica.status !== 'ready' &&
                replica.status !== 'failed' &&
                t('Processing')}
            </p>
            <TimingTimeline
              title={t('Asset Activation Timeline')}
              segments={lifecycle}
              totalDurationMs={lifecycle.reduce(
                (sum, item) => sum + item.durationMs,
                0
              )}
            />
            {!submitted && (
              <p className='text-muted-foreground text-xs'>
                {t('Submission timing unavailable for this asset.')}
              </p>
            )}
            <p className='text-muted-foreground text-xs'>
              {t('Poll Count')}: {replica.poll_count} ·{' '}
              {t(
                'Includes polling intervals; not pure upstream processing time.'
              )}
            </p>
            {!!replica.last_processing_at_ms && (
              <p className='text-muted-foreground text-xs'>
                {t('Last Processing Observation')}:{' '}
                {new Date(replica.last_processing_at_ms).toLocaleString()}
              </p>
            )}
            {!!replica.last_polled_at_ms && (
              <p className='text-muted-foreground text-xs'>
                {t('Last Status Check')}:{' '}
                {new Date(replica.last_polled_at_ms).toLocaleString()}
              </p>
            )}
          </div>
        )
      })}
    </div>
  )
}
