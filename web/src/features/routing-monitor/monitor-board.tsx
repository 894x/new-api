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
import { useState, type CSSProperties } from 'react'
import { useTranslation } from 'react-i18next'

import type { MonitorLayout } from './monitor-layout-picker'
import { MonitorPanel, type MonitorTarget } from './monitor-panel'

export function MonitorBoard(props: {
  layout: MonitorLayout
  userId?: number
}) {
  const { t } = useTranslation()
  const [targets, setTargets] = useState<
    Record<string, MonitorTarget | undefined>
  >({})

  return (
    <div
      role='region'
      aria-label={t('Multi-panel monitoring')}
      className='grid min-h-0 min-w-0 flex-1 [scrollbar-width:none] auto-rows-[32rem] gap-2 overflow-auto overscroll-contain p-1 md:grid-cols-[repeat(var(--monitor-columns),minmax(0,1fr))] md:grid-rows-[repeat(var(--monitor-rows),minmax(0,1fr))] [&::-webkit-scrollbar]:hidden'
      style={
        {
          '--monitor-columns': props.layout.columns,
          '--monitor-rows': props.layout.rows,
        } as CSSProperties
      }
    >
      {Array.from({ length: props.layout.rows }, (_, row) =>
        Array.from({ length: props.layout.columns }, (_, column) => {
          const key = `${row}:${column}`
          return (
            <MonitorPanel
              key={key}
              number={row * props.layout.columns + column + 1}
              initialUserId={props.userId}
              dense={props.layout.rows >= 3}
              target={targets[key]}
              onChange={(target) =>
                setTargets((previous) => ({ ...previous, [key]: target }))
              }
            />
          )
        })
      )}
    </div>
  )
}
