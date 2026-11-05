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
import {
  act,
  fireEvent,
  render,
  screen,
  waitFor,
  within,
} from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, expect, test, vi } from 'vitest'

import { api } from '@/lib/api'
import { ROLE } from '@/lib/roles'
import { useAuthStore } from '@/stores/auth-store'

import { ModelRoutingWorkspace } from '..'

const clients: QueryClient[] = []
afterEach(() => {
  clients.forEach((client) => client.clear())
  useAuthStore.getState().auth.setUser(null)
})

function mountWorkspace() {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  })
  clients.push(client)
  const onDirtyChange = vi.fn()
  const route = createRootRoute({
    component: () => (
      <QueryClientProvider client={client}>
        <ModelRoutingWorkspace onDirtyChange={onDirtyChange} />
      </QueryClientProvider>
    ),
  })
  const router = createRouter({
    routeTree: route,
    history: createMemoryHistory({ initialEntries: ['/'] }),
  })
  render(<RouterProvider router={router} />)
  return { onDirtyChange, client }
}

test('mounts usable filters while catalog and routing pools load independently', async () => {
  useAuthStore
    .getState()
    .auth.setUser({ id: 1, username: 'root', role: ROLE.SUPER_ADMIN })
  let resolveCatalog!: (value: unknown) => void
  let resolvePools!: (value: unknown) => void
  const get = vi.spyOn(api, 'get').mockImplementation(
    (url) =>
      new Promise((resolve) => {
        if (url.endsWith('/catalog')) resolveCatalog = resolve
        else resolvePools = resolve
      })
  )
  mountWorkspace()
  expect(
    await screen.findByRole('combobox', { name: 'Search models' })
  ).toBeEnabled()
  expect(
    screen.getByRole('combobox', { name: 'Filter scheduling strategy' })
  ).toHaveTextContent('All strategies')
  expect(
    screen.getByRole('button', { name: 'Configure routing pool' })
  ).toBeEnabled()
  expect(get).toHaveBeenCalledWith(
    '/api/channel/routing-pools/catalog',
    expect.anything()
  )
  expect(get).toHaveBeenCalledWith(
    '/api/channel/routing-pools',
    expect.anything()
  )
  await act(async () =>
    resolvePools({
      data: {
        success: true,
        data: { items: [], total: 0, page: 1, page_size: 12 },
      },
    })
  )
  expect(await screen.findByText('No configured routing pools')).toBeVisible()
  await act(async () =>
    resolveCatalog({
      data: {
        success: true,
        data: {
          groups: { standard: 'Standard' },
          models: ['kimi-k3', 'gpt-4o'],
        },
      },
    })
  )
  const model = screen.getByRole('combobox', { name: 'Search models' })
  expect(model).toBeEnabled()
  await userEvent.click(model)
  await userEvent.clear(model)
  await userEvent.type(model, 'kimi')
  expect(await screen.findByRole('option', { name: 'kimi-k3' })).toBeVisible()
  expect(
    screen.queryByRole('option', { name: 'gpt-4o' })
  ).not.toBeInTheDocument()
  await userEvent.click(screen.getByRole('option', { name: 'kimi-k3' }))
  expect(model).toHaveValue('kimi-k3')
})

test('retains edited cards and locks filtering until the administrator resets the draft', async () => {
  useAuthStore
    .getState()
    .auth.setUser({ id: 1, username: 'root', role: ROLE.SUPER_ADMIN })
  vi.spyOn(api, 'get').mockImplementation(async (url) => ({
    data: {
      success: true,
      data: url.endsWith('/catalog')
        ? { groups: { standard: 'Standard' }, models: ['kimi-k3'] }
        : {
            items: [
              {
                routing_group: 'standard',
                model: 'kimi-k3',
                mode: 'priority',
                revision: 'r1',
                channels: [
                  { channel_id: 1, priority: 100, weight: 10, enabled: true },
                ],
                candidates: [
                  {
                    channel_id: 1,
                    channel_name: 'Alpha',
                    priority: 100,
                    weight: 10,
                    available: true,
                  },
                ],
              },
            ],
            total: 1,
            page: 1,
            page_size: 12,
          },
    },
  }))
  const { onDirtyChange, client } = mountWorkspace()
  const priority = await screen.findByRole('spinbutton', {
    name: 'Priority for Alpha',
  })
  expect(
    screen.queryByText(
      'Save or reset edited cards before changing filters or views.'
    )
  ).not.toBeInTheDocument()
  fireEvent.change(priority, { target: { value: '7' } })
  await waitFor(() => expect(onDirtyChange).toHaveBeenLastCalledWith(true))
  expect(
    screen.getByText(
      'Save or reset edited cards before changing filters or views.'
    )
  ).toBeVisible()
  expect(screen.getByRole('combobox', { name: 'Search models' })).toBeDisabled()
  await userEvent.click(screen.getByRole('button', { name: 'Fullscreen' }))
  expect(screen.getByRole('spinbutton', { name: 'Priority for Alpha' })).toBe(
    priority
  )
  expect(priority).toHaveValue(7)
  expect(screen.getByRole('combobox', { name: 'Search models' })).toBeDisabled()
  await userEvent.click(screen.getByRole('button', { name: 'Exit fullscreen' }))
  expect(screen.getByRole('spinbutton', { name: 'Priority for Alpha' })).toBe(
    priority
  )
  expect(priority).toHaveValue(7)
  await act(async () => {
    await client.invalidateQueries({ queryKey: ['model-routing-pools'] })
  })
  expect(priority).toHaveValue(7)
  await userEvent.click(screen.getByRole('button', { name: 'Reset' }))
  await waitFor(() =>
    expect(
      screen.getByRole('combobox', { name: 'Search models' })
    ).toBeEnabled()
  )
  expect(priority).toHaveValue(100)
  expect(
    screen.queryByText(
      'Save or reset edited cards before changing filters or views.'
    )
  ).not.toBeInTheDocument()
})

