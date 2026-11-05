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
  createMemoryHistory,
  createRootRoute,
  createRouter,
  RouterProvider,
} from '@tanstack/react-router'
import { act, render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import i18next from 'i18next'
import { afterEach, expect, test, vi } from 'vitest'

import { api } from '@/lib/api'
import { ROLE } from '@/lib/roles'
import { useAuthStore } from '@/stores/auth-store'

import { GroupModelWorkspace } from '../components/group-model-workspace'
import { MatrixPagination } from '../components/matrix-pagination'
import type { GroupModelMatrixData } from '../group-model-api'

const clients: QueryClient[] = []
afterEach(() => {
  clients.forEach((client) => client.clear())
  useAuthStore.getState().auth.setUser(null)
})

test('pagination numbers follow language changes for all interface languages and invalid locale fallback', async () => {
  const previousLanguage = i18next.language
  render(
    <MatrixPagination
      axis='groups'
      page={1234}
      pageSize={10}
      total={12345}
      disabled={false}
      onPageChange={vi.fn()}
    />
  )
  try {
    const fallback = new Intl.NumberFormat()
    for (const [language, total, pages] of [
      ['en', '12,345', '1,234 / 1,235'],
      ['zhCN', '12,345', '1,234 / 1,235'],
      ['zhTW', '12,345', '1,234 / 1,235'],
      ['fr', '12\u202f345', '1\u202f234 / 1\u202f235'],
      ['ja', '12,345', '1,234 / 1,235'],
      ['ru', '12\u00a0345', '1\u00a0234 / 1\u00a0235'],
      ['vi', '12.345', '1.234 / 1.235'],
      [
        'invalid_locale',
        fallback.format(12345),
        `${fallback.format(1234)} / ${fallback.format(1235)}`,
      ],
    ] as const) {
      await act(() => i18next.changeLanguage(language))
      expect(
        screen.getByText(`${i18next.t('Groups')} · ${total}`, {
          normalizer: (text) => text,
        })
      ).toBeVisible()
      expect(
        screen.getByText(pages, { normalizer: (text) => text })
      ).toBeVisible()
    }
  } finally {
    await act(() => i18next.changeLanguage(previousLanguage))
  }
})

function fixture(): GroupModelMatrixData {
  return {
    groups: ['default', 'vip'],
    models: ['model-a', 'model-b'],
    available: { default: ['model-a', 'model-b'], vip: ['model-a'] },
    config: {},
    revision: 'original',
  }
}

function mountWorkspace(data = fixture(), writable = true) {
  useAuthStore.getState().auth.setUser({
    id: 1,
    username: 'admin',
    role: ROLE.ADMIN,
    permissions: {
      admin_permissions: { channel: { read: true, write: writable } },
    },
  })
  vi.spyOn(api, 'get').mockResolvedValue({ data: { success: true, data } })
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  })
  clients.push(client)
  const onDirtyChange = vi.fn()
  const route = createRootRoute({
    component: () => (
      <QueryClientProvider client={client}>
        <GroupModelWorkspace onDirtyChange={onDirtyChange} />
      </QueryClientProvider>
    ),
  })
  const router = createRouter({
    routeTree: route,
    history: createMemoryHistory({ initialEntries: ['/'] }),
  })
  render(<RouterProvider router={router} />)
  return { onDirtyChange }
}

test('shows groups as rows and models as columns with accessible allow controls', async () => {
  mountWorkspace()
  const region = await screen.findByRole('region', {
    name: 'Group-model matrix',
  })
  expect(
    within(region).getByRole('columnheader', { name: 'model-a' })
  ).toBeVisible()
  expect(
    within(region).getByRole('columnheader', { name: 'model-b' })
  ).toBeVisible()
  expect(
    within(region).getByRole('rowheader', { name: /default/ })
  ).toBeVisible()
  expect(
    within(region).getByRole('checkbox', {
      name: 'Allow default to use model-a',
    })
  ).toBeChecked()
  expect(within(region).getByText('No channel')).toBeVisible()
  const user = userEvent.setup()
  const checkbox = within(region).getByRole('checkbox', {
    name: 'Allow default to use model-b',
  })
  checkbox.focus()
  await user.keyboard(' ')
  expect(checkbox).not.toBeChecked()
  expect(
    screen.getByRole('switch', { name: 'Restrict models for default' })
  ).toBeChecked()
  expect(
    screen.getByRole('switch', { name: 'Restrict models for vip' })
  ).not.toBeChecked()
})

