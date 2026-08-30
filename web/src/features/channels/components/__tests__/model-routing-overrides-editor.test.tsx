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
import { createInstance } from 'i18next'
import { I18nextProvider, initReactI18next } from 'react-i18next'
import { afterEach, describe, expect, test } from 'vitest'

import { api } from '@/lib/api'
import { ROLE } from '@/lib/roles'
import { useAuthStore } from '@/stores/auth-store'

import { ModelRoutingOverridesEditor } from '../model-routing-overrides-editor'

const i18n = createInstance()
await i18n.use(initReactI18next).init({
  lng: 'en',
  resources: { en: { translation: {} } },
})

type ApiGet = (
  url: string,
  config?: { params?: Record<string, unknown> }
) => Promise<{ data: unknown }>
type MockableApi = { get: ApiGet }

const apiClient = api as unknown as MockableApi
const originalGet = apiClient.get
let queryClient: QueryClient | null = null

function installRoutingFixture(): void {
  apiClient.get = async () => ({
    data: {
      success: true,
      data: [
        {
          channel_id: 7,
          channel_name: 'Enabled channel',
          channel_type: 1,
          channel_status: 1,
          model: 'public-model',
          default_priority: 5,
          default_weight: 100,
          priority_override: null,
          weight_override: null,
          effective_priority: 5,
          effective_weight: 100,
        },
        {
          channel_id: 8,
          channel_name: 'Disabled channel',
          channel_type: 1,
          channel_status: 2,
          model: 'public-model',
          default_priority: 0,
          default_weight: 0,
          priority_override: null,
          weight_override: null,
          effective_priority: 0,
          effective_weight: 0,
        },
      ],
    },
  })
}

function renderEditor(): void {
  queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  render(
    <QueryClientProvider client={queryClient}>
      <I18nextProvider i18n={i18n}>
        <ModelRoutingOverridesEditor model='public-model' />
      </I18nextProvider>
    </QueryClientProvider>
  )
}

afterEach(() => {
  apiClient.get = originalGet
  useAuthStore.getState().auth.setUser(null)
  queryClient?.clear()
  queryClient = null
})

describe('model routing overrides editor', () => {
  test('shows enabled channels by default and reveals disabled channels after selecting all', async () => {
    installRoutingFixture()
    useAuthStore.getState().auth.setUser({
      id: 1,
      username: 'root',
      role: ROLE.SUPER_ADMIN,
    })

    renderEditor()

    await waitFor(() => {
      expect(screen.getByText('Enabled channel')).toBeInTheDocument()
    })
    expect(screen.queryByText('Disabled channel')).toBeNull()
    expect(screen.getByRole('button', { name: 'Enabled' })).toHaveAttribute(
      'aria-pressed',
      'true'
    )

    fireEvent.click(screen.getByRole('button', { name: 'All' }))

    expect(screen.getByText('Disabled channel')).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'All' })).toHaveAttribute(
      'aria-pressed',
      'true'
    )
  })
})
