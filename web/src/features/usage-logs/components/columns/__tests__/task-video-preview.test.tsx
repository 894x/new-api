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
import assert from 'node:assert/strict'

import type { CellContext, Row } from '@tanstack/react-table'
import { Window } from 'happy-dom'
import type React from 'react'
import { afterAll, afterEach, describe, test } from 'vitest'

import type { TaskLog } from '../../../types'

const domWindow = new Window()
const domGlobals = [
  'window',
  'document',
  'navigator',
  'HTMLElement',
  'SVGElement',
  'Node',
  'Element',
  'Event',
  'CustomEvent',
  'MutationObserver',
  'requestAnimationFrame',
  'cancelAnimationFrame',
  'getComputedStyle',
] as const

for (const key of domGlobals) {
  Object.defineProperty(globalThis, key, {
    configurable: true,
    value: domWindow[key],
  })
}

const { act } = await import('react')
const { createRoot } = await import('react-dom/client')
const { createInstance } = await import('i18next')
const { I18nextProvider, initReactI18next } = await import('react-i18next')
const { useTaskLogsColumns } = await import('../task-logs-columns')

const i18n = createInstance()
await i18n.use(initReactI18next).init({
  lng: 'en',
  resources: {
    en: {
      translation: {
        Details: 'Details',
        'Click to preview video': 'Click to preview video',
        'View details': 'View details',
        Video: 'Video',
        Preview: 'Preview',
        'Raw JSON': 'Raw JSON',
        'Loading...': 'Loading...',
        'Request failed': 'Request failed',
        'Open in new tab': 'Open in new tab',
        Download: 'Download',
      },
    },
  },
})

const reactTestGlobals = globalThis as typeof globalThis & {
  IS_REACT_ACT_ENVIRONMENT?: boolean
}
reactTestGlobals.IS_REACT_ACT_ENVIRONMENT = true

function TaskDetailsCell(props: {
  log: TaskLog
  isAdminView?: boolean
  canViewRawData?: boolean
}) {
  const columns = useTaskLogsColumns(
    Boolean(props.isAdminView),
    Boolean(props.canViewRawData)
  )
  const detailsColumn = columns.find(
    (column) => 'accessorKey' in column && column.accessorKey === 'fail_reason'
  )

  assert.ok(detailsColumn)
  assert.equal(typeof detailsColumn.cell, 'function')
  const Cell = detailsColumn.cell as React.ComponentType<
    CellContext<TaskLog, unknown>
  >
  const row = {
    original: props.log,
    getValue: (key: string) => props.log[key as keyof TaskLog],
  } as Row<TaskLog>
  const context = { row } as CellContext<TaskLog, unknown>

  return <Cell {...context} />
}

