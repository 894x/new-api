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

// @vitest-environment happy-dom
import type { CellContext } from '@tanstack/react-table'
import { cleanup, render, screen } from '@testing-library/react'
import type { ComponentType } from 'react'
import { afterEach, expect, it, vi } from 'vitest'

import { Route } from '@/routes/_authenticated/usage-logs/$section'

import { LOG_TYPE_FILTERS } from '../../constants'
import { usageLogSchema, type UsageLog } from '../../data/schema'
import { useCommonLogsColumns } from '../columns/common-logs-columns'

vi.mock('react-i18next', () => ({
  useTranslation: () => ({ t: (key: string) => key }),
}))
vi.mock('@/lib/lobe-icon', () => ({ getLobeIcon: () => null }))
vi.mock('@/features/usage-logs', () => ({ UsageLogs: () => null }))
afterEach(cleanup)

function LogCells(props: { log: UsageLog }) {
  const columns = useCommonLogsColumns(false)
  return (
    <>
      {columns
        .filter(
          (c) =>
            'accessorKey' in c &&
            ['created_at', 'content'].includes(c.accessorKey as string)
        )
        .map((c) => {
          const Cell = c.cell as ComponentType<CellContext<UsageLog, unknown>>
          const context = {
            row: { original: props.log, getValue: () => props.log.created_at },
          } as unknown as CellContext<UsageLog, unknown>
          return (
            <Cell
              key={'accessorKey' in c ? String(c.accessorKey) : c.id}
              {...context}
            />
          )
        })}
    </>
  )
}

it.each([
  [
    8,
    'Asset upload',
    'asset_library.asset.create',
    'Created asset (ID: {{id}}, type: {{asset_type}}, group: {{group_id}})',
  ],
  [
    9,
    'Asset deletion',
    'asset_library.asset.delete',
    'Deleted asset (ID: {{id}}, type: {{asset_type}}, group: {{group_id}})',
  ],
  [
    10,
    'Asset update',
    'asset_library.asset.update',
    'Updated asset (ID: {{id}}, type: {{asset_type}}, group: {{group_id}})',
  ],
  [
    11,
    'Asset group creation',
    'asset_library.group.create',
    'Created asset group {{name}} (ID: {{id}})',
  ],
  [
    12,
    'Asset synchronization',
    'asset_library.asset.sync',
    'Synchronized asset {{id}} (errors: {{error_count}})',
  ],
] as const)(
  'shows category %s and operation summary directly in the list',
  (type, label, action, summary) => {
    const log = usageLogSchema.parse({
      id: 1,
      user_id: 1,
      created_at: 1,
      type,
      content: 'raw fallback',
      other: JSON.stringify({ op: { action, params: {} } }),
    })
    render(<LogCells log={log} />)
    expect(screen.getByText(label)).toBeTruthy()
    expect(screen.getByText(summary)).toBeTruthy()
    expect(screen.queryByText('Manage')).toBeNull()
    expect(
      LOG_TYPE_FILTERS.find((item) => item.value === String(type))?.label
    ).toBe(label)
    const schema = Route.options.validateSearch as {
      parse: (input: unknown) => { type: string[] }
    }
    expect(schema.parse({ type: String(type) }).type).toEqual([String(type)])
  }
)
