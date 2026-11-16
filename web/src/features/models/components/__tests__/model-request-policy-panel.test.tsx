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
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import {
  fireEvent,
  render,
  screen,
  waitFor,
  within,
} from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import { api } from '@/lib/api'

import { ModelRequestPolicyPanel } from '../model-request-policy-panel'

const originalGetAnimations = Object.getOwnPropertyDescriptor(
  Element.prototype,
  'getAnimations'
)
beforeEach(() => {
  Object.defineProperty(Element.prototype, 'getAnimations', {
    configurable: true,
    value: () => [],
  })
})
afterEach(() => {
  if (originalGetAnimations) {
    Object.defineProperty(
      Element.prototype,
      'getAnimations',
      originalGetAnimations
    )
  } else {
    Reflect.deleteProperty(Element.prototype, 'getAnimations')
  }
})

function renderPanel() {
  const dirty = vi.fn()
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  })
  render(
    <QueryClientProvider client={client}>
      <ModelRequestPolicyPanel modelName='public/model' onDirtyChange={dirty} />
    </QueryClientProvider>
  )
  return { dirty, client }
}

describe('model request parameters', () => {
  it('saves capability editor changes and preserves explicit zero overrides', async () => {
    const user = userEvent.setup()
    vi.spyOn(api, 'get').mockResolvedValue({
      data: {
        success: true,
        data: { model: 'public/model', param_override: { temperature: 0 } },
      },
    })
    const put = vi
      .spyOn(api, 'put')
      .mockResolvedValue({ data: { success: true } })
    const { dirty, client } = renderPanel()
    expect(await screen.findByRole('button', { name: 'Save' })).toBeDisabled()
    expect(
      screen.getByText(
        /Duplicate override rules across model and channel settings are merged/
      )
    ).toBeVisible()
    await user.click(
      screen.getByRole('button', { name: 'Configure capabilities' })
    )
    const dialog = await screen.findByRole('dialog', {
      name: 'Model Parameter Capabilities',
    })
    expect(
      within(dialog).getByRole('button', { name: 'Model default' })
    ).toBeVisible()
    await user.click(within(dialog).getByRole('tab', { name: 'JSON Editor' }))
    fireEvent.input(
      within(dialog).getByRole('textbox', {
        name: 'Model Parameter Capabilities JSON',
      }),
      {
        target: {
          value:
            '{"defaults":{"temperature":{"max":1,"on_violation":"clamp"}}}',
        },
      }
    )
    await user.click(within(dialog).getByRole('button', { name: 'Save' }))
    await waitFor(() =>
      expect(screen.queryByRole('dialog')).not.toBeInTheDocument()
    )
    expect(dirty).toHaveBeenLastCalledWith(true)
    expect(put).not.toHaveBeenCalled()
    await user.click(
      screen.getByRole('button', { name: 'Configure parameter overrides' })
    )
    const overridesDialog = await screen.findByRole('dialog')
    fireEvent.input(
      within(overridesDialog).getByRole('textbox', {
        name: 'Legacy Format (JSON Object)',
      }),
      { target: { value: '{"temperature":0,"store":false}' } }
    )
    await user.click(
      within(overridesDialog).getByRole('button', { name: 'Save' })
    )
    await waitFor(() =>
      expect(screen.queryByRole('dialog')).not.toBeInTheDocument()
    )
    await user.click(screen.getByRole('button', { name: 'Save' }))
    await waitFor(() =>
      expect(put).toHaveBeenCalledWith('/api/models/request-policy', {
        model: 'public/model',
        parameter_capabilities: {
          defaults: { temperature: { max: 1, on_violation: 'clamp' } },
          rules: [],
        },
        param_override: { temperature: 0, store: false },
      })
    )
    await waitFor(() =>
      expect(screen.getByRole('button', { name: 'Save' })).toBeDisabled()
    )
    expect(dirty).toHaveBeenLastCalledWith(false)
    client.clear()
  })

  it('keeps cleared defaults dirty after a failed save and permits retry', async () => {
    const user = userEvent.setup()
    vi.spyOn(api, 'get').mockResolvedValue({
      data: {
        success: true,
        data: { model: 'public/model', param_override: { temperature: 0.5 } },
      },
    })
    const put = vi
      .spyOn(api, 'put')
      .mockResolvedValueOnce({
        data: { success: false, message: 'Policy storage unavailable' },
      })
      .mockResolvedValueOnce({ data: { success: true } })
    const { dirty, client } = renderPanel()
    await user.click(await screen.findByRole('button', { name: 'Clear' }))
    await user.click(screen.getByRole('button', { name: 'Save' }))
    expect(await screen.findByText('Policy storage unavailable')).toBeVisible()
    expect(screen.getByRole('button', { name: 'Save' })).toBeEnabled()
    expect(dirty).toHaveBeenLastCalledWith(true)
    await user.click(screen.getByRole('button', { name: 'Save' }))
    await waitFor(() =>
      expect(screen.getByRole('button', { name: 'Save' })).toBeDisabled()
    )
    expect(put).toHaveBeenLastCalledWith('/api/models/request-policy', {
      model: 'public/model',
      parameter_capabilities: {},
      param_override: {},
    })
    client.clear()
  })

  it('shows a load failure without editable controls and loads defaults after retry', async () => {
    const user = userEvent.setup()
    vi.spyOn(api, 'get')
      .mockResolvedValueOnce({
        data: { success: false, message: 'Cannot load policy' },
      })
      .mockResolvedValueOnce({
        data: { success: true, data: { model: 'public/model' } },
      })
    const { client } = renderPanel()
    expect(await screen.findByText('Cannot load policy')).toBeVisible()
    expect(
      screen.queryByRole('button', { name: 'Save' })
    ).not.toBeInTheDocument()
    await user.click(screen.getByRole('button', { name: 'Retry' }))
    expect(await screen.findByRole('button', { name: 'Save' })).toBeDisabled()
    client.clear()
  })
})
