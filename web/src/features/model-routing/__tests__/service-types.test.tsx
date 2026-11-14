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
import { afterEach, expect, test, vi } from 'vitest'

import { api } from '@/lib/api'
import { useAuthStore } from '@/stores/auth-store'

import type { RoutingServiceType, UserServiceTypeBinding } from '../api'
import { ServiceTypeManager } from '../service-type-manager'
import { UserServiceTypeSettings } from '../user-service-type-settings'

const clients: QueryClient[] = []
const serviceTypes: RoutingServiceType[] = [
  { id: 1, name: 'Economy', revision: 'economy-v1' },
  { id: 2, name: 'Quality', revision: 'quality-v1' },
]
afterEach(() => {
  clients.forEach((client) => client.clear())
  useAuthStore.getState().auth.setUser(null)
})

function mountSettings(
  options: { readOnly?: boolean; model?: string; group?: string } = {}
) {
  useAuthStore.getState().auth.setUser({ id: 1, username: 'root', role: 100 })
  const state = {
    bindings: [
      {
        model: '',
        routing_group: '',
        service_type_id: 1,
        revision: 'account-v1',
      },
    ] as UserServiceTypeBinding[],
  }
  vi.spyOn(api, 'get').mockImplementation(async (url) => ({
    data: {
      success: true,
      data: url.endsWith('/routing-service-types')
        ? { bindings: state.bindings, service_types: serviceTypes }
        : {
            user_id: 42,
            username: 'Acme',
            user_role: 1,
            models: ['kimi-k3'],
            key_groups: { vip: 'VIP' },
          },
    },
  }))
  const put = vi.spyOn(api, 'put').mockImplementation(async (_, data) => {
    const change = data as UserServiceTypeBinding & { reset?: boolean }
    state.bindings = state.bindings.filter(
      (row) =>
        row.model !== change.model || row.routing_group !== change.routing_group
    )
    if (!change.reset) state.bindings.push({ ...change, revision: 'saved-v2' })
    return { data: { success: true } }
  })
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  })
  clients.push(client)
  render(
    <QueryClientProvider client={client}>
      <UserServiceTypeSettings userId={42} {...options} />
    </QueryClientProvider>
  )
  return { put, state }
}

test('changing the account service type saves a reference and shows the saved selection', async () => {
  const { put } = mountSettings()
  expect(api.get).not.toHaveBeenCalled()
  await userEvent.click(screen.getByRole('button', { name: 'Service type' }))
  const selector = await screen.findByRole('combobox', {
    name: 'Account service type',
  })
  expect(selector).toHaveValue('Economy')
  await userEvent.click(selector)
  await userEvent.click(await screen.findByRole('option', { name: 'Quality' }))
  await waitFor(() =>
    expect(put).toHaveBeenCalledWith('/api/user/42/routing-service-types', {
      model: '',
      routing_group: '',
      service_type_id: 2,
      revision: 'account-v1',
      reset: false,
    })
  )
  await waitFor(() => expect(selector).toBeEnabled())
  expect(selector).toHaveValue('Quality')
})

test('a model and group exception can select default pools then restore inheritance', async () => {
  const { put } = mountSettings({ model: 'kimi-k3', group: 'vip' })
  await userEvent.click(screen.getByRole('button', { name: 'Service type' }))
  const selector = await screen.findByRole('combobox', {
    name: 'kimi-k3 · vip',
  })
  await userEvent.click(selector)
  await userEvent.click(
    await screen.findByRole('option', { name: 'Default routing pool' })
  )
  await waitFor(() =>
    expect(put).toHaveBeenLastCalledWith('/api/user/42/routing-service-types', {
      model: 'kimi-k3',
      routing_group: 'vip',
      service_type_id: 0,
      revision: '',
      reset: false,
    })
  )
  await waitFor(() => expect(selector).toBeEnabled())
  await userEvent.click(selector)
  await userEvent.click(
    await screen.findByRole('option', { name: 'Inherit service type' })
  )
  await waitFor(() =>
    expect(put).toHaveBeenLastCalledWith('/api/user/42/routing-service-types', {
      model: 'kimi-k3',
      routing_group: 'vip',
      service_type_id: 0,
      revision: 'saved-v2',
      reset: true,
    })
  )
})

test('view-only users can inspect bindings while assignment controls stay disabled', async () => {
  const { put } = mountSettings({ readOnly: true })
  await userEvent.click(screen.getByRole('button', { name: 'Service type' }))
  expect(
    await screen.findByRole('combobox', { name: 'Account service type' })
  ).toBeDisabled()
  expect(
    screen.queryByRole('combobox', { name: 'Model exception' })
  ).not.toBeInTheDocument()
  expect(put).not.toHaveBeenCalled()
})

test('a failed assignment retains the saved selection and displays an error', async () => {
  const { put } = mountSettings()
  put.mockResolvedValue({
    data: { success: false, message: 'configuration changed' },
  })
  await userEvent.click(screen.getByRole('button', { name: 'Service type' }))
  const selector = await screen.findByRole('combobox', {
    name: 'Account service type',
  })
  await userEvent.click(selector)
  await userEvent.click(await screen.findByRole('option', { name: 'Quality' }))
  expect(await screen.findByRole('alert')).toHaveTextContent(
    'Changes were not saved'
  )
  expect(selector).toHaveValue('Economy')
  expect(selector).toBeEnabled()
})

test('custom service types are validated and can be selected for shared pool editing', async () => {
  const state = { types: [...serviceTypes] }
  vi.spyOn(api, 'get').mockImplementation(async () => ({
    data: { success: true, data: state.types },
  }))
  const put = vi.spyOn(api, 'put').mockImplementation(async (_, data) => {
    const row = {
      ...(data as RoutingServiceType),
      id: 3,
      revision: 'custom-v1',
    }
    state.types = [...state.types, row]
    return { data: { success: true, data: row } }
  })
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  clients.push(client)
  const select = vi.fn()
  render(
    <QueryClientProvider client={client}>
      <ServiceTypeManager
        value={null}
        onSelect={select}
        disabled={false}
        canWrite
      />
    </QueryClientProvider>
  )
  await userEvent.click(
    screen.getByRole('button', { name: 'Routing service types' })
  )
  const dialog = await screen.findByRole('dialog', {
    name: 'Routing service types',
  })
  await userEvent.click(
    within(dialog).getByRole('button', { name: 'Create service type' })
  )
  expect(await within(dialog).findByRole('alert')).toHaveTextContent(
    'Please enter a valid name'
  )
  expect(put).not.toHaveBeenCalled()
  await userEvent.type(
    within(dialog).getByRole('textbox', { name: 'Service type name' }),
    'Dedicated SLA'
  )
  await userEvent.click(
    within(dialog).getByRole('button', { name: 'Create service type' })
  )
  const custom = await within(dialog).findByRole('button', {
    name: 'Dedicated SLA',
  })
  expect(put).toHaveBeenCalledWith('/api/channel/routing-service-types', {
    id: 0,
    revision: '',
    name: 'Dedicated SLA',
  })
  await userEvent.click(custom)
  expect(select).toHaveBeenCalledWith({
    id: 3,
    name: 'Dedicated SLA',
    revision: 'custom-v1',
  })
})
