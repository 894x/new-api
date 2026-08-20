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
import { after, beforeEach, describe, test } from 'node:test'

import { Window } from 'happy-dom'

const domWindow = new Window()
const domGlobals = [
  'window',
  'document',
  'navigator',
  'HTMLElement',
  'Event',
  'StorageEvent',
  'localStorage',
] as const

for (const key of domGlobals) {
  Object.defineProperty(globalThis, key, {
    configurable: true,
    value: domWindow[key],
  })
}

const { act } = await import('react')
const { createRoot } = await import('react-dom/client')
const { useTableCompactMode } = await import('../use-table-compact-mode')

const reactTestGlobals = globalThis as typeof globalThis & {
  IS_REACT_ACT_ENVIRONMENT?: boolean
}
reactTestGlobals.IS_REACT_ACT_ENVIRONMENT = true

function CompactModeProbe(props: { tableKey?: string }) {
  const [compact, setCompact] = useTableCompactMode(
    props.tableKey ?? 'channels'
  )

  return (
    <button type='button' onClick={() => setCompact(!compact)}>
      {compact ? 'compact' : 'comfortable'}
    </button>
  )
}

async function renderProbe(tableKey?: string) {
  const container = document.createElement('div')
  document.body.append(container)
  const root = createRoot(container)

  await act(async () => root.render(<CompactModeProbe tableKey={tableKey} />))

  return { container, root }
}

describe('useTableCompactMode', () => {
  beforeEach(() => {
    domWindow.localStorage.clear()
    document.body.replaceChildren()
  })

  after(() => {
    domWindow.close()
  })

  test('uses compact mode when no preference has been saved', async () => {
    const rendered = await renderProbe()

    assert.equal(rendered.container.textContent, 'compact')

    await act(async () => rendered.root.unmount())
  })

  test('switches to comfortable mode and persists the explicit preference', async () => {
    const rendered = await renderProbe()
    const button = rendered.container.querySelector('button')
    assert.ok(button)

    await act(async () => button.click())

    assert.equal(rendered.container.textContent, 'comfortable')
    assert.deepEqual(
      JSON.parse(domWindow.localStorage.getItem('table_compact_modes') ?? '{}'),
      { channels: false }
    )

    await act(async () => rendered.root.unmount())
  })

  test('loads the saved preference when the table key changes', async () => {
    domWindow.localStorage.setItem(
      'table_compact_modes',
      JSON.stringify({ common: false, task: true })
    )
    const rendered = await renderProbe('common')
    assert.equal(rendered.container.textContent, 'comfortable')

    await act(async () => {
      rendered.root.render(<CompactModeProbe tableKey='task' />)
    })

    assert.equal(rendered.container.textContent, 'compact')

    await act(async () => rendered.root.unmount())
  })
})
