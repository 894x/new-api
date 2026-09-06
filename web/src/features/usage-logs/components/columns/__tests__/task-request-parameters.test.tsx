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
        'Request parameters': 'Request parameters',
        Resolution: 'Resolution',
        Duration: 'Duration',
        Ratio: 'Ratio',
      },
    },
  },
})

const reactTestGlobals = globalThis as typeof globalThis & {
  IS_REACT_ACT_ENVIRONMENT?: boolean
}
reactTestGlobals.IS_REACT_ACT_ENVIRONMENT = true

function TaskRequestParametersCell(props: { log: TaskLog }) {
  const columns = useTaskLogsColumns(false, false)
  const requestParametersColumn = columns.find(
    (column) => column.id === 'request_parameters'
  )

  assert.ok(requestParametersColumn)
  assert.equal(typeof requestParametersColumn.cell, 'function')
  const Cell = requestParametersColumn.cell as React.ComponentType<
    CellContext<TaskLog, unknown>
  >
  const row = {
    original: props.log,
    getValue: (key: string) => props.log[key as keyof TaskLog],
  } as Row<TaskLog>

  return <Cell {...({ row } as CellContext<TaskLog, unknown>)} />
}

async function renderRequestParameters(log: TaskLog) {
  const container = document.createElement('div')
  document.body.append(container)
  const root = createRoot(container)

  await act(async () => {
    root.render(
      <I18nextProvider i18n={i18n}>
        <TaskRequestParametersCell log={log} />
      </I18nextProvider>
    )
  })

  return { container, root }
}

describe('task request parameters column', () => {
  afterEach(() => {
    document.body.replaceChildren()
  })

  afterAll(() => {
    domWindow.close()
  })

  test('shows persisted resolution, duration, and ratio', async () => {
    const log = {
      id: 1,
      user_id: 1,
      platform: 'hailuo-video',
      task_id: 'task_h3_parameters',
      action: 'textGenerate',
      channel_id: 1,
      submit_time: 1,
      status: 'SUCCESS',
      properties: {
        request_parameters: {
          resolution: '2K',
          duration: 7,
          ratio: '16:9',
        },
      },
    } as TaskLog

    const { container, root } = await renderRequestParameters(log)

    assert.match(container.textContent || '', /Resolution\s*2K/)
    assert.match(container.textContent || '', /Duration\s*7s/)
    assert.match(container.textContent || '', /Ratio\s*16:9/)

    await act(async () => root.unmount())
  })

  test('falls back to the stored MiniMax response for an existing task', async () => {
    const log = {
      id: 2,
      user_id: 1,
      platform: 'hailuo-video',
      task_id: 'task_h3_existing_parameters',
      action: 'textGenerate',
      channel_id: 1,
      submit_time: 1,
      status: 'SUCCESS',
      data: {
        task: {
          resolution: '768P',
          duration: 4,
          ratio: '16:9',
        },
      },
    } as TaskLog

    const { container, root } = await renderRequestParameters(log)

    assert.match(container.textContent || '', /Resolution\s*768P/)
    assert.match(container.textContent || '', /Duration\s*4s/)
    assert.match(container.textContent || '', /Ratio\s*16:9/)

    await act(async () => root.unmount())
  })
})
