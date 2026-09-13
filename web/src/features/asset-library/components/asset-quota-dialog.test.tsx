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
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from '@testing-library/react'
import { afterEach, expect, it, vi } from 'vitest'

import { AssetQuotaDialog } from './asset-quota-dialog'
import { AssetStorageUsage } from './asset-storage-usage'

const request = vi.hoisted(() => ({ put: vi.fn(), get: vi.fn() }))
vi.mock('@/lib/api', () => ({ api: request }))
vi.mock('react-i18next', () => ({
  useTranslation: () => ({ t: (key: string) => key, i18n: { language: 'en' } }),
}))

afterEach(() => {
  cleanup()
  vi.clearAllMocks()
})

it.each([
  { initial: null, value: '0', expected: 0 },
  { initial: 0, value: '', expected: null },
])(
  'distinguishes explicit zero from inheritance: $expected',
  async ({ initial, value, expected }) => {
    const close = vi.fn()
    request.put.mockResolvedValue({ data: { success: true } })
    render(
      <QueryClientProvider client={new QueryClient()}>
        <AssetQuotaDialog
          path='/api/asset-library/admin/users/7/storage'
          quotaOverrideMB={initial}
          defaultQuotaMB={1024}
          onClose={close}
        />
      </QueryClientProvider>
    )
    fireEvent.change(
      screen.getByRole('spinbutton', { name: 'User asset quota (MB)' }),
      { target: { value } }
    )
    fireEvent.click(screen.getByRole('button', { name: 'Save Changes' }))
    await waitFor(() =>
      expect(request.put).toHaveBeenCalledWith(
        '/api/asset-library/admin/users/7/storage',
        { quota_mb: expected }
      )
    )
    await waitFor(() => expect(close).toHaveBeenCalledOnce())
  }
)

it('keeps the input available for retry when saving fails', async () => {
  const close = vi.fn()
  request.put.mockResolvedValueOnce({ data: { success: false } })
  render(
    <QueryClientProvider client={new QueryClient()}>
      <AssetQuotaDialog
        path='/api/asset-library/admin/users/7/storage'
        quotaOverrideMB={null}
        defaultQuotaMB={1024}
        onClose={close}
      />
    </QueryClientProvider>
  )
  const input = screen.getByRole('spinbutton', {
    name: 'User asset quota (MB)',
  })
  fireEvent.change(input, { target: { value: '512' } })
  fireEvent.click(screen.getByRole('button', { name: 'Save Changes' }))
  await screen.findByText('Failed to update asset quota')
  expect(input).toHaveValue(512)
  expect(close).not.toHaveBeenCalled()
  request.put.mockResolvedValueOnce({ data: { success: true } })
  fireEvent.click(screen.getByRole('button', { name: 'Save Changes' }))
  await waitFor(() => expect(close).toHaveBeenCalledOnce())
})

it('loads the selected user usage and only exposes editing when authorized', async () => {
  const data = {
    enabled: true,
    quota_mb: 4,
    quota_override_mb: 4,
    default_quota_mb: 1,
    used_bytes: 0,
    remaining_bytes: 4000000,
    bytes_per_mb: 1000000,
    can_edit: false,
  }
  request.get.mockResolvedValueOnce({ data: { success: true, data } })
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  const view = render(
    <QueryClientProvider client={client}>
      <AssetStorageUsage targetUserId={7} />
    </QueryClientProvider>
  )
  await screen.findByRole('status')
  expect(request.get).toHaveBeenCalledWith(
    '/api/asset-library/admin/users/7/storage'
  )
  expect(
    screen.queryByRole('button', { name: 'Set user quota' })
  ).not.toBeInTheDocument()
  request.get.mockResolvedValueOnce({
    data: { success: true, data: { ...data, can_edit: true } },
  })
  view.rerender(
    <QueryClientProvider client={client}>
      <AssetStorageUsage targetUserId={8} />
    </QueryClientProvider>
  )
  await screen.findByRole('button', { name: 'Set user quota' })
  expect(request.get).toHaveBeenLastCalledWith(
    '/api/asset-library/admin/users/8/storage'
  )
})
