import type { Row } from '@tanstack/react-table'
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
import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { afterEach, describe, expect, test } from 'vitest'

const { createInstance } = await import('i18next')
const { I18nextProvider, initReactI18next } = await import('react-i18next')
const { QueryClient, QueryClientProvider } =
  await import('@tanstack/react-query')
const { api } = await import('@/lib/api')
const { ROLE } = await import('@/lib/roles')
const { useAuthStore } = await import('@/stores/auth-store')
const { DataTableRowActions } = await import('../data-table-row-actions')
const { ModelChannelCapabilitiesDrawer } =
  await import('../drawers/model-channel-capabilities-drawer')
const { ModelsDialogs } = await import('../models-dialogs')
const { ModelsProvider } = await import('../models-provider')
type Model = import('../../types').Model

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
let queryClient: InstanceType<typeof QueryClient> | null = null

const model: Model = {
  id: 10,
  model_name: 'public-model',
  status: 1,
  doc_enabled: 0,
  sync_official: 0,
  created_time: 1,
  updated_time: 1,
  name_rule: 0,
}

function installCapabilityFixture(): void {
  apiClient.get = async (url, config) => {
    expect(url).toBe('/api/channel/model-capabilities')
    expect(config?.params).toEqual({ model: 'public-model' })
    return {
      data: {
        success: true,
        data: {
          model: 'public-model',
          channels: [
            {
              channel_id: 6201,
              channel_name: 'Primary channel',
              channel_type: 1,
              channel_status: 1,
              model: 'public-model',
              default_priority: 10,
              default_weight: 20,
              priority_override: 30,
              weight_override: null,
              effective_priority: 30,
              effective_weight: 20,
              groups: [
                { group: 'default', enabled: true },
                { group: 'vip', enabled: false },
              ],
              upstream_model: 'upstream-model',
              model_mapped: true,
              endpoint_types: ['openai', 'openai-response'],
              parameter_capabilities_configured: true,
              parameter_capabilities: {
                temperature: {
                  supported: true,
                  max: 1,
                  on_violation: 'clamp',
                },
                top_p: { supported: false, on_violation: 'drop' },
              },
              video_capabilities_configured: true,
              video_resolutions: ['720p', '1080p'],
              parameter_override_configured: true,
              parameter_override_mode: 'mixed',
              parameter_override_legacy: { temperature: 0.2 },
              parameter_override_operations: [
                {
                  order: 1,
                  description: 'cap output length',
                  mode: 'set',
                  path: 'max_tokens',
                  value: null,
                  value_configured: true,
                  keep_origin: false,
                  logic: 'AND',
                  conditions: [
                    {
                      order: 1,
                      path: 'original_model',
                      mode: 'full',
                      value: 'public-model',
                      invert: false,
                      pass_missing_key: false,
                    },
                  ],
                },
              ],
            },
            {
              channel_id: 6202,
              channel_name: 'Fallback channel',
              channel_type: 14,
              channel_status: 2,
              model: 'public-model',
              default_priority: 0,
              default_weight: 0,
              priority_override: null,
              weight_override: null,
              effective_priority: 0,
              effective_weight: 0,
              groups: [{ group: 'default', enabled: false }],
              upstream_model: 'public-model',
              model_mapped: false,
              endpoint_types: ['openai'],
              parameter_capabilities_configured: false,
              video_capabilities_configured: false,
              video_resolutions: [],
              parameter_override_configured: false,
              parameter_override_mode: 'none',
              parameter_override_operations: [],
              configuration_error: 'invalid channel settings',
            },
          ],
        },
      },
    }
  }
}

function renderWithProviders(children: React.ReactNode) {
  queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  return render(
    <QueryClientProvider client={queryClient}>
      <I18nextProvider i18n={i18n}>{children}</I18nextProvider>
    </QueryClientProvider>
  )
}

afterEach(() => {
  apiClient.get = originalGet
  useAuthStore.getState().auth.setUser(null)
  queryClient?.clear()
  queryClient = null
})

