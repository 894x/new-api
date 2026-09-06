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

import type { TaskUsage } from '../types'

interface UsageCellProps {
  usage?: TaskUsage | null
  promptTokens?: number
  completionTokens?: number
  cacheReadTokens?: number
  cacheWriteTokens?: number
}

function formatUsageValue(value: number): string {
  return value.toLocaleString(undefined, { maximumFractionDigits: 3 })
}

export function UsageCell(props: UsageCellProps) {
  const { t } = useTranslation()

  if (props.usage) {
    const unit = props.usage.unit === 'second' ? 's' : props.usage.unit

    return (
      <div className='flex flex-col gap-0.5'>
        <span className='font-mono text-xs font-medium tabular-nums'>
          <span className='text-muted-foreground'>{t('Input')}</span>{' '}
          {formatUsageValue(props.usage.input)}
          {unit}
        </span>
        <span className='font-mono text-xs font-medium tabular-nums'>
          <span className='text-muted-foreground'>{t('Output')}</span>{' '}
          {formatUsageValue(props.usage.output)}
          {unit}
        </span>
        <span className='text-muted-foreground/60 text-[11px] leading-none'>
          {t('Total')} {formatUsageValue(props.usage.total)}
          {unit}
        </span>
      </div>
    )
  }

  const promptTokens = props.promptTokens || 0
  const completionTokens = props.completionTokens || 0
  if (promptTokens === 0 && completionTokens === 0) {
    return <span className='text-muted-foreground text-xs'>-</span>
  }

  const cacheReadTokens = props.cacheReadTokens || 0
  const cacheWriteTokens = props.cacheWriteTokens || 0
  const showCache = cacheReadTokens > 0 || cacheWriteTokens > 0

  return (
    <div className='flex flex-col gap-0.5'>
      <span className='font-mono text-xs font-medium tabular-nums'>
        {promptTokens.toLocaleString()} / {completionTokens.toLocaleString()}
      </span>
      {showCache && (
        <div className='flex items-center gap-1 text-[11px]'>
          {cacheReadTokens > 0 && (
            <span className='text-muted-foreground/60'>
              {t('Cache')}↓ {cacheReadTokens.toLocaleString()}
            </span>
          )}
          {cacheWriteTokens > 0 && (
            <span className='text-muted-foreground/60'>
              ↑ {cacheWriteTokens.toLocaleString()}
            </span>
          )}
        </div>
      )}
    </div>
  )
}
