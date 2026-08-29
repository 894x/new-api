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
import { after, afterEach, describe, test } from 'node:test'

import { Window } from 'happy-dom'

const domWindow = new Window()
const domGlobals = [
  'window',
  'document',
  'navigator',
  'HTMLElement',
  'HTMLButtonElement',
  'HTMLInputElement',
  'HTMLTextAreaElement',
  'Node',
  'Element',
  'Event',
  'CustomEvent',
  'MutationObserver',
  'ResizeObserver',
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
Object.defineProperty(domWindow.Element.prototype, 'getAnimations', {
  configurable: true,
  value: () => [],
})

const { act } = await import('react')
const { createRoot } = await import('react-dom/client')
const i18next = (await import('i18next')).default
const { initReactI18next } = await import('react-i18next')
await i18next.use(initReactI18next).init({
  lng: 'en',
  resources: {
    en: {
      translation: {
        'JSON Editor': 'JSON Editor',
        'Model Parameter Capabilities JSON':
          'Model Parameter Capabilities JSON',
        Save: 'Save',
      },
    },
  },
})
const { ParameterCapabilityEditorDialog } =
  await import('../parameter-capability-editor-dialog')
const reactTestGlobals = globalThis as typeof globalThis & {
  IS_REACT_ACT_ENVIRONMENT?: boolean
}
reactTestGlobals.IS_REACT_ACT_ENVIRONMENT = true

type RenderedDialog = {
  container: HTMLDivElement
  root: ReturnType<typeof createRoot>
}

async function renderDialog(
  onSave: (value: string) => void
): Promise<RenderedDialog> {
  const container = document.createElement('div')
  document.body.append(container)
  const root = createRoot(container)

  await act(async () => {
    root.render(
      <ParameterCapabilityEditorDialog
        open
        value=''
        models={['kimi-k3']}
        paramOverrideConfigured={false}
        onOpenChange={() => undefined}
        onSave={onSave}
      />
    )
  })

  return { container, root }
}

function findButton(label: string): HTMLButtonElement | undefined {
  return [...document.querySelectorAll('button')].find(
    (button) => button.textContent?.trim() === label
  )
}

async function enterJsonDraft(value: string): Promise<HTMLTextAreaElement> {
  const jsonTab = findButton('JSON Editor')
  assert.ok(jsonTab)
  await act(async () => jsonTab.click())

  const textarea = document.querySelector<HTMLTextAreaElement>(
    'textarea[aria-label="Model Parameter Capabilities JSON"]'
  )
  assert.ok(textarea)
  await act(async () => {
    textarea.value = value
    textarea.dispatchEvent(new Event('input', { bubbles: true }))
  })
  return textarea
}

describe('ParameterCapabilityEditorDialog JSON editing', () => {
  let rendered: RenderedDialog | undefined

  afterEach(async () => {
    if (rendered) {
      await act(async () => rendered?.root.unmount())
      rendered.container.remove()
      rendered = undefined
    }
  })

  after(() => {
    domWindow.close()
  })

  test('saves a valid JSON draft through the JSON editor tab', async () => {
    const saved: string[] = []
    rendered = await renderDialog((value) => saved.push(value))
    await enterJsonDraft(
      '{"rules":[{"selector":{"type":"exact","value":"kimi-k3"},"parameters":{"temperature":{"supported":false,"on_violation":"reject"}}}]}'
    )

    const saveButton = findButton('Save')
    assert.ok(saveButton)
    await act(async () => saveButton.click())

    assert.equal(saved.length, 1)
    assert.deepEqual(JSON.parse(saved[0]), {
      defaults: {},
      rules: [
        {
          selector: { type: 'exact', value: 'kimi-k3' },
          parameters: {
            temperature: {
              supported: false,
              on_violation: 'reject',
            },
          },
        },
      ],
    })
  })

  test('blocks invalid JSON without discarding the draft', async () => {
    const saved: string[] = []
    rendered = await renderDialog((value) => saved.push(value))
    const textarea = await enterJsonDraft('{"defaults":')

    const saveButton = findButton('Save')
    assert.ok(saveButton)
    await act(async () => saveButton.click())

    assert.deepEqual(saved, [])
    assert.equal(textarea.value, '{"defaults":')
  })
})
