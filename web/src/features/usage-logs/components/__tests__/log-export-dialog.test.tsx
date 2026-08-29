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

import { Window } from 'happy-dom'
import type React from 'react'
import { afterAll, afterEach, describe, test } from 'vitest'

const domWindow = new Window()
const domGlobals = [
  'window',
  'document',
  'navigator',
  'HTMLElement',
  'HTMLAnchorElement',
  'SVGElement',
  'Node',
  'Element',
  'Event',
  'MouseEvent',
  'PointerEvent',
  'URL',
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
const { createInstance } = await import('i18next')
const { I18nextProvider, initReactI18next } = await import('react-i18next')
const { LogExportDialog } = await import('../log-export-dialog')

const i18n = createInstance()
await i18n.use(initReactI18next).init({ lng: 'en', resources: { en: {} } })

const reactTestGlobals = globalThis as typeof globalThis & {
  IS_REACT_ACT_ENVIRONMENT?: boolean
}
reactTestGlobals.IS_REACT_ACT_ENVIRONMENT = true

const originalCreateObjectURL = Object.getOwnPropertyDescriptor(
  domWindow.URL,
  'createObjectURL'
)
const originalRevokeObjectURL = Object.getOwnPropertyDescriptor(
  domWindow.URL,
  'revokeObjectURL'
)
const originalAnchorClick = domWindow.HTMLAnchorElement.prototype.click

function restoreURLMethod(
  name: 'createObjectURL' | 'revokeObjectURL',
  descriptor: PropertyDescriptor | undefined
) {
  if (descriptor) {
    Object.defineProperty(domWindow.URL, name, descriptor)
  } else {
    Reflect.deleteProperty(domWindow.URL, name)
  }
}

function findButton(root: ParentNode, label: string): HTMLButtonElement {
  const button = [...root.querySelectorAll('button')].find(
    (candidate) => candidate.textContent?.trim() === label
  )
  assert.ok(button instanceof domWindow.HTMLButtonElement)
  return button as HTMLButtonElement
}

describe('usage log export dialog', () => {
  afterAll(() => domWindow.close())
  afterEach(() => {
    restoreURLMethod('createObjectURL', originalCreateObjectURL)
    restoreURLMethod('revokeObjectURL', originalRevokeObjectURL)
    domWindow.HTMLAnchorElement.prototype.click = originalAnchorClick
  })

  test('associates the scope label and downloads the configured archive', async () => {
    const requests: unknown[] = []
    const exporter: React.ComponentProps<
      typeof LogExportDialog
    >['exporter'] = async (request) => {
      requests.push(request)
      return { blob: new Blob(['zip']), filename: 'usage-upstream.zip' }
    }
    let downloadedFilename = ''
    let revokedURL = ''
    Object.defineProperty(domWindow.URL, 'createObjectURL', {
      configurable: true,
      value: () => 'blob:usage-export',
    })
    Object.defineProperty(domWindow.URL, 'revokeObjectURL', {
      configurable: true,
      value: (url: string) => {
        revokedURL = url
      },
    })
    domWindow.HTMLAnchorElement.prototype.click = function click() {
      downloadedFilename = this.download
    }

    const container = document.createElement('div')
    document.body.append(container)
    const root = createRoot(container)
    await act(async () => {
      root.render(
        <I18nextProvider i18n={i18n}>
          <LogExportDialog
            params={{ request_id: 'client-request' }}
            exporter={exporter}
          />
        </I18nextProvider>
      )
    })

    await act(async () => findButton(container, 'Export').click())
    const dialog = document.querySelector('[role="dialog"]')
    assert.ok(dialog)
    const scopeTrigger = dialog.querySelector(
      '[aria-labelledby="log-export-view-label"]'
    )
    assert.ok(scopeTrigger)

    await act(async () => findButton(dialog, 'Export').click())
    assert.equal(requests.length, 1)
    assert.equal(downloadedFilename, 'usage-upstream.zip')
    assert.equal(revokedURL, 'blob:usage-export')
    assert.equal(document.querySelector('[role="dialog"]'), null)

    await act(async () => root.unmount())
    container.remove()
  })

  test('keeps the dialog open while exporting and after a failed export', async () => {
    let rejectExport: ((reason: Error) => void) | undefined
    const exportResult = new Promise<never>((_resolve, reject) => {
      rejectExport = reject
    })
    const container = document.createElement('div')
    document.body.append(container)
    const root = createRoot(container)
    await act(async () => {
      root.render(
        <I18nextProvider i18n={i18n}>
          <LogExportDialog params={{}} exporter={() => exportResult} />
        </I18nextProvider>
      )
    })

    await act(async () => findButton(container, 'Export').click())
    const dialog = document.querySelector('[role="dialog"]')
    assert.ok(dialog)
    await act(async () => {
      findButton(dialog, 'Export').click()
      await Promise.resolve()
    })
    const exportingButton = findButton(dialog, 'Exporting...')
    assert.equal(exportingButton.disabled, true)

    await act(async () => {
      rejectExport?.(new Error('export failed'))
      await exportResult.catch(() => undefined)
    })
    assert.ok(document.querySelector('[role="dialog"]'))
    assert.equal(findButton(dialog, 'Export').disabled, false)

    await act(async () => root.unmount())
    container.remove()
  })
})