test('fullscreen contains the filters, routing content and pagination and restores focus on Escape', async () => {
  render(<div role='listbox' hidden />)
  useAuthStore
    .getState()
    .auth.setUser({ id: 1, username: 'root', role: ROLE.SUPER_ADMIN })
  vi.spyOn(api, 'get').mockImplementation(async (url) => ({
    data: {
      success: true,
      data: url.endsWith('/catalog')
        ? { groups: { standard: 'Standard' }, models: ['kimi-k3'] }
        : { items: [], total: 0, page: 1, page_size: 12 },
    },
  }))
  mountWorkspace()
  await screen.findByText('No configured routing pools')
  const fullscreen = screen.getByRole('button', { name: 'Fullscreen' })
  const refresh = screen.getByRole('button', { name: 'Refresh' })
  expect(refresh.nextElementSibling).toBe(fullscreen)
  expect(fullscreen).toHaveAttribute('aria-pressed', 'false')

  await userEvent.click(fullscreen)
  const workspace = screen.getByRole('dialog', { name: 'Model routing' })
  expect(workspace).toHaveAttribute('aria-modal', 'true')
  expect(workspace).toHaveClass('fixed', 'inset-0', 'overflow-hidden')
  expect(
    within(workspace).getByRole('combobox', { name: 'Search models' })
  ).toBeEnabled()
  expect(
    within(workspace).getByText('No configured routing pools')
  ).toBeVisible()
  expect(within(workspace).getByRole('button', { name: 'Next' })).toBeDisabled()
  expect(
    within(workspace).queryByText(
      'Each card overrides channel defaults for one model and public group. Customer-specific routing takes precedence.'
    )
  ).not.toBeInTheDocument()
  expect(fullscreen).toHaveAttribute('aria-pressed', 'true')
  expect(fullscreen).toHaveFocus()

  await userEvent.click(
    within(workspace).getAllByRole('button', {
      name: 'Configure routing pool',
    })[0]
  )
  const createDialog = screen.getByRole('dialog', {
    name: 'Configure routing pool',
  })
  expect(createDialog).toBeVisible()
  await userEvent.keyboard('{Escape}')
  await waitFor(() => expect(createDialog).not.toBeInTheDocument())
  expect(workspace).toBeVisible()

  await userEvent.click(screen.getByRole('button', { name: 'Exit fullscreen' }))
  await userEvent.keyboard('{Enter}')
  expect(screen.getByRole('dialog', { name: 'Model routing' })).toBeVisible()
  await userEvent.keyboard('{Escape}')
  expect(
    screen.queryByRole('dialog', { name: 'Model routing' })
  ).not.toBeInTheDocument()
  expect(fullscreen).toHaveAttribute('aria-pressed', 'false')
  expect(fullscreen).toHaveFocus()
})

test('discount-first filtering requests only pools using the selected strategy', async () => {
  useAuthStore
    .getState()
    .auth.setUser({ id: 1, username: 'root', role: ROLE.SUPER_ADMIN })
  const get = vi.spyOn(api, 'get').mockImplementation(async (url) => ({
    data: {
      success: true,
      data: url.endsWith('/catalog')
        ? { groups: {}, models: [] }
        : { items: [], total: 0, page: 1, page_size: 12 },
    },
  }))
  mountWorkspace()
  await userEvent.click(
    await screen.findByRole('combobox', { name: 'Filter scheduling strategy' })
  )
  await userEvent.click(
    await screen.findByRole('option', { name: 'Discount-first routing' })
  )
  await waitFor(() =>
    expect(get).toHaveBeenCalledWith('/api/channel/routing-pools', {
      params: expect.objectContaining({ mode: 'discount' }),
    })
  )
})

test('an administrator without channel read permission cannot fetch or configure routing pools', async () => {
  useAuthStore
    .getState()
    .auth.setUser({ id: 2, username: 'restricted', role: ROLE.ADMIN })
  const get = vi.spyOn(api, 'get')
  mountWorkspace()
  expect(
    await screen.findByText("You don't have necessary permission")
  ).toBeVisible()
  expect(get).not.toHaveBeenCalled()
  expect(
    screen.queryByRole('button', { name: 'Configure routing pool' })
  ).not.toBeInTheDocument()
})