test('saves a whitelist while preserving selections outside model filters', async () => {
  const put = vi.spyOn(api, 'put').mockResolvedValue({
    data: {
      success: true,
      data: { config: { default: ['model-a'] }, revision: 'saved' },
    },
  })
  const { onDirtyChange } = mountWorkspace()
  const user = userEvent.setup()
  await user.click(
    await screen.findByRole('checkbox', {
      name: 'Allow default to use model-b',
    })
  )
  await user.type(
    screen.getByRole('textbox', { name: 'Search models' }),
    'model-b'
  )
  expect(
    screen.queryByRole('columnheader', { name: 'model-a' })
  ).not.toBeInTheDocument()
  await user.click(screen.getByRole('button', { name: 'Save' }))
  await waitFor(() =>
    expect(put).toHaveBeenCalledWith('/api/channel/group-model-matrix', {
      config: { default: ['model-a'] },
      revision: 'original',
    })
  )
  await waitFor(() => expect(onDirtyChange).toHaveBeenLastCalledWith(false))
  expect(screen.getByRole('button', { name: 'Save' })).toBeDisabled()
})

test('clearing a restricted row denies all and switching restriction off restores inheritance', async () => {
  const data = fixture()
  data.config = { default: ['model-a'] }
  const put = vi.spyOn(api, 'put').mockResolvedValue({
    data: {
      success: true,
      data: { config: { default: [] }, revision: 'cleared' },
    },
  })
  mountWorkspace(data)
  const user = userEvent.setup()
  await user.click(await screen.findByRole('button', { name: 'Clear' }))
  expect(
    screen.getByRole('checkbox', { name: 'Allow default to use model-a' })
  ).not.toBeChecked()
  await user.click(screen.getByRole('button', { name: 'Save' }))
  await waitFor(() =>
    expect(put).toHaveBeenCalledWith('/api/channel/group-model-matrix', {
      config: { default: [] },
      revision: 'original',
    })
  )
  await waitFor(() =>
    expect(screen.getByRole('button', { name: 'Save' })).toBeDisabled()
  )
  await user.click(
    screen.getByRole('switch', { name: 'Restrict models for default' })
  )
  expect(
    screen.getByRole('checkbox', { name: 'Allow default to use model-b' })
  ).toBeChecked()
})

test('a conflicting save retains pending choices until they are explicitly discarded', async () => {
  vi.spyOn(api, 'put').mockResolvedValue({
    data: { success: false, message: 'matrix changed' },
  })
  mountWorkspace()
  const user = userEvent.setup()
  await user.click(
    await screen.findByRole('checkbox', {
      name: 'Allow default to use model-b',
    })
  )
  await user.click(screen.getByRole('button', { name: 'Save' }))
  await waitFor(() =>
    expect(screen.getByRole('button', { name: 'Save' })).toBeEnabled()
  )
  expect(
    screen.getByRole('checkbox', { name: 'Allow default to use model-b' })
  ).not.toBeChecked()
  await user.click(screen.getByRole('button', { name: 'Discard changes' }))
  expect(
    screen.getByRole('checkbox', { name: 'Allow default to use model-b' })
  ).toBeChecked()
})

test('read-only administrators can inspect but cannot change group model access', async () => {
  mountWorkspace(fixture(), false)
  expect(
    await screen.findByRole('checkbox', {
      name: 'Allow default to use model-a',
    })
  ).toHaveAttribute('aria-disabled', 'true')
  expect(
    screen.getByRole('switch', { name: 'Restrict models for default' })
  ).toHaveAttribute('aria-disabled', 'true')
  expect(screen.queryByRole('button', { name: 'Save' })).not.toBeInTheDocument()
})

test('model pagination preserves pending choices and full model names remain discoverable', async () => {
  const data = fixture()
  const longName = 'long-model-name-'.repeat(15)
  data.models = [
    longName,
    ...Array.from({ length: 10 }, (_, index) => `model-${index}`),
  ]
  mountWorkspace(data)
  const user = userEvent.setup()
  const heading = await screen.findByRole('columnheader', { name: longName })
  expect(within(heading).getByTitle(longName)).toBeVisible()
  await user.click(
    screen.getByRole('checkbox', { name: `Allow default to use ${longName}` })
  )
  await user.click(screen.getByRole('button', { name: 'Next models' }))
  expect(screen.getByRole('columnheader', { name: 'model-9' })).toBeVisible()
  await user.click(screen.getByRole('button', { name: 'Previous models' }))
  expect(
    screen.getByRole('checkbox', { name: `Allow default to use ${longName}` })
  ).not.toBeChecked()
})