describe('task video preview', () => {
  afterEach(() => {
    document.body.replaceChildren()
  })

  afterAll(() => {
    domWindow.close()
  })

  test('previews a successful video task from the returned result URL', async () => {
    const container = document.createElement('div')
    document.body.append(container)
    const root = createRoot(container)
    const log: TaskLog = {
      id: 1,
      user_id: 1,
      platform: 'doubao',
      task_id: 'task/id?source=1',
      action: 'textGenerate',
      channel_id: 1,
      submit_time: 1,
      status: 'SUCCESS',
      fail_reason: '',
      result_url: 'https://example.com/video.mp4',
      data: { status: 'succeeded', detail: { seed: 42 } },
    }
    await act(async () => {
      root.render(
        <I18nextProvider i18n={i18n}>
          <TaskDetailsCell log={log} isAdminView canViewRawData />
        </I18nextProvider>
      )
    })

    const previewButton = [...container.querySelectorAll('button')].find(
      (button) => button.textContent === 'View details'
    )
    assert.ok(previewButton)

    await act(async () => {
      previewButton.click()
    })

    const video = document.body.querySelector('video')
    assert.ok(video)
    assert.equal(video.getAttribute('src'), 'https://example.com/video.mp4')
    assert.equal(
      document.body.querySelectorAll('a[href="https://example.com/video.mp4"]')
        .length,
      1
    )

    const rawJsonTab = [...document.body.querySelectorAll('button')].find(
      (button) => button.textContent === 'Raw JSON'
    )
    assert.ok(rawJsonTab)
    await act(async () => rawJsonTab.click())
    assert.match(
      document.body.querySelector('pre')?.textContent || '',
      /"seed": 42/
    )

    await act(async () => root.unmount())
    container.remove()
  })

  test('keeps raw task JSON available when an admin views only their tasks', async () => {
    const container = document.createElement('div')
    document.body.append(container)
    const root = createRoot(container)
    const log: TaskLog = {
      id: 4,
      user_id: 1,
      platform: 'doubao',
      task_id: 'task_admin_self_view',
      action: 'textGenerate',
      channel_id: 1,
      submit_time: 1,
      status: 'SUCCESS',
      fail_reason: '',
      result_url: 'https://example.com/admin-self-view.mp4',
      data: { status: 'succeeded', detail: { seed: 99 } },
    }

    await act(async () => {
      root.render(
        <I18nextProvider i18n={i18n}>
          <TaskDetailsCell log={log} canViewRawData />
        </I18nextProvider>
      )
    })

    const previewButton = [...container.querySelectorAll('button')].find(
      (button) => button.textContent === 'View details'
    )
    assert.ok(previewButton)
    await act(async () => previewButton.click())

    assert.ok(
      [...document.body.querySelectorAll('button')].some(
        (button) => button.textContent === 'Raw JSON'
      )
    )

    await act(async () => root.unmount())
    container.remove()
  })

  test('does not expose raw task JSON to non-admin users', async () => {
    const container = document.createElement('div')
    document.body.append(container)
    const root = createRoot(container)
    const log: TaskLog = {
      id: 3,
      user_id: 1,
      platform: 'doubao',
      task_id: 'task_non_admin',
      action: 'textGenerate',
      channel_id: 1,
      submit_time: 1,
      status: 'SUCCESS',
      fail_reason: '',
      result_url: 'https://example.com/non-admin-video.mp4',
      data: { status: 'succeeded', internal: { seed: 7 } },
    }

    await act(async () => {
      root.render(
        <I18nextProvider i18n={i18n}>
          <TaskDetailsCell log={log} />
        </I18nextProvider>
      )
    })

    const previewButton = [...container.querySelectorAll('button')].find(
      (button) => button.textContent === 'Click to preview video'
    )
    assert.ok(previewButton)
    await act(async () => previewButton.click())

    assert.equal(
      [...document.body.querySelectorAll('button')].some(
        (button) => button.textContent === 'Raw JSON'
      ),
      false
    )
    assert.equal(document.body.textContent?.includes('"seed": 7'), false)

    await act(async () => root.unmount())
    container.remove()
  })

  test('keeps the empty placeholder for missing or unsafe result URLs', async () => {
    for (const resultUrl of [
      '   ',
      'javascript:alert(1)',
      'https://gateway.example.com/v1/videos/task_123/content',
    ]) {
      const container = document.createElement('div')
      document.body.append(container)
      const root = createRoot(container)
      const log: TaskLog = {
        id: 2,
        user_id: 1,
        platform: 'doubao',
        task_id: 'task_without_result',
        action: 'textGenerate',
        channel_id: 1,
        submit_time: 1,
        status: 'SUCCESS',
        fail_reason: '',
        result_url: resultUrl,
      }

      await act(async () => {
        root.render(
          <I18nextProvider i18n={i18n}>
            <TaskDetailsCell log={log} />
          </I18nextProvider>
        )
      })

      assert.equal(container.textContent, '-')
      assert.equal(container.querySelector('button'), null)

      await act(async () => root.unmount())
      container.remove()
    }
  })
})
