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
import type { UseFormReturn } from 'react-hook-form'

import {
  DYNAMIC_ROUTING_FLAT_DEFAULTS,
  type DynamicRoutingSettings,
} from '../../types'
import type { DynamicRoutingFormValues } from '../lib/dynamic-routing-schema'

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
const { QueryClient, QueryClientProvider } =
  await import('@tanstack/react-query')
const { api } = await import('@/lib/api')
const { useSettingsForm } = await import('../../hooks/use-settings-form')
const { useUpdateDynamicRoutingSettings } =
  await import('../../hooks/use-update-option')
const { buildDynamicRoutingFormValues } =
  await import('../lib/dynamic-routing-schema')

const reactTestGlobals = globalThis as typeof globalThis & {
  IS_REACT_ACT_ENVIRONMENT?: boolean
}
reactTestGlobals.IS_REACT_ACT_ENVIRONMENT = true

type ApiPut = (
  url: string,
  data?: unknown,
  config?: { skipBusinessError?: boolean; skipErrorHandler?: boolean }
) => Promise<{ data: { success: boolean; message: string } }>

type ExposedForm = {
  form: UseFormReturn<DynamicRoutingFormValues>
  handleSubmit: () => Promise<void>
  handleReset: () => void
  isDirty: boolean
}

type RenderedHarness = {
  host: HTMLDivElement
  queryClient: InstanceType<typeof QueryClient>
  root: ReturnType<typeof createRoot>
}

const apiClient = api as unknown as { put: ApiPut }
const originalPut = apiClient.put
let exposedForm: ExposedForm | null = null
let renderedHarness: RenderedHarness | null = null

function DynamicRoutingFormHarness(props: {
  defaultValues: DynamicRoutingFormValues
}) {
  const updateDynamicRouting = useUpdateDynamicRoutingSettings()
  const settingsForm = useSettingsForm<DynamicRoutingFormValues>({
    defaultValues: props.defaultValues,
    preserveDirtyValuesOnDefaultChange: true,
    onSubmit: async (values) => {
      await updateDynamicRouting.mutateAsync(values.dynamic_routing_setting)
    },
  })
  exposedForm = settingsForm

  return (
    <input
      aria-label='Maximum samples'
      type='number'
      {...settingsForm.form.register('dynamic_routing_setting.max_samples', {
        valueAsNumber: true,
      })}
    />
  )
}

async function renderHarness(defaultValues: DynamicRoutingFormValues) {
  const host = document.createElement('div')
  document.body.append(host)
  const root = createRoot(host)
  const queryClient = new QueryClient({
    defaultOptions: {
      mutations: { retry: false },
      queries: { retry: false },
    },
  })
  renderedHarness = { host, queryClient, root }

  await act(async () => {
    root.render(
      <QueryClientProvider client={queryClient}>
        <DynamicRoutingFormHarness defaultValues={defaultValues} />
      </QueryClientProvider>
    )
  })
}

async function rerenderHarness(defaultValues: DynamicRoutingFormValues) {
  assert.ok(renderedHarness)
  await act(async () => {
    renderedHarness?.root.render(
      <QueryClientProvider client={renderedHarness.queryClient}>
        <DynamicRoutingFormHarness defaultValues={defaultValues} />
      </QueryClientProvider>
    )
  })
}

async function submitExposedForm() {
  assert.ok(exposedForm)
  let submitError: unknown
  await act(async () => {
    try {
      await exposedForm?.handleSubmit()
    } catch (error) {
      submitError = error
    }
  })
  return submitError
}

afterEach(async () => {
  apiClient.put = originalPut
  exposedForm = null
  if (renderedHarness) {
    await act(async () => renderedHarness?.root.unmount())
    renderedHarness.queryClient.clear()
    renderedHarness.host.remove()
    renderedHarness = null
  }
  document.body.replaceChildren()
})

after(() => {
  domWindow.close()
})

describe('dynamic routing atomic mutation', () => {
  test('submits the complete configuration once and invalidates options once', async () => {
    const requests: Array<{
      url: string
      body: DynamicRoutingSettings
      config: Parameters<ApiPut>[2]
    }> = []
    apiClient.put = async (url, data, config) => {
      requests.push({ url, body: data as DynamicRoutingSettings, config })
      return { data: { success: true, message: '' } }
    }

    const defaults = buildDynamicRoutingFormValues(
      DYNAMIC_ROUTING_FLAT_DEFAULTS
    )
    await renderHarness(defaults)
    assert.ok(exposedForm)

    let invalidationCount = 0
    const queryClient = renderedHarness?.queryClient
    assert.ok(queryClient)
    const invalidateQueries = queryClient.invalidateQueries.bind(queryClient)
    queryClient.invalidateQueries = ((...args) => {
      invalidationCount++
      return invalidateQueries(...args)
    }) as typeof queryClient.invalidateQueries

    await act(async () => {
      exposedForm?.form.setValue('dynamic_routing_setting.max_samples', 61, {
        shouldDirty: true,
      })
    })
    const submitError = await submitExposedForm()

    assert.equal(submitError, undefined)
    assert.equal(requests.length, 1)
    assert.equal(requests[0]?.url, '/api/option/dynamic_routing')
    assert.deepEqual(requests[0]?.config, {
      skipBusinessError: true,
      skipErrorHandler: true,
    })
    assert.deepEqual(requests[0]?.body, {
      ...defaults.dynamic_routing_setting,
      max_samples: 61,
    })
    assert.equal(invalidationCount, 1)
    assert.equal(exposedForm?.isDirty, false)
  })

  test('keeps dirty edits through refetch and a rejected API response', async () => {
    const requests: Array<{
      url: string
      body: DynamicRoutingSettings
      config: Parameters<ApiPut>[2]
    }> = []
    apiClient.put = async (url, data, config) => {
      requests.push({ url, body: data as DynamicRoutingSettings, config })
      return { data: { success: false, message: 'Backend rejected settings' } }
    }

    const defaults = buildDynamicRoutingFormValues(
      DYNAMIC_ROUTING_FLAT_DEFAULTS
    )
    await renderHarness(defaults)
    assert.ok(exposedForm)

    await act(async () => {
      exposedForm?.form.setValue('dynamic_routing_setting.max_samples', 61, {
        shouldDirty: true,
      })
    })

    const refreshed = {
      dynamic_routing_setting: {
        ...defaults.dynamic_routing_setting,
        max_age_seconds: 120,
      },
    }
    await rerenderHarness(refreshed)

    assert.equal(
      exposedForm?.form.getValues('dynamic_routing_setting.max_samples'),
      61
    )
    assert.equal(
      exposedForm?.form.getValues('dynamic_routing_setting.max_age_seconds'),
      120
    )

    const submitError = await submitExposedForm()

    assert.match(String(submitError), /Backend rejected settings/)
    assert.equal(requests.length, 1)
    assert.equal(requests[0]?.url, '/api/option/dynamic_routing')
    assert.deepEqual(requests[0]?.body, {
      ...refreshed.dynamic_routing_setting,
      max_samples: 61,
    })
    assert.equal(
      exposedForm?.form.getValues('dynamic_routing_setting.max_samples'),
      61
    )
    assert.equal(exposedForm?.isDirty, true)
  })
})
