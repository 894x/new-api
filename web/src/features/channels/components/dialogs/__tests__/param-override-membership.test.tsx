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
import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'

import { fireEvent, render, screen } from '@testing-library/react'
import { expect, test } from 'vitest'

import { ParamOverrideEditorDialog } from '../param-override-editor-dialog'

test('saving the Kimi conversion in the visual editor preserves dynamic conditions', async () => {
  const fixture = readFileSync(
    resolve('../docs/examples/kimi-k3-allowed-tools.json'),
    'utf8'
  )
  const saved: string[] = []
  render(
    <ParamOverrideEditorDialog
      open
      value={fixture}
      onOpenChange={() => undefined}
      onSave={(value) => saved.push(value)}
    />
  )
  fireEvent.click(screen.getByRole('button', { name: 'Save' }))
  expect(saved).toHaveLength(1)
  // The visual editor may add explicit AND/OR defaults; compare each contract.
  const expected = JSON.parse(fixture)
  const actual = JSON.parse(saved[0])
  expect(actual.operations).toEqual(expected.operations)
})

test('editing a prune reference preserves membership and the new request path', () => {
  const saved: string[] = []
  const value = JSON.stringify({
    operations: [
      {
        mode: 'prune_objects',
        path: 'tools',
        value: {
          recursive: false,
          conditions: [
            {
              path: 'function.name',
              mode: 'in',
              value_path: 'allowed',
              invert: true,
            },
          ],
        },
      },
    ],
  })
  render(
    <ParamOverrideEditorDialog
      open
      value={value}
      onOpenChange={() => undefined}
      onSave={(next) => saved.push(next)}
    />
  )
  fireEvent.change(
    screen.getByRole('textbox', { name: 'Match Value Path (optional)' }),
    { target: { value: 'next_allowed' } }
  )
  expect(
    screen.getByRole('textbox', { name: 'Match Value (optional)' })
  ).toBeDisabled()
  fireEvent.click(screen.getByRole('button', { name: 'Save' }))
  expect(saved).toHaveLength(1)
  expect(JSON.parse(saved[0]).operations[0].value).toEqual({
    recursive: false,
    conditions: [
      {
        path: 'function.name',
        mode: 'in',
        value_path: 'next_allowed',
        invert: true,
      },
    ],
  })
})