describe('model channel capabilities drawer', () => {
  test('shows routing, mapping, groups, endpoints, effective capabilities, and unknown configuration states', async () => {
    installCapabilityFixture()

    renderWithProviders(
      <ModelChannelCapabilitiesDrawer
        open
        onOpenChange={() => undefined}
        model={model}
      />
    )

    await waitFor(() => {
      expect(screen.getByText('Primary channel')).toBeInTheDocument()
    })
    expect(
      screen.getByRole('heading', { name: 'Model channel capabilities' })
    ).toBeInTheDocument()
    expect(
      screen.getByText('public-model → upstream-model')
    ).toBeInTheDocument()
    expect(screen.getByText('default')).toBeInTheDocument()
    expect(screen.getByText(/vip/)).toBeInTheDocument()
    expect(screen.getByText('openai-response')).toBeInTheDocument()
    expect(screen.getAllByText('temperature')).toHaveLength(2)
    expect(screen.getByText('top_p')).toBeInTheDocument()
    expect(screen.getByText('720p')).toBeInTheDocument()
    expect(screen.getByText('1080p')).toBeInTheDocument()
    expect(screen.getAllByText('Not configured')).toHaveLength(3)
    expect(screen.getAllByText('Parameter override rules')).toHaveLength(2)
    expect(screen.getByText('Mixed')).toBeInTheDocument()
    expect(screen.getByText('Evaluated at request time')).toBeInTheDocument()
    expect(screen.getByText('Ordered operations')).toBeInTheDocument()
    expect(screen.getByText('Legacy overrides')).toBeInTheDocument()
    expect(screen.getByText('cap output length')).toBeInTheDocument()
    expect(screen.getByText('max_tokens')).toBeInTheDocument()
    expect(screen.getByText('null')).toBeInTheDocument()
    expect(screen.getByText('original_model')).toBeInTheDocument()
    expect(screen.getByText('public-model')).toBeInTheDocument()
    expect(screen.getByText('invalid channel settings')).toBeInTheDocument()
    expect(screen.queryByRole('button', { name: /save/i })).toBeNull()
  })

  test('opens the read-only drawer from an exact model row action', async () => {
    installCapabilityFixture()
    useAuthStore.getState().auth.setUser({
      id: 1,
      username: 'root',
      role: ROLE.SUPER_ADMIN,
    })
    const row = { original: model } as Row<Model>

    renderWithProviders(
      <ModelsProvider>
        <DataTableRowActions row={row} />
        <ModelsDialogs />
      </ModelsProvider>
    )

    fireEvent.click(
      screen.getByRole('button', { name: 'View channel capabilities' })
    )

    await waitFor(() => {
      expect(screen.getByText('Primary channel')).toBeInTheDocument()
    })
    expect(
      screen.getByRole('heading', { name: 'Model channel capabilities' })
    ).toBeInTheDocument()
  })

  test('shows loading, empty, and failed request states without treating failures as empty data', async () => {
    apiClient.get = () => new Promise(() => undefined)
    const loadingView = renderWithProviders(
      <ModelChannelCapabilitiesDrawer
        open
        onOpenChange={() => undefined}
        model={model}
      />
    )
    expect(screen.getByLabelText('Loading')).toBeInTheDocument()
    loadingView.unmount()
    queryClient?.clear()

    apiClient.get = async () => ({
      data: {
        success: true,
        data: { model: 'public-model', channels: [] },
      },
    })
    const emptyView = renderWithProviders(
      <ModelChannelCapabilitiesDrawer
        open
        onOpenChange={() => undefined}
        model={model}
      />
    )
    await waitFor(() => {
      expect(
        screen.getByText('No channels support this exact model.')
      ).toBeInTheDocument()
    })
    emptyView.unmount()
    queryClient?.clear()

    apiClient.get = async () => ({
      data: { success: false, message: 'database unavailable' },
    })
    renderWithProviders(
      <ModelChannelCapabilitiesDrawer
        open
        onOpenChange={() => undefined}
        model={model}
      />
    )
    await waitFor(() => {
      expect(screen.getByText('database unavailable')).toBeInTheDocument()
    })
    expect(
      screen.queryByText('No channels support this exact model.')
    ).toBeNull()
  })

  test('hides the channel capability action without channel read permission', () => {
    useAuthStore.getState().auth.setUser({
      id: 2,
      username: 'restricted-admin',
      role: ROLE.ADMIN,
      permissions: { admin_permissions: { channel: { read: false } } },
    })

    renderWithProviders(
      <ModelsProvider>
        <DataTableRowActions row={{ original: model } as Row<Model>} />
      </ModelsProvider>
    )

    expect(
      screen.queryByRole('button', { name: 'View channel capabilities' })
    ).toBeNull()
  })
})
