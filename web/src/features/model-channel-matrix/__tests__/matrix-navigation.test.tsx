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
import { renderHook, waitFor } from '@testing-library/react'
import { afterEach, expect, test, vi } from 'vitest'

import { parseSidebarModulesAdmin } from '@/features/system-settings/maintenance/config'
import { useSidebarConfig } from '@/hooks/use-sidebar-config'
import { useSidebarData } from '@/hooks/use-sidebar-data'
import { api } from '@/lib/api'
import { ROLE } from '@/lib/roles'
import { useAuthStore } from '@/stores/auth-store'

import { matrixSearchSchema } from '../types'
import { createMatrixQueryFixture } from './fixtures'

let queryFixture: ReturnType<typeof createMatrixQueryFixture>
afterEach(() => {
  queryFixture?.client.clear()
  useAuthStore.getState().auth.setUser(null)
  localStorage.removeItem('status')
})

test('shows the standalone menu only to administrators with channel read access', () => {
  const view = renderHook(() => useSidebarData())
  expect(
    view.result.current.navGroups
      .flatMap((group) => group.items)
      .find((item) => item.url === '/model-channel-matrix')
  ).toBeUndefined()
  view.unmount()
  useAuthStore.getState().auth.setUser({
    id: 2,
    username: 'reader',
    role: ROLE.ADMIN,
    permissions: {
      admin_permissions: { channel: { read: true, write: false } },
    },
  })
  const reader = renderHook(() => useSidebarData())
  const items = reader.result.current.navGroups.find(
    (group) => group.id === 'admin'
  )?.items
  expect(items).toEqual(
    expect.arrayContaining([
      expect.objectContaining({ url: '/channels' }),
      expect.objectContaining({ url: '/models/metadata' }),
      expect.objectContaining({
        title: 'Model-channel matrix',
        url: '/model-channel-matrix',
      }),
    ])
  )
})

test('an independent sidebar switch hides the matrix while preserving Channels', async () => {
  useAuthStore
    .getState()
    .auth.setUser({ id: 1, username: 'root', role: ROLE.SUPER_ADMIN })
  queryFixture = createMatrixQueryFixture()
  vi.spyOn(api, 'get').mockResolvedValue({
    data: {
      data: {
        SidebarModulesAdmin: JSON.stringify({
          admin: { enabled: true, channel: true, model_channel_matrix: false },
        }),
      },
    },
  })
  const view = renderHook(() => useSidebarConfig(useSidebarData().navGroups), {
    wrapper: queryFixture.wrapper,
  })
  await waitFor(() =>
    expect(
      view.result.current
        .flatMap((group) => group.items)
        .find((item) => item.url === '/model-channel-matrix')
    ).toBeUndefined()
  )
  expect(
    view.result.current
      .flatMap((group) => group.items)
      .find((item) => item.url === '/channels')
  ).toBeDefined()
})

test('legacy sidebar settings enable the new menu and invalid URL filters recover to defaults', () => {
  expect(
    parseSidebarModulesAdmin(
      JSON.stringify({ admin: { enabled: true, channel: true } })
    ).admin.model_channel_matrix
  ).toBe(true)
  expect(
    matrixSearchSchema.parse({
      model_page: -1,
      channel_page: 'bad',
      status: 'unknown',
      model: 'x'.repeat(256),
    })
  ).toEqual({
    model: '',
    channel: '',
    group: '',
    status: 'enabled',
    model_page: 1,
    channel_page: 1,
  })
})
