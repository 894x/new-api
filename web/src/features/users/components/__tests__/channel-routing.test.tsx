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
  act,
  fireEvent,
  render,
  screen,
  waitFor,
  within,
} from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import { api } from '@/lib/api'
import { useAuthStore } from '@/stores/auth-store'

import type { UserChannelRoutingConfig } from '../../types'
import {
  UserChannelRoutingEditor,
  UserChannelRoutingRuleEditor,
} from '../user-channel-routing-dialog'
import { UserModelChannelRouting } from '../user-model-channel-routing'

vi.mock('@/lib/api', () => ({ api: { get: vi.fn(), patch: vi.fn() } }))

const config: UserChannelRoutingConfig = {
  model: 'gpt-5',
  models: [],
  revision: 'v1',
  channels: [
    {
      channel_id: 1,
      channel_name: 'Alpha',
      default_priority: 100,
      model_priority: null,
      inherited_priority: 100,
      priority_override: null,
      effective_priority: 100,
      enabled: true,
      available: true,
    },
    {
      channel_id: 2,
      channel_name: 'Beta',
      default_priority: 10,
      model_priority: 20,
      inherited_priority: 20,
      priority_override: null,
      effective_priority: 20,
      enabled: true,
      available: true,
    },
  ],
}

function renderEditor(data = config, onDirtyChange = vi.fn()) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  })
  return render(
    <QueryClientProvider client={client}>
      <UserChannelRoutingEditor
        userId={42}
        config={data}
        onDirtyChange={onDirtyChange}
        onReload={vi.fn()}
      />
    </QueryClientProvider>
  )
}

