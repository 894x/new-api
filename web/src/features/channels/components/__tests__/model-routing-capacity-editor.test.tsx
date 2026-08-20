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
import { after, describe, test } from 'node:test'

import { Window } from 'happy-dom'

import type { ModelRoutingOverride } from '../../types'

const domWindow = new Window()
const domGlobals = [
  'window',
  'document',
  'navigator',
  'HTMLElement',
  'HTMLInputElement',
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
const { QueryClient, QueryClientProvider } =
  await import('@tanstack/react-query')
const { ROLE } = await import('@/lib/roles')
const { useAuthStore } = await import('@/stores/auth-store')
const { channelsQueryKeys } = await import('../../lib/channel-actions')
const { ModelRoutingOverridesEditor } =
  await import('../model-routing-overrides-editor')

const reactTestGlobals = globalThis as typeof globalThis & {
  IS_REACT_ACT_ENVIRONMENT?: boolean
}
reactTestGlobals.IS_REACT_ACT_ENVIRONMENT = true

const i18n = createInstance()
await i18n.use(initReactI18next).init({
  lng: 'en',
  resources: { en: { translation: {} } },
})

const row: ModelRoutingOverride = {
  channel_id: 7,
  channel_name: 'Primary channel',
  channel_type: 1,
  channel_status: 1,
  model: 'gpt-test',
  default_priority: 100,
  default_weight: 20,
  default_rpm: 60,
  default_tpm: 6000,
  priority_override: null,
  weight_override: 3,
  rpm_override: null,
  tpm_override: 1000,
  effective_priority: 100,
  effective_weight: 3,
  effective_rpm: 60,
  effective_tpm: 1000,
}

describe('model routing capacity editor', () => {
  after(() => {
    domWindow.close()
  })

  test('aligns accessible Priority, Weight, RPM, and TPM inputs with their columns', async () => {
    useAuthStore.getState().auth.setUser({
      id: 1,
      username: 'root',
      role: ROLE.SUPER_ADMIN,
    })
    const queryClient = new QueryClient({
      defaultOptions: { queries: { retry: false } },
    })
    queryClient.setQueryData(
      channelsQueryKeys.modelRoutingOverridesByChannel(7),
      { success: true, data: [row] }
    )
    const container = document.createElement('div')
    document.body.append(container)
    const root = createRoot(container)

    await act(async () => {
      root.render(
        <I18nextProvider i18n={i18n}>
          <QueryClientProvider client={queryClient}>
            <ModelRoutingOverridesEditor channelId={7} />
          </QueryClientProvider>
        </I18nextProvider>
      )
    })

    const headers = [...container.querySelectorAll('th')].map((cell) =>
      cell.textContent?.trim()
    )
    assert.deepEqual(headers.slice(0, 5), [
      'Model',
      'Priority',
      'Weight',
      'RPM',
      'TPM',
    ])
    const inputs = [...container.querySelectorAll('tbody input')].map((input) =>
      input.getAttribute('aria-label')
    )
    assert.deepEqual(inputs, [
      'Priority · gpt-test',
      'Weight · gpt-test',
      'RPM · gpt-test',
      'TPM · gpt-test',
    ])

    await act(async () => root.unmount())
    queryClient.clear()
    container.remove()
    useAuthStore.getState().auth.setUser(null)
  })
})
