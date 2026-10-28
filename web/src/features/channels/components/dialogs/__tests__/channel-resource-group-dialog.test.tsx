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
import { act, render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import i18next from 'i18next'
import { describe, expect, test, vi } from 'vitest'

import zh from '@/i18n/locales/zh.json'
import { api } from '@/lib/api'

import { ChannelResourceGroupDialog } from '../channel-resource-group-dialog'

function renderDialog(channelName = 'from Vendor') {
  const onOpenChange = vi.fn()
  const onApplied = vi.fn()
  const client = new QueryClient({
    defaultOptions: { mutations: { retry: false } },
  })
  render(
    <QueryClientProvider client={client}>
      <ChannelResourceGroupDialog
        channelId={12}
        channelName={channelName}
        onOpenChange={onOpenChange}
        onApplied={onApplied}
      />
    </QueryClientProvider>
  )
  return { onOpenChange, onApplied }
}

describe('channel resource group confirmation', () => {
  test('switching the interface language updates copy while preserving the group name', async () => {
    const user = userEvent.setup()
    i18next.addResourceBundle('zhCN', 'translation', zh.translation)
    try {
      renderDialog()
      const input = screen.getByRole('textbox', { name: 'Group name' })
      await user.clear(input)
      await user.type(input, '资源-custom')
      await act(async () => {
        await i18next.changeLanguage('zhCN')
      })
      expect(
        screen.getByRole('alertdialog', { name: '创建资源分组' })
      ).toBeVisible()
      expect(input).toHaveValue('资源-custom')
      expect(screen.getByRole('button', { name: '创建并关联' })).toBeVisible()
    } finally {
      await act(async () => {
        await i18next.changeLanguage('en')
      })
      i18next.removeResourceBundle('zhCN', 'translation')
    }
  })
  test('prefills the full channel name and waits for confirmation before creating', async () => {
    const user = userEvent.setup()
    const request = vi.spyOn(api, 'post').mockResolvedValue({
      data: {
        success: true,
        data: {
          group_name: '资源-from Vendor',
          channel_groups: ['default', '资源-from Vendor'],
          created: true,
          ratio: 1,
        },
      },
    })
    const { onOpenChange, onApplied } = renderDialog()
    expect(screen.getByRole('textbox', { name: 'Group name' })).toHaveValue(
      '资源-from Vendor'
    )
    expect(request).not.toHaveBeenCalled()
    await user.click(
      screen.getByRole('button', { name: 'Create and associate' })
    )
    await waitFor(() => expect(onOpenChange).toHaveBeenCalledWith(false))
    expect(request).toHaveBeenCalledWith(
      '/api/channel/12/resource-group',
      { group_name: '资源-from Vendor', reuse_existing: false },
      expect.anything()
    )
    expect(onApplied).toHaveBeenCalledWith('资源-from Vendor')
  })

  test('Enter submits an edited name and trims surrounding whitespace', async () => {
    const user = userEvent.setup()
    const request = vi.spyOn(api, 'post').mockResolvedValue({
      data: { success: true, data: { channel_groups: ['资源-custom'] } },
    })
    renderDialog()
    const input = screen.getByRole('textbox', { name: 'Group name' })
    await user.clear(input)
    await user.type(input, ' 资源-custom {Enter}')
    await waitFor(() =>
      expect(request).toHaveBeenCalledWith(
        '/api/channel/12/resource-group',
        { group_name: '资源-custom', reuse_existing: false },
        expect.anything()
      )
    )
  })

  test.each(['资源-', 'Other', '资源-a,b', `资源-${'长'.repeat(62)}`])(
    'rejects invalid name %s without a request',
    async (name) => {
      const user = userEvent.setup()
      const request = vi.spyOn(api, 'post')
      renderDialog(name)
      const input = screen.getByRole('textbox', { name: 'Group name' })
      await user.clear(input)
      await user.type(input, name)
      await user.click(
        screen.getByRole('button', { name: 'Create and associate' })
      )
      expect(
        await screen.findByText(
          'Use the 资源- prefix and a name of up to 64 characters without commas or control characters.'
        )
      ).toBeVisible()
      expect(input).toHaveAttribute('aria-invalid', 'true')
      expect(request).not.toHaveBeenCalled()
    }
  )

  test('shows an existing group and requires a separate explicit reuse confirmation', async () => {
    const user = userEvent.setup()
    const request = vi
      .spyOn(api, 'post')
      .mockResolvedValueOnce({
        data: {
          success: false,
          code: 'RESOURCE_GROUP_EXISTS',
          data: {
            group_name: '资源-from Vendor',
            ratio: 0.7,
            channels: [{ id: 8, name: 'Existing Vendor' }],
          },
        },
      })
      .mockResolvedValueOnce({
        data: {
          success: true,
          data: { channel_groups: ['default', '资源-from Vendor'] },
        },
      })
    const { onOpenChange } = renderDialog()
    await user.click(
      screen.getByRole('button', { name: 'Create and associate' })
    )
    expect(await screen.findByRole('status')).toHaveTextContent(
      'Existing Vendor (#8)'
    )
    expect(screen.getByRole('status')).toHaveTextContent('0.7')
    expect(onOpenChange).not.toHaveBeenCalled()
    await user.click(
      screen.getByRole('button', { name: 'Reuse and associate' })
    )
    await waitFor(() =>
      expect(request).toHaveBeenLastCalledWith(
        '/api/channel/12/resource-group',
        { group_name: '资源-from Vendor', reuse_existing: true },
        expect.anything()
      )
    )
  })

  test('editing after a collision clears the reuse confirmation', async () => {
    const user = userEvent.setup()
    const request = vi
      .spyOn(api, 'post')
      .mockResolvedValueOnce({
        data: {
          success: false,
          code: 'RESOURCE_GROUP_EXISTS',
          data: { group_name: '资源-from Vendor', ratio: 1, channels: [] },
        },
      })
      .mockResolvedValueOnce({
        data: { success: true, data: { channel_groups: ['资源-new'] } },
      })
    renderDialog()
    await user.click(
      screen.getByRole('button', { name: 'Create and associate' })
    )
    await screen.findByRole('button', { name: 'Reuse and associate' })
    const input = screen.getByRole('textbox', { name: 'Group name' })
    await user.clear(input)
    await user.type(input, '资源-new')
    expect(screen.queryByRole('status')).not.toBeInTheDocument()
    await user.click(
      screen.getByRole('button', { name: 'Create and associate' })
    )
    await waitFor(() =>
      expect(request).toHaveBeenLastCalledWith(
        '/api/channel/12/resource-group',
        { group_name: '资源-new', reuse_existing: false },
        expect.anything()
      )
    )
  })

  test('keeps the dialog open after failure and prevents duplicate submission while pending', async () => {
    const user = userEvent.setup()
    let reject: (error: Error) => void = () => undefined
    const request = vi.spyOn(api, 'post').mockImplementation(
      () =>
        new Promise((_, rejectRequest) => {
          reject = rejectRequest
        })
    )
    const { onOpenChange } = renderDialog()
    await user.click(
      screen.getByRole('button', { name: 'Create and associate' })
    )
    expect(
      screen.getByRole('button', { name: 'Create and associate' })
    ).toBeDisabled()
    expect(screen.getByRole('button', { name: 'Cancel' })).toBeDisabled()
    expect(screen.getByRole('textbox')).toBeDisabled()
    reject(new Error('Network unavailable'))
    await waitFor(() =>
      expect(
        screen.getByRole('button', { name: 'Create and associate' })
      ).toBeEnabled()
    )
    expect(onOpenChange).not.toHaveBeenCalled()
    expect(request).toHaveBeenCalledTimes(1)
  })

  test('cancel closes the dialog without creating a group', async () => {
    const user = userEvent.setup()
    const request = vi.spyOn(api, 'post')
    const { onOpenChange } = renderDialog()
    await user.click(screen.getByRole('button', { name: 'Cancel' }))
    expect(onOpenChange).toHaveBeenCalledWith(false)
    expect(request).not.toHaveBeenCalled()
  })
})
