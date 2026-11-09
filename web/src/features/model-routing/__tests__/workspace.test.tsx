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
import type { ReactNode } from 'react'
import { afterEach, expect, test, vi } from 'vitest'

import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { api } from '@/lib/api'
import { ROLE } from '@/lib/roles'
import { useAuthStore } from '@/stores/auth-store'

import { ModelRoutingWorkspace } from '..'
import type { RoutingPoolView } from '../api'
import { PoolCard } from '../pool-card'
import { RoutingWorkspaceDisplay } from '../routing-workspace-display'
import { RoutingScopeSelect } from '../scope-select'
import { getSettingsField } from './settings-queries'

const clients: QueryClient[] = []
afterEach(() => {
  clients.forEach((client) => client.clear())
  useAuthStore.getState().auth.setUser(null)
})

function mountWorkspace(
  options: { allUsers?: boolean; scopeSelect?: ReactNode } = {}
) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  })
  clients.push(client)
  const onDirtyChange = vi.fn()
  const route = createRootRoute({
    component: () => (
      <QueryClientProvider client={client}>
        <ModelRoutingWorkspace {...options} onDirtyChange={onDirtyChange} />
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
  await userEvent.click(
    await screen.findByRole('button', { name: 'Settings: Alpha' })
  )
  const priority = await getSettingsField('spinbutton', {
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
  expect(getSettingsField('spinbutton', { name: 'Priority for Alpha' })).toBe(
    priority
  )
  expect(priority).toHaveValue(7)
  expect(screen.getByRole('combobox', { name: 'Search models' })).toBeDisabled()
  await userEvent.click(screen.getByRole('button', { name: 'Exit fullscreen' }))
  expect(getSettingsField('spinbutton', { name: 'Priority for Alpha' })).toBe(
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

test('Escape closes only the nested pool monitor and preserves the outer fullscreen workspace, focus and editor draft', async () => {
  useAuthStore
    .getState()
    .auth.setUser({ id: 1, username: 'root', role: ROLE.SUPER_ADMIN })
  const pool: RoutingPoolView = {
    routing_group: 'standard',
    model: 'kimi-k3',
    mode: 'priority',
    revision: 'r1',
    channels: [{ channel_id: 1, priority: 100, weight: 10, enabled: true }],
    candidates: [
      {
        channel_id: 1,
        channel_name: 'Alpha',
        priority: 100,
        weight: 10,
        available: true,
      },
    ],
  }
  vi.spyOn(api, 'get').mockImplementation(async (url) => {
    if (url === '/api/channel/monitoring') {
      return { data: { success: true, data: [] } }
    }
    if (url === '/api/channel/routing-pool') {
      return {
        data: {
          success: true,
          data: {
            pool,
            candidates: pool.candidates,
            effective_mode: 'priority',
          },
        },
      }
    }
    if (url === '/api/channel/routing-monitor') {
      return {
        data: {
          success: true,
          data: {
            generated_at: Date.now(),
            legacy_enabled: false,
            coverage: 'local',
            scope: 'base',
            partial: false,
            nodes: [
              {
                id: 'local',
                name: 'gateway-01',
                local: true,
                updated_at: Date.now(),
                routes: [],
              },
            ],
          },
        },
      }
    }
    throw new Error(`Unexpected nested monitor request: ${url}`)
  })
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  clients.push(client)
  const overflow = document.body.style.overflow
  render(
    <QueryClientProvider client={client}>
      <RoutingWorkspaceDisplay
        toolbar={<span>Routing pools</span>}
        refreshAction={null}
      >
        <div
          role='region'
          aria-label='Routing pool list'
          className='overflow-auto'
        >
          <PoolCard pool={pool} canWrite onDirtyChange={() => undefined} />
        </div>
      </RoutingWorkspaceDisplay>
    </QueryClientProvider>
  )
  const fullscreen = screen.getByRole('button', { name: 'Fullscreen' })
  await userEvent.click(fullscreen)
  const outer = screen.getByRole('dialog', { name: 'Model routing' })
  await userEvent.click(
    within(outer).getByRole('button', { name: 'Settings: Alpha' })
  )
  const priority = getSettingsField('spinbutton', {
    name: 'Priority for Alpha',
  })
  fireEvent.change(priority, { target: { value: '7' } })
  const trigger = within(outer).getByRole('button', { name: 'Monitoring view' })
  const poolList = within(outer).getByRole('region', {
    name: 'Routing pool list',
  })
  poolList.scrollTop = 120
  await userEvent.click(trigger)
  const inner = screen.getByRole('dialog', {
    name: 'kimi-k3 · standard · Default configuration',
  })
  expect(getSettingsField('spinbutton', { name: 'Priority for Alpha' })).toBe(
    priority
  )
  expect(outer.parentElement?.inert).toBe(true)
  await userEvent.keyboard('{Escape}')
  expect(inner).toHaveAttribute('role', 'dialog')
  poolList.scrollTop = 0 // Browsers can clamp the list after its card moves into the portal.
  await userEvent.keyboard('{Escape}')
  expect(inner).not.toHaveAttribute('role', 'dialog')
  expect(
    screen.queryByRole('dialog', {
      name: 'kimi-k3 · standard · Default configuration',
    })
  ).not.toBeInTheDocument()
  expect(screen.getByRole('dialog', { name: 'Model routing' })).toBe(outer)
  expect(outer.parentElement?.inert).not.toBe(true)
  expect(document.body.style.overflow).toBe('hidden')
  expect(priority).toHaveValue(7)
  expect(poolList.scrollTop).toBe(120)
  expect(trigger).toHaveFocus()
  await userEvent.keyboard('{Escape}')
  expect(screen.queryByRole('dialog')).not.toBeInTheDocument()
  expect(document.body.style.overflow).toBe(overflow)
  expect(fullscreen).toHaveFocus()
  expect(priority).toHaveValue(7)
})

test('unmounting nested fullscreen workspaces restores the page background and original scroll lock', async () => {
  const background = document.createElement('div')
  background.inert = false
  document.body.appendChild(background)
  const overflow = document.body.style.overflow
  const page = render(
    <RoutingWorkspaceDisplay
      label='Outer routing workspace'
      toolbar={null}
      refreshAction={null}
    >
      <RoutingWorkspaceDisplay
        label='Pool monitor'
        toolbar={null}
        refreshAction={null}
      >
        <p>Routing observations</p>
      </RoutingWorkspaceDisplay>
    </RoutingWorkspaceDisplay>
  )
  try {
    await userEvent.click(
      within(
        screen.getByRole('region', { name: 'Outer routing workspace' })
      ).getAllByRole('button', { name: 'Fullscreen' })[0]
    )
    await userEvent.click(
      within(screen.getByRole('region', { name: 'Pool monitor' })).getByRole(
        'button',
        { name: 'Fullscreen' }
      )
    )
    expect(background.inert).toBe(true)
    expect(document.body.style.overflow).toBe('hidden')
    page.unmount()
    expect(background.inert).toBe(false)
    expect(document.body.style.overflow).toBe(overflow)
  } finally {
    page.unmount()
    background.remove()
  }
})

test('Escape closes an open project Select without leaving fullscreen, then a second Escape returns to the workspace', async () => {
  render(
    <RoutingWorkspaceDisplay
      label='Pool monitoring'
      toolbar={null}
      refreshAction={null}
    >
      <Select defaultValue='gateway-01'>
        <SelectTrigger aria-label='Routing node'>
          <SelectValue />
        </SelectTrigger>
        <SelectContent>
          <SelectItem value='gateway-01'>Gateway 01</SelectItem>
          <SelectItem value='gateway-02'>Gateway 02</SelectItem>
        </SelectContent>
      </Select>
    </RoutingWorkspaceDisplay>
  )
  await userEvent.click(screen.getByRole('button', { name: 'Fullscreen' }))
  const dialog = screen.getByRole('dialog', { name: 'Pool monitoring' })
  const trigger = within(dialog).getByRole('combobox', { name: 'Routing node' })
  await userEvent.click(trigger)
  expect(await screen.findByRole('listbox')).toBeVisible()
  await userEvent.keyboard('{Escape}')
  await waitFor(() =>
    expect(screen.queryByRole('listbox')).not.toBeInTheDocument()
  )
  expect(screen.getByRole('dialog', { name: 'Pool monitoring' })).toBe(dialog)
  expect(trigger).toHaveFocus()
  await userEvent.keyboard('{Escape}')
  expect(
    screen.queryByRole('dialog', { name: 'Pool monitoring' })
  ).not.toBeInTheDocument()
})

test('all scope shows default and named user cards and filters by model and channel before editing a user pool', async () => {
  useAuthStore
    .getState()
    .auth.setUser({ id: 1, username: 'root', role: ROLE.SUPER_ADMIN })
  const defaultPool: RoutingPoolView = {
    model: 'kimi-k3',
    routing_group: 'standard',
    mode: 'priority',
    revision: 'default-r1',
    channels: [{ channel_id: 1, priority: 100, weight: 10, enabled: true }],
    candidates: [
      {
        channel_id: 1,
        channel_name: 'Primary',
        priority: 100,
        weight: 10,
        available: true,
      },
    ],
  }
  const userPool = {
    ...defaultPool,
    user_id: 42,
    username: 'acme',
    revision: 'user-r1',
  }
  const get = vi.spyOn(api, 'get').mockImplementation(async (url, config) => {
    if (url === '/api/channel/routing-pools/catalog') {
      return {
        data: {
          success: true,
          data: {
            models: ['kimi-k3'],
            groups: { standard: 'Standard' },
            channels: [
              { id: 1, name: 'Primary' },
              { id: 2, name: 'Backup' },
            ],
          },
        },
      }
    }
    if (url === '/api/channel/routing-pools') {
      const params = config?.params as { channel_id?: number }
      const items = params.channel_id === 2 ? [] : [defaultPool, userPool]
      return {
        data: {
          success: true,
          data: { items, total: items.length, page: 1, page_size: 12 },
        },
      }
    }
    if (url === '/api/user/42/channel-routing-overrides') {
      return {
        data: {
          success: true,
          data: {
            user_id: 42,
            username: 'acme',
            user_role: 1,
            model: 'kimi-k3',
            models: ['kimi-k3'],
            routing_group: 'standard',
            mode: 'priority',
            source: 'user',
            configured: true,
            revision: 'user-r1',
            pool_channels: userPool.channels,
            channels: [
              {
                channel_id: 1,
                channel_name: 'Primary',
                configured: true,
                available: true,
                enabled: true,
                default_priority: 100,
                inherited_priority: 100,
                inherited_weight: 10,
                effective_priority: 100,
                effective_weight: 10,
                priority_override: 100,
                weight_override: 10,
              },
            ],
          },
        },
      }
    }
    throw new Error(`Unexpected GET ${url}`)
  })
  const patch = vi
    .spyOn(api, 'patch')
    .mockResolvedValue({ data: { success: true } })
  const scopeChanged = vi.fn()
  mountWorkspace({
    allUsers: true,
    scopeSelect: (
      <RoutingScopeSelect allUsers disabled={false} onChange={scopeChanged} />
    ),
  })
  expect(await screen.findByText('Default resource pool')).toBeVisible()
  const userCard = await screen.findByRole('region', {
    name: 'acme · kimi-k3 · Standard',
  })
  expect(within(userCard).getByText('acme · #42')).toBeVisible()
  await userEvent.click(screen.getByRole('combobox', { name: 'Search models' }))
  await userEvent.click(await screen.findByRole('option', { name: 'kimi-k3' }))
  await userEvent.click(
    screen.getByRole('combobox', { name: 'Search channels' })
  )
  await userEvent.click(
    await screen.findByRole('option', { name: 'Primary #1' })
  )
  await waitFor(() =>
    expect(get).toHaveBeenCalledWith('/api/channel/routing-pools', {
      params: expect.objectContaining({
        scope: 'all',
        model: 'kimi-k3',
        channel_id: 1,
      }),
    })
  )
  await userEvent.click(
    await within(userCard).findByRole('button', { name: 'Edit user override' })
  )
  await userEvent.click(
    within(userCard).getByRole('button', { name: 'Settings: Primary' })
  )
  fireEvent.change(
    getSettingsField('spinbutton', { name: 'Priority for Primary' }, userCard),
    { target: { value: '50' } }
  )
  expect(
    screen.getByRole('combobox', { name: 'Search channels' })
  ).toBeDisabled()
  expect(
    screen.getByRole('combobox', { name: 'Filter primary channel' })
  ).toBeDisabled()
  await userEvent.click(within(userCard).getByRole('button', { name: 'Save' }))
  await waitFor(() =>
    expect(patch).toHaveBeenCalledWith(
      '/api/user/42/channel-routing-overrides',
      expect.objectContaining({
        model: 'kimi-k3',
        routing_group: 'standard',
        revision: 'user-r1',
        overrides: [
          expect.objectContaining({ channel_id: 1, priority_override: 50 }),
        ],
      })
    )
  )
  await waitFor(() =>
    expect(
      screen.getByRole('combobox', { name: 'Search channels' })
    ).toBeEnabled()
  )
  await userEvent.click(
    screen.getByRole('combobox', { name: 'Search channels' })
  )
  await userEvent.click(
    await screen.findByRole('option', { name: 'Backup #2' })
  )
  expect(await screen.findByText('No configured routing pools')).toBeVisible()
})

test('channel role filtering requires a selected channel and resets when the channel is cleared', async () => {
  useAuthStore
    .getState()
    .auth.setUser({ id: 1, username: 'root', role: ROLE.SUPER_ADMIN })
  const get = vi.spyOn(api, 'get').mockImplementation(async (url) => ({
    data: {
      success: true,
      data: url.endsWith('/catalog')
        ? { models: [], groups: {}, channels: [{ id: 1, name: 'Primary' }] }
        : { items: [], total: 0, page: 1, page_size: 12 },
    },
  }))
  mountWorkspace({ allUsers: true })
  const role = await screen.findByRole('combobox', {
    name: 'Filter primary channel',
  })
  expect(role).toBeDisabled()
  await userEvent.click(
    screen.getByRole('combobox', { name: 'Search channels' })
  )
  await userEvent.click(
    await screen.findByRole('option', { name: 'Primary #1' })
  )
  expect(role).toBeEnabled()
  await userEvent.click(role)
  await userEvent.click(
    await screen.findByRole('option', { name: /^Primary channel$/ })
  )
  await waitFor(() =>
    expect(get).toHaveBeenCalledWith('/api/channel/routing-pools', {
      params: expect.objectContaining({
        scope: 'all',
        channel_id: 1,
        channel_role: 'primary',
        page: 1,
      }),
    })
  )
  await userEvent.click(role)
  await userEvent.click(
    await screen.findByRole('option', { name: 'Non-primary channel' })
  )
  await waitFor(() =>
    expect(get).toHaveBeenCalledWith('/api/channel/routing-pools', {
      params: expect.objectContaining({
        channel_id: 1,
        channel_role: 'non_primary',
      }),
    })
  )
  await userEvent.click(
    screen.getByRole('combobox', { name: 'Search channels' })
  )
  await userEvent.click(
    await screen.findByRole('option', { name: 'All channels' })
  )
  expect(role).toBeDisabled()
  expect(role).toHaveTextContent('All channel roles')
  await waitFor(() =>
    expect(get).toHaveBeenLastCalledWith('/api/channel/routing-pools', {
      params: expect.objectContaining({
        channel_id: undefined,
        channel_role: undefined,
      }),
    })
  )
})
