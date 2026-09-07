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

import { buildRequestTimingSegments } from '../lib/request-timing'
import type { RequestTimingInfo } from '../types'
import { TimingTimeline } from './timing-timeline'

const segmentColorClasses = {
  client_upload: 'bg-slate-400 dark:bg-slate-500',
  gateway_processing: 'bg-violet-500 dark:bg-violet-400',
  upstream_upload: 'bg-cyan-500 dark:bg-cyan-400',
  upstream_wait: 'bg-amber-500 dark:bg-amber-400',
  first_response: 'bg-blue-500 dark:bg-blue-400',
  streaming: 'bg-emerald-500 dark:bg-emerald-400',
} as const

export function RequestTimingTimeline(props: { timing: RequestTimingInfo }) {
  const { t } = useTranslation()
  const timing = buildRequestTimingSegments(props.timing)
  return (
    <TimingTimeline
      title={t('Request Timeline')}
      totalDurationMs={timing.totalDurationMs}
      segments={timing.segments.map((segment) => ({
        key: segment.key,
        label: t(segment.labelKey),
        durationMs: segment.durationMs,
        colorClass: segmentColorClasses[segment.key],
      }))}
    />
  )
}
