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
import { CardHeader, CardTitle, CardDescription } from '@/components/ui/card'

export function PerformanceChartHeader(props: {
  title: string
  description?: string
  formatValue: (value: number) => string
  statistics?: Array<{ label: string; value?: number }>
}) {
  return (
    <CardHeader
      className={
        props.statistics
          ? 'flex min-h-11 flex-nowrap items-start justify-between gap-2'
          : 'min-h-11'
      }
    >
      <div className='min-w-0 shrink-0 space-y-1'>
        <CardTitle>{props.title}</CardTitle>
        {props.description && (
          <CardDescription className='text-xs'>
            {props.description}
          </CardDescription>
        )}
      </div>
      {props.statistics && (
        <dl className='ml-auto flex min-w-0 flex-nowrap justify-end gap-1 text-right'>
          {props.statistics.map(({ label, value }) => {
            const displayValue =
              value === undefined ? '—' : props.formatValue(value)
            return (
              <div
                key={label}
                className='bg-muted/60 min-w-0 rounded-md px-0.5 py-1'
              >
                <dt className='text-muted-foreground text-[10px]'>{label}</dt>
                <dd
                  title={displayValue}
                  className='truncate font-mono text-[10px] font-semibold whitespace-nowrap tabular-nums'
                >
                  {displayValue}
                </dd>
              </div>
            )
          })}
        </dl>
      )}
    </CardHeader>
  )
}
