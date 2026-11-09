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
import { render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { useState, type ReactNode } from 'react'
import { afterEach, expect, test, vi } from 'vitest'

import { PoolMonitorDisplay } from '@/features/model-routing/pool-monitor-display'
import { api } from '@/lib/api'
import { ROLE } from '@/lib/roles'
import { useAuthStore, type AuthUser } from '@/stores/auth-store'

import { MonitorBoard } from '../monitor-board'
import {
  MonitorLayoutPicker,
  type MonitorLayout,
} from '../monitor-layout-picker'

const clients: QueryClient[] = []
afterEach(() => {
  clients.forEach((client) => client.clear())
  clients.length = 0
  useAuthStore.getState().auth.setUser(null)
})

function mount(children: ReactNode, actor?: AuthUser) {
  useAuthStore
    .getState()
    .auth.setUser(actor ?? { id: 1, username: 'root', role: ROLE.SUPER_ADMIN })
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false, gcTime: Infinity } },
  })
  clients.push(client)
  return render(
    <QueryClientProvider client={client}>{children}</QueryClientProvider>
  )
}

function mockApi(failCatalog = false) {
  return vi.spyOn(api, 'get').mockImplementation(async (url, config) => {
    let data: unknown
    if (url.includes('/search')) {
      data = { items: [{ id: 7, username: 'customer', role: ROLE.USER }] }
    } else if (url.endsWith('/routing-monitor')) {
      data = {
        generated_at: Date.now(),
        scope: 'base',
        coverage: 'cluster',
        nodes: [],
        partial: false,
        legacy_enabled: false,
      }
    } else if (url.endsWith('/channel-routing-overrides')) {
      data = {
        key_groups: { vip: 'VIP' },
        models: ['customer-model'],
        model_groups: { 'customer-model': ['vip'] },
        channels: [],
        source: 'user',
        mode: 'priority',
      }
    } else if (url.endsWith('/catalog')) {
      if (failCatalog) throw new Error('catalog unavailable')
      const group = config?.params?.routing_group
      data = {
        groups: { standard: 'Standard', vip: 'VIP' },
        models: group === 'vip' ? ['vip-model'] : ['base-model'],
      }
    } else {
      data = { pool: { mode: 'priority' }, candidates: [] }
    }
    return { data: { success: true, data } }
  })
}

function Board() {
  const [layout, setLayout] = useState<MonitorLayout>({ rows: 1, columns: 2 })
  return (
    <>
      <MonitorLayoutPicker value={layout} onChange={setLayout} />
      <MonitorBoard layout={layout} />
    </>
  )
}

async function choosePool(panel: HTMLElement, group: string, model: string) {
  await userEvent.click(
    within(panel).getByRole('combobox', { name: 'Select model and group' })
  )
  await userEvent.click(await screen.findByRole('button', { name: group }))
  await userEvent.click(await screen.findByRole('option', { name: model }))
  await waitFor(() =>
    expect(
      within(panel).getByRole('button', { name: 'Start monitoring' })
    ).toBeEnabled()
  )
  await userEvent.click(
    within(panel).getByRole('button', { name: 'Start monitoring' })
  )
  await within(panel).findByRole('button', { name: 'Change monitoring pool' })
}

test('hovering a layout previews its dimensions and clicking applies that grid of empty panels', async () => {
  const get = mockApi()
  mount(<Board />)
  expect(
    screen.getAllByRole('button', { name: 'Start monitoring' })
  ).toHaveLength(2)
  expect(get.mock.calls.some(([url]) => url.endsWith('/routing-monitor'))).toBe(
    false
  )
  await userEvent.click(
    screen.getByRole('button', { name: 'Multi-panel mode' })
  )
  const choice = screen.getByRole('button', { name: '2 rows × 3 columns' })
  await userEvent.hover(choice)
  expect(screen.getByRole('status')).toHaveTextContent('2 rows × 3 columns')
  const cells = within(
    screen.getByRole('group', { name: 'Monitor layout' })
  ).getAllByRole('button')
  for (const index of [0, 1, 2, 4, 5, 6]) {
    expect(cells[index]).toHaveClass('bg-primary/20', 'dark:bg-primary/25')
  }
  for (const index of [3, 7, 8, 9, 10, 11, 12, 13, 14, 15]) {
    expect(cells[index]).not.toHaveClass('bg-primary/20', 'dark:bg-primary/25')
  }
  expect(
    screen.getAllByRole('button', { name: 'Start monitoring' })
  ).toHaveLength(2)
  await userEvent.click(choice)
  expect(
    screen.getAllByRole('button', { name: 'Start monitoring' })
  ).toHaveLength(6)
  expect(
    screen.getByRole('button', { name: 'Multi-panel mode' })
  ).toHaveAttribute('aria-expanded', 'false')
})

