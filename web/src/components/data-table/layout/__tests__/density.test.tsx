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

import {
  getCoreRowModel,
  useReactTable,
  type ColumnDef,
} from '@tanstack/react-table'
import { Window } from 'happy-dom'
import { afterAll, beforeEach, describe, test } from 'vitest'

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
const { createInstance } = await import('i18next')
const { I18nextProvider, initReactI18next } = await import('react-i18next')
const { DataTablePage } = await import('../data-table-page')

const i18n = createInstance()
await i18n.use(initReactI18next).init({
  lng: 'en',
  resources: {
    en: {
      translation: {
        Compact: 'Compact',
        Comfortable: 'Comfortable',
        Filter: 'Filter',
      },
    },
  },
})

const reactTestGlobals = globalThis as typeof globalThis & {
  IS_REACT_ACT_ENVIRONMENT?: boolean
}
reactTestGlobals.IS_REACT_ACT_ENVIRONMENT = true

type FixtureRow = { name: string }

const columns: ColumnDef<FixtureRow>[] = [
  { accessorKey: 'name', header: 'Name' },
]

function DensityFixture() {
  const table = useReactTable({
    data: [{ name: 'Kimi' }],
    columns,
    getCoreRowModel: getCoreRowModel(),
  })

  return (
    <DataTablePage
      table={table}
      columns={columns}
      enableCompactMode
      compactModeStorageKey='models'
      showPagination={false}
      fixedHeight={false}
      toolbarProps={{ hideViewOptions: true }}
    />
  )
}

describe('DataTablePage density', () => {
  beforeEach(() => {
    domWindow.localStorage.clear()
    document.body.replaceChildren()
  })

  afterAll(() => {
    domWindow.close()
  })

  test('starts compact and lets the user switch to comfortable rows', async () => {
    const container = document.createElement('div')
    document.body.append(container)
    const root = createRoot(container)

    await act(async () => {
      root.render(
        <I18nextProvider i18n={i18n}>
          <DensityFixture />
        </I18nextProvider>
      )
    })

    const table = container.querySelector('table')
    assert.ok(table)
    assert.ok(table.className.split(' ').includes('[&_tbody>tr]:h-10'))

    const densityButton = container.querySelector<HTMLButtonElement>(
      'button[aria-label="Comfortable"]'
    )
    assert.ok(densityButton)

    await act(async () => densityButton.click())

    assert.equal(
      table.className.split(' ').includes('[&_tbody>tr]:h-10'),
      false
    )
    assert.deepEqual(
      JSON.parse(domWindow.localStorage.getItem('table_compact_modes') ?? '{}'),
      { models: false }
    )

    await act(async () => root.unmount())
    container.remove()
  })
})
