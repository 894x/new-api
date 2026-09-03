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
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { useState } from 'react'
import { beforeEach, expect, test, vi } from 'vitest'

import { AdminAssetLibraryUserScope } from '../admin-user-scope'

vi.mock('react-i18next', () => ({
  useTranslation: () => ({ t: (key: string) => key }),
}))

const { getUsers, searchUsers } = vi.hoisted(() => ({
  getUsers: vi.fn(),
  searchUsers: vi.fn(),
}))

vi.mock('@/features/users/api', () => ({ getUsers, searchUsers }))

beforeEach(() => {
  getUsers.mockResolvedValue({
    success: true,
    data: { items: [], total: 0, page: 1, page_size: 20 },
  })
  searchUsers.mockResolvedValue({
    success: true,
    data: {
      items: [
        {
          id: 42,
          username: 'alice',
          display_name: 'Alice Zhang',
          quota: 0,
          used_quota: 0,
          request_count: 0,
          group: 'default',
          status: 1,
          role: 1,
        },
      ],
      total: 1,
      page: 1,
      page_size: 20,
    },
  })
})

test('lets an administrator search for and select a user', async () => {
  const onTargetUserIdChange = vi.fn()
  const user = userEvent.setup()
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })

  function ControlledScope() {
    const [targetUserId, setTargetUserId] = useState<number>()

    return (
      <AdminAssetLibraryUserScope
        currentUser={{ id: 1, username: 'admin', display_name: 'Admin' }}
        targetUserId={targetUserId}
        onTargetUserIdChange={(nextUserId) => {
          setTargetUserId(nextUserId)
          onTargetUserIdChange(nextUserId)
        }}
      />
    )
  }

  render(
    <QueryClientProvider client={queryClient}>
      <ControlledScope />
    </QueryClientProvider>
  )

  await user.click(
    screen.getByRole('combobox', { name: 'Select asset library owner' })
  )
  await user.type(
    screen.getByPlaceholderText('Search users by name or ID...'),
    'alice'
  )
  await user.click(await screen.findByRole('option', { name: /Alice Zhang/ }))

  expect(onTargetUserIdChange).toHaveBeenCalledWith(42)
  expect(
    screen.getByRole('combobox', { name: 'Select asset library owner' })
  ).toHaveTextContent('Alice Zhang (alice)')
  expect(searchUsers).toHaveBeenCalledWith(
    expect.objectContaining({ keyword: 'alice' })
  )
})