test('keyboard focus previews a layout and Enter confirms it', async () => {
  mockApi()
  mount(<Board />)
  await userEvent.click(
    screen.getByRole('button', { name: 'Multi-panel mode' })
  )
  const choice = screen.getByRole('button', { name: '1 rows × 1 columns' })
  choice.focus()
  await waitFor(() =>
    expect(screen.getByRole('status')).toHaveTextContent('1 rows × 1 columns')
  )
  await userEvent.keyboard('{Enter}')
  expect(
    screen.getAllByRole('button', { name: 'Start monitoring' })
  ).toHaveLength(1)
})

test('a disabled layout picker cannot open or change the monitoring layout', async () => {
  const onChange = vi.fn()
  mount(<MonitorLayoutPicker disabled onChange={onChange} />)
  const trigger = screen.getByRole('button', { name: 'Multi-panel mode' })
  expect(trigger).toBeDisabled()
  await userEvent.click(trigger)
  expect(trigger).toHaveAttribute('aria-expanded', 'false')
  expect(onChange).not.toHaveBeenCalled()
})

test('a four-row layout keeps all sixteen pool selection forms compact without visible scrollbars', () => {
  mockApi()
  mount(<MonitorBoard layout={{ rows: 4, columns: 4 }} />)
  const board = screen.getByRole('region', { name: 'Multi-panel monitoring' })
  expect(board).toHaveClass(
    'md:grid-rows-[repeat(var(--monitor-rows),minmax(0,1fr))]',
    '[scrollbar-width:none]'
  )
  expect(
    within(board).getAllByRole('button', { name: 'Start monitoring' })
  ).toHaveLength(16)
  expect(
    within(board).getAllByRole('combobox', { name: 'Select routing user' })
  ).toHaveLength(16)
  expect(
    within(board).getAllByRole('combobox', { name: 'Select model and group' })
  ).toHaveLength(16)
  expect(
    within(board).queryByText('Choose a user, group and model for this panel.')
  ).not.toBeInTheDocument()
})

