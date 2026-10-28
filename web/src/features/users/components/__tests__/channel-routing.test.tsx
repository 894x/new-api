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

  it('saves one outlet group without changing the same channel or an unsaved draft in another group', async () => {
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
          channelGroups={['pool-a', 'pool-b']}
        />
      </QueryClientProvider>
    )
    const a = await screen.findByRole('region', { name: 'pool-a' })
    const b = await screen.findByRole('region', { name: 'pool-b' })
    const inputA = await within(a).findByRole('spinbutton', {
      name: 'Priority for Alpha #1',
    })
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

  it('adds a channel within its outlet group and saves it without adding it to another group', async () => {
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
          channelGroups={['pool-a', 'pool-b']}
        />
      </QueryClientProvider>
    )
    const a = await screen.findByRole('region', { name: 'pool-a' })
    const b = screen.getByRole('region', { name: 'pool-b' })
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
    expect(within(b).queryByRole('listitem')).not.toBeInTheDocument()
    expect(
      within(b).getByRole('button', { name: 'Save changes' })
    ).toBeDisabled()
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
