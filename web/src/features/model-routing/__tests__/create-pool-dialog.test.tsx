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
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, expect, test, vi } from 'vitest'

import { api } from '@/lib/api'

import { CreatePoolDialog } from '../create-pool-dialog'

const clients: QueryClient[] = []
afterEach(() => clients.forEach((client) => client.clear()))

function mountDialog() {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  })
  clients.push(client)
  const onOpenChange = vi.fn()
  render(
    <QueryClientProvider client={client}>
      <CreatePoolDialog
        open
        onOpenChange={onOpenChange}
        groups={{ standard: 'Standard' }}
        canWrite
      />
    </QueryClientProvider>
  )
  return onOpenChange
}

async function chooseIdentity() {
  await userEvent.click(screen.getByRole('combobox', { name: 'Public group' }))
  await userEvent.click(
    await screen.findByRole('option', { name: 'Standard (standard)' })
  )
  const model = screen.getByRole('combobox', { name: 'Model' })
  await waitFor(() => expect(model).toBeEnabled())
  await userEvent.click(model)
  await userEvent.click(await screen.findByRole('option', { name: 'kimi-k3' }))
}

test('administrators create a model and group pool from available channel defaults with their chosen strategy', async () => {
  vi.spyOn(api, 'get').mockImplementation(async (url) => ({
    data: {
      success: true,
      data: url.endsWith('/catalog')
        ? { groups: { standard: 'Standard' }, models: ['kimi-k3'] }
        : {
            pool: null,
            candidates: [
              {
                channel_id: 1,
                channel_name: 'Alpha',
                priority: 0,
                weight: 0,
                available: true,
              },
              {
                channel_id: 2,
                channel_name: 'Closed',
                priority: 100,
                weight: 10,
                available: false,
              },
            ],
          },
    },
  }))
  const put = vi.spyOn(api, 'put').mockImplementation(async (_url, data) => ({
    data: {
      success: true,
      data: { ...(data as object), revision: 'created' },
    },
  }))
  const close = mountDialog()
  expect(screen.getByRole('button', { name: 'Create' })).toBeDisabled()
  await chooseIdentity()
  await userEvent.click(
    screen.getByRole('combobox', { name: 'Scheduling strategy' })
  )
  await userEvent.click(
    await screen.findByRole('option', { name: 'Smart routing' })
  )
  await waitFor(() =>
    expect(screen.getByRole('button', { name: 'Create' })).toBeEnabled()
  )
  await userEvent.click(screen.getByRole('button', { name: 'Create' }))
  await waitFor(() =>
    expect(put).toHaveBeenCalledWith('/api/channel/routing-pool', {
      routing_group: 'standard',
      model: 'kimi-k3',
      mode: 'smart',
      revision: '',
      channels: [{ channel_id: 1, priority: 0, weight: 0, enabled: true }],
    })
  )
  await waitFor(() => expect(close).toHaveBeenCalledWith(false))
})

test('choosing an existing pool keeps creation disabled and points administrators to its existing configuration', async () => {
  vi.spyOn(api, 'get').mockImplementation(async (url) => ({
    data: {
      success: true,
      data: url.endsWith('/catalog')
        ? { groups: { standard: 'Standard' }, models: ['kimi-k3'] }
        : {
            pool: {
              routing_group: 'standard',
              model: 'kimi-k3',
              mode: 'priority',
              revision: 'r1',
              channels: [],
            },
            candidates: [],
          },
    },
  }))
  const put = vi.spyOn(api, 'put')
  mountDialog()
  await chooseIdentity()
  expect(
    await screen.findByText('This routing pool is already configured.')
  ).toBeVisible()
  expect(screen.getByRole('button', { name: 'Create' })).toBeDisabled()
  expect(put).not.toHaveBeenCalled()
})
