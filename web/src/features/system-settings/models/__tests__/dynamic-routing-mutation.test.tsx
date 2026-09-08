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
import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, describe, expect, test, vi } from 'vitest'

import { api } from '@/lib/api'

import { SettingsPageProvider } from '../../components/settings-page-context'
import {
  DYNAMIC_ROUTING_FLAT_DEFAULTS,
  type DynamicRoutingFlatSettings,
} from '../../types'
import { DynamicRoutingSection } from '../dynamic-routing-section'
import { buildDynamicRoutingFormValues } from '../lib/dynamic-routing-schema'

vi.mock('@tanstack/react-router', async (importOriginal) => ({
  ...(await importOriginal<typeof import('@tanstack/react-router')>()),
  useBlocker: () => ({ status: 'idle' }),
}))

let actions: HTMLDivElement
let client: QueryClient

afterEach(() => {
  actions?.remove()
  client?.clear()
})

function renderSettings(defaults = DYNAMIC_ROUTING_FLAT_DEFAULTS) {
  actions = document.createElement('div')
  document.body.append(actions)
  client = new QueryClient({
    defaultOptions: { mutations: { retry: false }, queries: { retry: false } },
  })
  const view = (values: DynamicRoutingFlatSettings) => (
    <QueryClientProvider client={client}>
      <SettingsPageProvider actionsContainer={actions}>
        <DynamicRoutingSection defaultValues={values} />
      </SettingsPageProvider>
    </QueryClientProvider>
  )
  const result = render(view(defaults))
  return (values: DynamicRoutingFlatSettings) => result.rerender(view(values))
}

describe('dynamic routing settings', () => {
  test('saving an edited field submits the full configuration and clears dirty state', async () => {
    const request = vi
      .spyOn(api, 'put')
      .mockResolvedValue({ data: { success: true, message: '' } })
    const user = userEvent.setup()
    renderSettings()
    const input = screen.getByRole('spinbutton', { name: 'Maximum samples' })
    fireEvent.change(input, { target: { value: '61' } })
    expect(screen.getByRole('button', { name: 'Reset' })).toBeEnabled()
    await user.click(screen.getByRole('button', { name: 'Save Changes' }))
    await waitFor(() =>
      expect(screen.getByRole('button', { name: 'Reset' })).toBeDisabled()
    )
    expect(input).toHaveValue(61)
    expect(request).toHaveBeenCalledExactlyOnceWith(
      '/api/option/dynamic_routing',
      {
        ...buildDynamicRoutingFormValues(DYNAMIC_ROUTING_FLAT_DEFAULTS)
          .dynamic_routing_setting,
        max_samples: 61,
      },
      { skipBusinessError: true, skipErrorHandler: true }
    )
  })

  test('refetch and rejected save preserve edited input and allow a successful retry', async () => {
    const request = vi
      .spyOn(api, 'put')
      .mockResolvedValueOnce({
        data: { success: false, message: 'Backend rejected settings' },
      })
      .mockResolvedValueOnce({ data: { success: true, message: '' } })
    const user = userEvent.setup()
    const rerender = renderSettings()
    const input = screen.getByRole('spinbutton', { name: 'Maximum samples' })
    fireEvent.change(input, { target: { value: '61' } })
    rerender({
      ...DYNAMIC_ROUTING_FLAT_DEFAULTS,
      'dynamic_routing_setting.max_age_seconds': 120,
    })
    expect(input).toHaveValue(61)
    await user.click(screen.getByRole('button', { name: 'Save Changes' }))
    await waitFor(() => expect(request).toHaveBeenCalledTimes(1))
    expect(input).toHaveValue(61)
    expect(screen.getByRole('button', { name: 'Reset' })).toBeEnabled()
    await user.click(screen.getByRole('button', { name: 'Save Changes' }))
    await waitFor(() =>
      expect(screen.getByRole('button', { name: 'Reset' })).toBeDisabled()
    )
    expect(request).toHaveBeenLastCalledWith(
      '/api/option/dynamic_routing',
      expect.objectContaining({ max_samples: 61, max_age_seconds: 120 }),
      expect.any(Object)
    )
  })

  test('invalid sample count identifies and focuses the field without sending a request', async () => {
    const request = vi.spyOn(api, 'put')
    const user = userEvent.setup()
    renderSettings()
    const input = screen.getByRole('spinbutton', { name: 'Maximum samples' })
    fireEvent.change(input, { target: { value: '0' } })
    await user.click(screen.getByRole('button', { name: 'Save Changes' }))
    await waitFor(() => expect(input).toHaveAttribute('aria-invalid', 'true'))
    await waitFor(() => expect(input).toHaveFocus())
    expect(request).not.toHaveBeenCalled()
  })
})
