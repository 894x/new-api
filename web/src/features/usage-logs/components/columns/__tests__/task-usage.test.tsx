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
        Usage: 'Usage',
        Input: 'Input',
        Output: 'Output',
        Total: 'Total',
        Image: 'Image',
      },
    },
  },
})

const reactTestGlobals = globalThis as typeof globalThis & {
  IS_REACT_ACT_ENVIRONMENT?: boolean
}
reactTestGlobals.IS_REACT_ACT_ENVIRONMENT = true

function TaskUsageCell(props: { log: TaskLog }) {
  const columns = useTaskLogsColumns(false, false)
  const usageColumn = columns.find(
    (column) => 'accessorKey' in column && column.accessorKey === 'usage'
  )

  assert.ok(usageColumn)
  assert.equal(typeof usageColumn.cell, 'function')
  const Cell = usageColumn.cell as React.ComponentType<
    CellContext<TaskLog, unknown>
  >
  const row = {
    original: props.log,
    getValue: (key: string) => props.log[key as keyof TaskLog],
  } as Row<TaskLog>

  return <Cell {...({ row } as CellContext<TaskLog, unknown>)} />
}

describe('task usage column', () => {
  afterEach(() => {
    document.body.replaceChildren()
  })

  afterAll(() => {
    domWindow.close()
  })

  test('shows input and output seconds separately for a video task', async () => {
    const container = document.createElement('div')
    document.body.append(container)
    const root = createRoot(container)
    const log = {
      id: 1,
      user_id: 1,
      platform: 'ali',
      task_id: 'task_usage',
      action: 'textGenerate',
      channel_id: 1,
      submit_time: 1,
      status: 'SUCCESS',
      usage: {
        kind: 'video_duration',
        unit: 'second',
        input: 2.5,
        output: 5,
        total: 7.5,
      },
    } as TaskLog

    await act(async () => {
      root.render(
        <I18nextProvider i18n={i18n}>
          <TaskUsageCell log={log} />
        </I18nextProvider>
      )
    })

    assert.match(container.textContent || '', /Input\s*2\.5s/)
    assert.match(container.textContent || '', /Output\s*5s/)
    assert.match(container.textContent || '', /Total\s*7\.5s/)

    await act(async () => root.unmount())
  })

  test('falls back to persisted provider usage for an existing video task', async () => {
    const container = document.createElement('div')
    document.body.append(container)
    const root = createRoot(container)
    const log = {
      id: 2,
      user_id: 1,
      platform: 'ali',
      task_id: 'task_existing_usage',
      action: 'textGenerate',
      channel_id: 1,
      submit_time: 1,
      status: 'SUCCESS',
      data: {
        usage: {
          duration: 5,
          input_video_duration: 0,
          output_video_duration: 5,
        },
      },
    } as TaskLog

    await act(async () => {
      root.render(
        <I18nextProvider i18n={i18n}>
          <TaskUsageCell log={log} />
        </I18nextProvider>
      )
    })

    assert.match(container.textContent || '', /Input\s*0s/)
    assert.match(container.textContent || '', /Output\s*5s/)
    assert.match(container.textContent || '', /Total\s*5s/)

    await act(async () => root.unmount())
  })

  test('falls back to nested MiniMax H3 usage including input images', async () => {
	const container = document.createElement('div')
	document.body.append(container)
	const root = createRoot(container)
	const log = {
	  id: 3,
	  user_id: 1,
	  platform: 'hailuo-video',
	  task_id: 'task_h3_existing_usage',
	  action: 'textGenerate',
	  channel_id: 2,
	  submit_time: 1,
	  status: 'SUCCESS',
	  data: {
		task: {
		  usage: {
			input_seconds: 2,
			output_seconds: 4,
			total_seconds: 6,
			input_image_count: 7,
		  },
		},
	  },
	} as TaskLog

	await act(async () => {
	  root.render(
		<I18nextProvider i18n={i18n}>
		  <TaskUsageCell log={log} />
		</I18nextProvider>
	  )
	})

	assert.match(container.textContent || '', /Input\s*2s/)
	assert.match(container.textContent || '', /Output\s*4s/)
	assert.match(container.textContent || '', /Image\s*7/)

	await act(async () => root.unmount())
  })
})
