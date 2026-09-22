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
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import type { CellContext } from '@tanstack/react-table'
import { cleanup, render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import type { ComponentType } from 'react'
import { afterEach, expect, test, vi } from 'vitest'

import { api } from '@/lib/api'

import type { TaskLog } from '../../../types'
import { UsageLogsProvider } from '../../usage-logs-provider'
import { useTaskLogsColumns } from '../task-logs-columns'

vi.mock('react-i18next', () => ({
  useTranslation: () => ({ t: (key: string) => key }),
}))

afterEach(() => {
  cleanup()
  vi.restoreAllMocks()
})

const log: TaskLog = {
  id: 1,
  user_id: 1,
  platform: 'doubao',
  task_id: 'task/id?source=1',
  group: 'default',
  quota: 100,
  action: 'textGenerate',
  channel_id: 1,
  submit_time: 1,
  status: 'SUCCESS',
  fail_reason: '',
  result_url: 'https://provider.example/private-video.mp4',
  data: { status: 'succeeded', detail: { seed: 42 } },
}

function TaskCells(props: {
  canViewRawData: boolean
  isAdminView?: boolean
  task?: TaskLog
}) {
  const task = props.task ?? log
  const columns = useTaskLogsColumns(
    Boolean(props.isAdminView),
    false,
    props.canViewRawData
  )
  return (
    <>
      {columns
        .filter(
          (column) =>
            column.id === 'artifacts' ||
            ('accessorKey' in column && column.accessorKey === 'fail_reason')
        )
        .map((column) => {
          const Cell = column.cell as ComponentType<
            CellContext<TaskLog, unknown>
          >
          return (
            <Cell
              key={column.id ?? 'details'}
              {...({
                row: {
                  original: task,
                  getValue: (key: string) => task[key as keyof TaskLog],
                },
              } as CellContext<TaskLog, unknown>)}
            />
          )
        })}
    </>
  )
}

function renderTask(canViewRawData: boolean, isAdminView = false, task = log) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false, gcTime: 0 } },
  })
  return render(
    <QueryClientProvider client={client}>
      <UsageLogsProvider>
        <TaskCells
          canViewRawData={canViewRawData}
          isAdminView={isAdminView}
          task={task}
        />
      </UsageLogsProvider>
    </QueryClientProvider>
  )
}

test('previews an owned completed video through the authenticated artifact projection', async () => {
  const contentUrl = `https://gateway.example/v1/tasks/task-public/artifacts/video/content?access=${'a'.repeat(43)}`
  const get = vi.spyOn(api, 'get').mockResolvedValue({
    data: {
      success: true,
      data: {
        artifacts: [{ key: 'video', type: 'video', content_url: contentUrl }],
      },
    },
  })
  renderTask(true, true)
  expect(get).not.toHaveBeenCalled()
  await userEvent.click(screen.getByRole('button', { name: 'Artifacts' }))
  await waitFor(() =>
    expect(document.querySelector('video')).toHaveAttribute('src', contentUrl)
  )
  expect(get).toHaveBeenCalledWith(
    '/api/task/task%2Fid%3Fsource%3D1/artifacts',
    expect.any(Object)
  )
  expect(screen.getByRole('button', { name: 'Download' })).toHaveAttribute(
    'href',
    contentUrl
  )
  expect(document.body.innerHTML).not.toContain('provider.example')
})

test.each([true, false])(
  'keeps raw task JSON available to an authorized admin with all-users view=%s',
  async (isAdminView) => {
    renderTask(true, isAdminView)
    await userEvent.click(screen.getByRole('button', { name: 'View details' }))
    expect(await screen.findByLabelText('Raw JSON')).toHaveTextContent(
      '"seed": 42'
    )
  }
)

test('does not expose raw task JSON to ordinary users', async () => {
  renderTask(false)
  await userEvent.click(screen.getByRole('button', { name: 'View details' }))
  expect(await screen.findByText('Task Details')).toBeVisible()
  expect(screen.queryByLabelText('Raw JSON')).toBeNull()
  expect(document.body.textContent).not.toContain('"seed": 42')
})

test.each([
  '',
  'javascript:alert(1)',
  'https://gateway.example.com/v1/videos/task_123/content',
])(
  'does not use an unprojected result URL as a media source: %s',
  async (resultUrl) => {
    vi.spyOn(api, 'get').mockResolvedValue({
      data: { success: true, data: { artifacts: [] } },
    })
    renderTask(false, false, { ...log, result_url: resultUrl })
    await userEvent.click(screen.getByRole('button', { name: 'Artifacts' }))
    expect(await screen.findByText('None')).toBeVisible()
    expect(document.querySelector('video')).toBeNull()
    expect(screen.queryByRole('button', { name: 'Download' })).toBeNull()
  }
)
