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
import {
  flexRender,
  getCoreRowModel,
  useReactTable,
} from '@tanstack/react-table'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { expect, it, vi } from 'vitest'

import type { TaskLog } from '../../../types'
import { UsageLogsMobileList } from '../../usage-logs-mobile-card'
import { useTaskLogsColumns } from '../task-logs-columns'

const task: TaskLog = {
  id: 1,
  user_id: 1,
  platform: 'seedance-sls',
  task_id: 'task_public_seedance_123456789',
  group: 'default',
  quota: 100,
  action: 'unknown',
  channel_id: 1,
  submit_time: 1,
  status: 'SUCCESS',
  fail_reason: '',
}

function TaskIdFixture(props: {
  isAdmin: boolean
  layout: 'desktop' | 'mobile'
  task?: TaskLog
}) {
  const columns = useTaskLogsColumns(props.isAdmin, false).filter(
    (column) => 'accessorKey' in column && column.accessorKey === 'task_id'
  )
  const table = useReactTable({
    data: [props.task ?? task],
    columns,
    getCoreRowModel: getCoreRowModel(),
  })

  if (props.layout === 'mobile') {
    return <UsageLogsMobileList table={table} logCategory='task' />
  }

  const cell = table.getRowModel().rows[0].getVisibleCells()[0]
  return <div>{flexRender(cell.column.columnDef.cell, cell.getContext())}</div>
}

it.each(['desktop', 'mobile'] as const)(
  'shows only the task ID to ordinary users in the %s view',
  (layout) => {
    render(<TaskIdFixture isAdmin={false} layout={layout} />)

    expect(screen.getByText(task.task_id)).toBeVisible()
    expect(screen.queryByText(/seedance-sls|Unknown/)).not.toBeInTheDocument()
  }
)

it.each(['desktop', 'mobile'] as const)(
  'retains the platform and action for admins in the %s view',
  (layout) => {
    render(
      <TaskIdFixture
        isAdmin
        layout={layout}
        task={{ ...task, platform: 'moyu-wan3', action: 'textGenerate' }}
      />
    )

    expect(screen.getByText(task.task_id)).toBeVisible()
    expect(screen.getByText('moyu-wan3 · Text to Video')).toBeVisible()
  }
)

it.each(['desktop', 'mobile'] as const)(
  'copies the complete task ID for ordinary users in the %s view',
  async (layout) => {
    const user = userEvent.setup()
    const copy = vi.spyOn(navigator.clipboard, 'writeText').mockResolvedValue()
    render(<TaskIdFixture isAdmin={false} layout={layout} />)

    await user.click(screen.getByTitle(`Click to copy: ${task.task_id}`))

    expect(copy).toHaveBeenCalledWith(task.task_id)
  }
)