describe('user channel routing', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    useAuthStore.getState().auth.setUser({ id: 1, username: 'root', role: 100 })
    vi.mocked(api.patch).mockResolvedValue({ data: { success: true } })
  })

  it('disables a dirty routing draft and rejects reordering after write permission is revoked', () => {
    const actor = {
      id: 1,
      username: 'operator',
      role: 10,
      permissions: {
        admin_permissions: { user_group_config: { read: true, write: true } },
      },
    }
    useAuthStore.getState().auth.setUser(actor)
    renderEditor()
    const priority = screen.getByRole('spinbutton', {
      name: 'Priority for Alpha #1',
    })
    fireEvent.change(priority, { target: { value: '7' } })
    act(() =>
      useAuthStore.getState().auth.setUser({
        ...actor,
        permissions: {
          admin_permissions: {
            user_group_config: { read: true, write: false },
          },
        },
      })
    )
    expect(priority).toBeDisabled()
    const reorder = screen.getByRole('button', { name: 'Reorder Beta #2' })
    expect(reorder).toBeDisabled()
    expect(reorder).toHaveAttribute('draggable', 'false')
    fireEvent.keyDown(reorder, { key: 'ArrowUp' })
    fireEvent.dragStart(reorder, { dataTransfer: { setData: vi.fn() } })
    fireEvent.drop(screen.getByRole('listitem', { name: 'Alpha #1' }))
    expect(screen.getAllByRole('listitem')[0]).toHaveAccessibleName('Alpha #1')
    expect(priority).toHaveValue(7)
    expect(screen.getByRole('button', { name: 'Save changes' })).toBeDisabled()
    fireEvent.click(screen.getByRole('button', { name: 'Save changes' }))
    expect(api.patch).not.toHaveBeenCalled()
  })

  it('saves one Key group without changing the same channel or an unsaved draft in another group', async () => {
    vi.mocked(api.get).mockImplementation(async (_url, options) => {
      const group = options?.params?.routing_group as string
      return {
        data: {
          success: true,
          data: {
            ...config,
            routing_group: group,
            revision: `revision-${group}`,
            channels: config.channels.map((row) => ({
              ...row,
              configured: true,
            })),
          },
        },
      }
    })
    const client = new QueryClient({
      defaultOptions: { queries: { retry: false } },
    })
    render(
      <QueryClientProvider client={client}>
        <UserChannelRoutingRuleEditor
          userId={42}
          model='gpt-5'
          keyGroups={['pool-a', 'pool-b', 'unconfigured']}
          configuredGroups={['pool-a', 'pool-b']}
        />
      </QueryClientProvider>
    )
    const a = await screen.findByRole('region', { name: 'pool-a' })
    const inputA = await within(a).findByRole('spinbutton', {
      name: 'Priority for Alpha #1',
    })
    const b = await screen.findByRole('region', { name: 'pool-b' })
    expect(a).toBeVisible()
    expect(b).toBeVisible()
    expect(
      screen.queryByRole('region', { name: 'unconfigured' })
    ).not.toBeInTheDocument()
    const inputB = await within(b).findByRole('spinbutton', {
      name: 'Priority for Alpha #1',
    })
    fireEvent.change(inputB, { target: { value: '200' } })
    fireEvent.change(inputA, { target: { value: '7' } })
    fireEvent.click(within(a).getByRole('button', { name: 'Save changes' }))
    await waitFor(() =>
      expect(api.patch).toHaveBeenCalledWith(
        '/api/user/42/channel-routing-overrides',
        {
          model: 'gpt-5',
          routing_group: 'pool-a',
          revision: 'revision-pool-a',
          overrides: [{ channel_id: 1, priority_override: 7, enabled: true }],
        }
      )
    )
    expect(inputB).toHaveValue(200)
    expect(
      within(b).getByRole('button', { name: 'Save changes' })
    ).toBeEnabled()
    fireEvent.click(within(b).getByRole('button', { name: 'Save changes' }))
    await waitFor(() =>
      expect(api.patch).toHaveBeenCalledWith(
        expect.any(String),
        expect.objectContaining({
          routing_group: 'pool-b',
          overrides: [{ channel_id: 1, priority_override: 200, enabled: true }],
        })
      )
    )
  })

  it('adds a channel within its Key group and saves it without adding it to another group', async () => {
    const operator = userEvent.setup()
    vi.mocked(api.get).mockImplementation(async (_url, options) => ({
      data: {
        success: true,
        data: {
          ...config,
          routing_group: options?.params?.routing_group,
          channels: config.channels.map((row) => ({
            ...row,
            configured: false,
          })),
        },
      },
    }))
    const client = new QueryClient({
      defaultOptions: { queries: { retry: false } },
    })
    render(
      <QueryClientProvider client={client}>
        <UserChannelRoutingRuleEditor
          userId={42}
          model='gpt-5'
          keyGroups={['pool-a', 'pool-b', 'unconfigured']}
          configuredGroups={['pool-a', 'pool-b']}
        />
      </QueryClientProvider>
    )
    const a = await screen.findByRole('region', { name: 'pool-a' })
    const selector = await within(a).findByRole('combobox', {
      name: 'Select channel',
    })
    expect(
      within(a).getByRole('button', { name: 'Add channel' })
    ).toBeDisabled()
    await operator.type(selector, 'Alpha')
    await operator.click(
      await screen.findByRole('option', { name: 'Alpha #1' })
    )
    await operator.click(within(a).getByRole('button', { name: 'Add channel' }))
    expect(
      within(a).getByRole('listitem', { name: 'Alpha #1' })
    ).toBeInTheDocument()
    await operator.click(
      within(a).getByRole('button', { name: 'Save changes' })
    )
    await waitFor(() =>
      expect(api.patch).toHaveBeenCalledWith(
        expect.any(String),
        expect.objectContaining({
          routing_group: 'pool-a',
          overrides: [{ channel_id: 1, priority_override: 100, enabled: true }],
        })
      )
    )
    const b = await screen.findByRole('region', { name: 'pool-b' })
    expect(within(b).queryByRole('listitem')).not.toBeInTheDocument()
    expect(
      within(b).getByRole('button', { name: 'Save changes' })
    ).toBeDisabled()
  })

  it('dragging a channel persists descending user priorities and keeps paused rows', async () => {
    renderEditor()
    const dataTransfer = { effectAllowed: '', setData: vi.fn() }
    fireEvent.dragStart(
      screen.getByRole('button', { name: 'Reorder Beta #2' }),
      { dataTransfer }
    )
    fireEvent.dragOver(screen.getByRole('listitem', { name: 'Alpha #1' }), {
      dataTransfer,
    })
    fireEvent.drop(screen.getByRole('listitem', { name: 'Alpha #1' }), {
      dataTransfer,
    })
    expect(screen.getAllByRole('listitem')[0]).toHaveAccessibleName('Beta #2')
    fireEvent.click(
      screen.getByRole('switch', { name: 'Enable Beta #2 for this user' })
    )
    expect(
      screen.getByRole('spinbutton', { name: 'Priority for Beta #2' })
    ).toHaveValue(2)
    expect(
      screen.getByRole('switch', { name: 'Enable Beta #2 for this user' })
    ).not.toBeChecked()
    fireEvent.click(screen.getByRole('button', { name: 'Save changes' }))
    await waitFor(() =>
      expect(api.patch).toHaveBeenCalledWith(
        '/api/user/42/channel-routing-overrides',
        {
          model: 'gpt-5',
          revision: 'v1',
          overrides: [
            { channel_id: 2, priority_override: 2, enabled: false },
            { channel_id: 1, priority_override: 1, enabled: true },
          ],
        }
      )
    )
  })

  it('keyboard and move buttons reorder channels with bounded end positions', () => {
    renderEditor()
    expect(
      screen.getByRole('button', { name: 'Move Alpha #1 up' })
    ).toBeDisabled()
    fireEvent.keyDown(screen.getByRole('button', { name: 'Reorder Beta #2' }), {
      key: 'ArrowUp',
    })
    expect(screen.getAllByRole('listitem')[0]).toHaveAccessibleName('Beta #2')
    fireEvent.click(screen.getByRole('button', { name: 'Move Beta #2 down' }))
    expect(screen.getAllByRole('listitem')[1]).toHaveAccessibleName('Beta #2')
    expect(
      screen.getByRole('button', { name: 'Move Beta #2 down' })
    ).toBeDisabled()
  })

  it('re-enabling a paused channel preserves its priority, including explicit zero', async () => {
    renderEditor({
      ...config,
      channels: [
        {
          ...config.channels[0],
          priority_override: 0,
          effective_priority: 0,
          enabled: false,
        },
      ],
    })
    fireEvent.click(
      screen.getByRole('switch', { name: 'Enable Alpha #1 for this user' })
    )
    fireEvent.click(screen.getByRole('button', { name: 'Save changes' }))
    await waitFor(() =>
      expect(api.patch).toHaveBeenCalledWith(
        expect.any(String),
        expect.objectContaining({
          overrides: [{ channel_id: 1, priority_override: 0, enabled: true }],
        })
      )
    )
  })

  it('restoring priority inheritance does not enable a paused channel', async () => {
    renderEditor({
      ...config,
      channels: [
        { ...config.channels[0], priority_override: 5, enabled: false },
      ],
    })
    fireEvent.click(screen.getByRole('button', { name: 'Inherit' }))
    fireEvent.click(screen.getByRole('button', { name: 'Save changes' }))
    await waitFor(() =>
      expect(api.patch).toHaveBeenCalledWith(
        expect.any(String),
        expect.objectContaining({
          overrides: [
            { channel_id: 1, priority_override: null, enabled: false },
          ],
        })
      )
    )
  })

  it('failed saving preserves the draft and shows a reload action', async () => {
    vi.mocked(api.patch).mockRejectedValue(new Error('conflict'))
    renderEditor()
    fireEvent.change(
      screen.getByRole('spinbutton', { name: 'Priority for Alpha #1' }),
      { target: { value: '7' } }
    )
    fireEvent.click(screen.getByRole('button', { name: 'Save changes' }))
    expect(await screen.findByRole('alert')).toHaveTextContent(
      'Changes were not saved'
    )
    expect(
      screen.getByRole('spinbutton', { name: 'Priority for Alpha #1' })
    ).toHaveValue(7)
    expect(screen.getByRole('button', { name: 'Reload' })).toBeEnabled()
  })

  it('invalid priorities cannot be saved and untouched rows are omitted', async () => {
    renderEditor()
    const input = screen.getByRole('spinbutton', {
      name: 'Priority for Beta #2',
    })
    fireEvent.change(input, { target: { value: '-1' } })
    fireEvent.click(screen.getByRole('button', { name: 'Save changes' }))
    await waitFor(() => expect(input).toHaveAttribute('aria-invalid', 'true'))
    expect(api.patch).not.toHaveBeenCalled()
    fireEvent.change(input, { target: { value: '0' } })
    fireEvent.click(screen.getByRole('button', { name: 'Save changes' }))
    await waitFor(() =>
      expect(api.patch).toHaveBeenCalledWith(
        expect.any(String),
        expect.objectContaining({
          overrides: [{ channel_id: 2, priority_override: 0, enabled: true }],
        })
      )
    )
  })

  it('shows an empty state without a save button when no channels are available', () => {
    renderEditor({ ...config, channels: [] })
    expect(screen.getByText('No available channels')).toBeInTheDocument()
    expect(
      screen.queryByRole('button', { name: 'Save changes' })
    ).not.toBeInTheDocument()
  })

  it('adds a model without resource pools and only offers published Key groups', async () => {
    const operator = userEvent.setup()
    let configuredModels: string[] = []
    vi.mocked(api.get).mockImplementation(async (_url, options) => ({
      data: {
        success: true,
        data: options?.params?.model
          ? {
              ...config,
              routing_group: options.params.routing_group,
              channels: config.channels.map((row) => ({
                ...row,
                configured: true,
              })),
            }
          : {
              ...config,
              models: ['gpt-5'],
              configured_models: configuredModels,
              configured_key_groups: configuredModels.length
                ? { 'gpt-5': ['vip'] }
                : {},
              key_groups: { standard: 'Standard', vip: 'VIP' },
            },
      },
    }))
    vi.mocked(api.patch).mockImplementation(async () => {
      configuredModels = ['gpt-5']
      return { data: { success: true } }
    })
    const client = new QueryClient({
      defaultOptions: { queries: { retry: false } },
    })
    const view = render(
      <QueryClientProvider client={client}>
        <UserModelChannelRouting
          userId={42}
          poolModels={[]}
          onDirtyChange={vi.fn()}
        />
      </QueryClientProvider>
    )
    const modelInput = await screen.findByRole('combobox', { name: 'Model' })
    expect(
      screen.queryByRole('combobox', { name: 'Key group' })
    ).not.toBeInTheDocument()
    await operator.type(modelInput, 'gpt-5')
    await operator.click(screen.getByRole('button', { name: 'Add model' }))
    expect(
      await screen.findByRole('heading', { name: 'gpt-5' })
    ).toBeInTheDocument()
    const groupInput = screen.getByRole('combobox', { name: 'Key group' })
    await operator.click(groupInput)
    expect(
      screen.getAllByRole('option').map((option) => option.textContent)
    ).toEqual(['standard', 'vip'])
    await operator.click(screen.getByRole('option', { name: 'vip' }))
    expect(
      screen.queryByRole('region', { name: 'vip' })
    ).not.toBeInTheDocument()
    await operator.click(screen.getByRole('button', { name: 'Add Key group' }))
    const priority = await screen.findByRole('spinbutton', {
      name: 'Priority for Alpha #1',
    })
    fireEvent.change(priority, { target: { value: '7' } })
    await operator.click(screen.getByRole('button', { name: 'Save changes' }))
    await waitFor(() =>
      expect(api.patch).toHaveBeenCalledWith(
        expect.any(String),
        expect.objectContaining({ model: 'gpt-5', routing_group: 'vip' })
      )
    )
    expect(api.get).toHaveBeenCalledWith(
      '/api/user/42/channel-routing-overrides',
      { params: { model: 'gpt-5', routing_group: 'vip' } }
    )
    view.unmount()
    const reopenedClient = new QueryClient({
      defaultOptions: { queries: { retry: false } },
    })
    render(
      <QueryClientProvider client={reopenedClient}>
        <UserModelChannelRouting
          userId={42}
          poolModels={[]}
          onDirtyChange={vi.fn()}
        />
      </QueryClientProvider>
    )
    expect(
      await screen.findByRole('heading', { name: 'gpt-5' })
    ).toBeInTheDocument()
    expect(await screen.findByRole('region', { name: 'vip' })).toBeVisible()
    expect(
      screen.queryByRole('region', { name: 'standard' })
    ).not.toBeInTheDocument()
  })

  it('shows all assigned public groups together inside their own model frame', async () => {
    vi.mocked(api.get).mockImplementation(async (_url, options) => ({
      data: {
        success: true,
        data: options?.params?.model
          ? {
              ...config,
              routing_group: options.params.routing_group,
              channels: [],
            }
          : {
              ...config,
              configured_models: ['gpt-5', 'gpt-6'],
              key_groups: {
                standard: 'Standard',
                vip: 'VIP',
                unassigned: 'Unassigned',
              },
              configured_key_groups: {
                'gpt-5': ['standard', 'vip', 'internal-pool'],
                'gpt-6': ['standard'],
              },
            },
      },
    }))
    const client = new QueryClient({
      defaultOptions: { queries: { retry: false } },
    })
    render(
      <QueryClientProvider client={client}>
        <UserModelChannelRouting
          userId={42}
          poolModels={[]}
          onDirtyChange={vi.fn()}
        />
      </QueryClientProvider>
    )
    const model = await screen.findByRole('region', { name: 'gpt-5' })
    expect(
      within(model).getByRole('region', { name: 'standard' })
    ).toBeVisible()
    expect(within(model).getByRole('region', { name: 'vip' })).toBeVisible()
    expect(
      within(model).queryByRole('region', { name: 'unassigned' })
    ).not.toBeInTheDocument()
    expect(
      within(model).queryByRole('region', { name: 'internal-pool' })
    ).not.toBeInTheDocument()
    const otherModel = screen.getByRole('region', { name: 'gpt-6' })
    expect(
      within(otherModel).getByRole('region', { name: 'standard' })
    ).toBeVisible()
    expect(
      within(otherModel).queryByRole('region', { name: 'vip' })
    ).not.toBeInTheDocument()
  })

  it('shows an empty state when the customer has no public Key groups', async () => {
    vi.mocked(api.get).mockResolvedValue({
      data: { success: true, data: { ...config, key_groups: {} } },
    })
    const client = new QueryClient({
      defaultOptions: { queries: { retry: false } },
    })
    render(
      <QueryClientProvider client={client}>
        <UserModelChannelRouting
          userId={42}
          poolModels={['gpt-5']}
          onDirtyChange={vi.fn()}
        />
      </QueryClientProvider>
    )
    expect(
      await screen.findByText('No public Key groups available')
    ).toBeInTheDocument()
    expect(screen.queryByRole('combobox')).not.toBeInTheDocument()
    expect(api.patch).not.toHaveBeenCalled()
  })

  it('preserves another group draft when saving triggers a failed background catalog refresh', async () => {
    const operator = userEvent.setup()
    let catalogFailed = false
    vi.mocked(api.get).mockImplementation(async (_url, options) => {
      if (!options?.params?.model) {
        if (catalogFailed) throw new Error('catalog unavailable')
        return {
          data: {
            success: true,
            data: {
              ...config,
              configured_models: ['gpt-5'],
              key_groups: { standard: 'Standard', vip: 'VIP' },
              configured_key_groups: { 'gpt-5': ['standard', 'vip'] },
            },
          },
        }
      }
      return {
        data: {
          success: true,
          data: {
            ...config,
            routing_group: options.params.routing_group,
            channels: config.channels.map((row) => ({
              ...row,
              configured: true,
            })),
          },
        },
      }
    })
    const client = new QueryClient({
      defaultOptions: { queries: { retry: false } },
    })
    render(
      <QueryClientProvider client={client}>
        <UserModelChannelRouting
          userId={42}
          poolModels={[]}
          onDirtyChange={vi.fn()}
        />
      </QueryClientProvider>
    )
    const a = await screen.findByRole('region', { name: 'standard' })
    const b = await screen.findByRole('region', { name: 'vip' })
    fireEvent.change(
      await within(b).findByRole('spinbutton', {
        name: 'Priority for Alpha #1',
      }),
      { target: { value: '200' } }
    )
    fireEvent.change(
      await within(a).findByRole('spinbutton', {
        name: 'Priority for Alpha #1',
      }),
      { target: { value: '7' } }
    )
    catalogFailed = true
    await operator.click(
      within(a).getByRole('button', { name: 'Save changes' })
    )
    const retry = await screen.findByRole('button', { name: 'Retry' })
    expect(
      within(screen.getByRole('region', { name: 'vip' })).getByRole(
        'spinbutton',
        { name: 'Priority for Alpha #1' }
      )
    ).toHaveValue(200)
    catalogFailed = false
    await operator.click(retry)
    await waitFor(() =>
      expect(
        screen.queryByRole('button', { name: 'Retry' })
      ).not.toBeInTheDocument()
    )
    expect(
      within(screen.getByRole('region', { name: 'vip' })).getByRole(
        'spinbutton',
        { name: 'Priority for Alpha #1' }
      )
    ).toHaveValue(200)
  })

  it('retries a failed model catalog without treating the error as an empty group list', async () => {
    vi.mocked(api.get).mockResolvedValueOnce({
      data: { success: false, message: 'unavailable' },
    })
    const client = new QueryClient({
      defaultOptions: { queries: { retry: false } },
    })
    render(
      <QueryClientProvider client={client}>
        <UserModelChannelRouting
          userId={42}
          poolModels={[]}
          onDirtyChange={vi.fn()}
        />
      </QueryClientProvider>
    )
    const retry = await screen.findByRole('button', { name: 'Retry' })
    expect(
      screen.queryByText('No public Key groups available')
    ).not.toBeInTheDocument()
    vi.mocked(api.get).mockResolvedValue({
      data: { success: true, data: { ...config, key_groups: {} } },
    })
    fireEvent.click(retry)
    expect(
      await screen.findByText('No public Key groups available')
    ).toBeInTheDocument()
  })

  it('reverting a switch restores the clean state so model selection can resume', () => {
    const onDirtyChange = vi.fn()
    renderEditor(config, onDirtyChange)
    const toggle = screen.getByRole('switch', {
      name: 'Enable Alpha #1 for this user',
    })
    fireEvent.click(toggle)
    expect(onDirtyChange).toHaveBeenLastCalledWith(true)
    fireEvent.click(toggle)
    expect(onDirtyChange).toHaveBeenLastCalledWith(false)
    expect(screen.getByRole('button', { name: 'Save changes' })).toBeDisabled()
  })
})