test('panels independently monitor a default pool and a selected user pool, preserving targets after resizing', async () => {
  const get = mockApi()
  mount(<Board />)
  const first = screen.getByRole('region', { name: 'Monitor panel 1' })
  const second = screen.getByRole('region', { name: 'Monitor panel 2' })
  await choosePool(first, 'Standard', 'base-model')
  const header = first.querySelector('header')
  expect(header).toContainElement(
    await within(first).findByRole('combobox', { name: 'Monitor node' })
  )
  expect(header).toContainElement(
    within(first).getByRole('button', { name: 'Refresh' })
  )
  expect(header).toHaveClass('flex')
  expect(header).not.toHaveClass('flex-wrap')
  expect(
    within(second).getByRole('button', { name: 'Start monitoring' })
  ).toBeDisabled()
  await userEvent.click(
    within(second).getByRole('combobox', { name: 'Select routing user' })
  )
  expect(screen.queryByRole('option', { name: 'All' })).not.toBeInTheDocument()
  await userEvent.click(
    await screen.findByRole('option', { name: 'customer · #7' })
  )
  await choosePool(second, 'VIP', 'customer-model')
  await waitFor(() =>
    expect(get).toHaveBeenCalledWith(
      '/api/user/7/routing-monitor',
      expect.anything()
    )
  )
  expect(get).toHaveBeenCalledWith(
    '/api/channel/routing-pool',
    expect.objectContaining({
      params: { routing_group: 'standard', model: 'base-model' },
    })
  )
  expect(get).toHaveBeenCalledWith(
    '/api/user/7/channel-routing-overrides',
    expect.objectContaining({
      params: { routing_group: 'vip', model: 'customer-model', view: 'pool' },
    })
  )
  await userEvent.click(
    screen.getByRole('button', { name: 'Multi-panel mode' })
  )
  await userEvent.click(
    screen.getByRole('button', { name: '1 rows × 1 columns' })
  )
  expect(
    screen.queryByRole('region', { name: 'Monitor panel 2' })
  ).not.toBeInTheDocument()
  await userEvent.click(
    screen.getByRole('button', { name: 'Multi-panel mode' })
  )
  await userEvent.click(
    screen.getByRole('button', { name: '1 rows × 2 columns' })
  )
  const restored = screen.getByRole('region', { name: 'Monitor panel 2' })
  expect(within(restored).getByText('customer-model · vip')).toBeVisible()
  await userEvent.click(
    within(restored).getByRole('button', { name: 'Change monitoring pool' })
  )
  expect(
    within(restored).getByRole('button', { name: 'Start monitoring' })
  ).toBeDisabled()
  expect(within(first).getByText('base-model · standard')).toBeVisible()
})

test('a failed catalog leaves monitoring disabled and Retry restores pool selection', async () => {
  const get = mockApi(true)
  mount(<Board />)
  const panel = screen.getByRole('region', { name: 'Monitor panel 1' })
  await within(panel).findByText('Failed to load routing pools.')
  expect(
    within(panel).getByRole('button', { name: 'Start monitoring' })
  ).toBeDisabled()
  get.mockRestore()
  mockApi()
  await userEvent.click(within(panel).getByRole('button', { name: 'Retry' }))
  await waitFor(() =>
    expect(
      within(panel).queryByText('Failed to load routing pools.')
    ).not.toBeInTheDocument()
  )
  await choosePool(panel, 'Standard', 'base-model')
})

test('a user-scoped operator can select that user pool without fetching channel defaults', async () => {
  const get = mockApi()
  mount(<MonitorBoard layout={{ rows: 1, columns: 1 }} userId={7} />, {
    id: 2,
    username: 'operator',
    role: ROLE.ADMIN,
    permissions: { admin_permissions: { user_group_config: { read: true } } },
  })
  const panel = screen.getByRole('region', { name: 'Monitor panel 1' })
  expect(
    within(panel).getByRole('combobox', { name: 'Select routing user' })
  ).toHaveValue('User #7')
  await choosePool(panel, 'VIP', 'customer-model')
  expect(get.mock.calls.some(([url]) => url.startsWith('/api/channel/'))).toBe(
    false
  )
})

test('the fullscreen pool monitor enters multi-panel mode and returns with its editor draft intact', async () => {
  mockApi()
  mount(
    <PoolMonitorDisplay
      open
      onOpenChange={() => {}}
      model='base-model'
      group='standard'
    >
      <input aria-label='Editor draft' defaultValue='unsaved' />
    </PoolMonitorDisplay>
  )
  await userEvent.click(
    screen.getByRole('button', { name: 'Multi-panel mode' })
  )
  await userEvent.click(
    screen.getByRole('button', { name: '2 rows × 2 columns' })
  )
  await screen.findByRole('region', { name: 'Multi-panel monitoring' })
  expect(
    screen.getAllByRole('button', { name: 'Start monitoring' })
  ).toHaveLength(4)
  expect(
    screen.queryByRole('textbox', { name: 'Editor draft' })
  ).not.toBeInTheDocument()
  await userEvent.click(
    screen.getByRole('button', { name: 'Single-panel mode' })
  )
  expect(screen.getByRole('textbox', { name: 'Editor draft' })).toHaveValue(
    'unsaved'
  )
})
