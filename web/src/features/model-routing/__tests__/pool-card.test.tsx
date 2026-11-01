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
import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, expect, test, vi } from 'vitest'

import { api } from '@/lib/api'

import { routingPoolKeys, type RoutingPoolView } from '../api'
import { PoolCard } from '../pool-card'

const clients: QueryClient[] = []
afterEach(() => clients.forEach((client) => client.clear()))

function fixture(): RoutingPoolView {
  return {
    routing_group: 'standard',
    model: 'kimi-k3',
    mode: 'priority',
    revision: 'original',
    channels: [
      { channel_id: 1, priority: 100, weight: 10, enabled: true },
      { channel_id: 2, priority: 80, weight: 0, enabled: true },
    ],
    candidates: [
      {
        channel_id: 1,
        channel_name: 'Alpha',
        priority: 100,
        weight: 10,
        available: true,
      },
      {
        channel_id: 2,
        channel_name: 'Beta',
        priority: 80,
        weight: 0,
        available: true,
      },
    ],
  }
}

function mountCard(pool = fixture(), canWrite = true) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  })
  clients.push(client)
  const onDirtyChange = vi.fn()
  render(
    <QueryClientProvider client={client}>
      <PoolCard pool={pool} canWrite={canWrite} onDirtyChange={onDirtyChange} />
    </QueryClientProvider>
  )
  return { client, onDirtyChange }
}

test('keyboard reordering persists channel order and priorities while preserving zero weights', async () => {
  const pool = fixture()
  const put = vi.spyOn(api, 'put').mockImplementation(async (_url, body) => ({
    data: { success: true, data: { ...(body as object), revision: 'saved' } },
  }))
  const { onDirtyChange } = mountCard(pool)
  fireEvent.keyDown(screen.getByRole('button', { name: 'Drag 2 to reorder' }), {
    key: 'ArrowUp',
  })
  await waitFor(() => expect(onDirtyChange).toHaveBeenLastCalledWith(true))
  await userEvent.click(screen.getByRole('button', { name: 'Save' }))
  await waitFor(() =>
    expect(put).toHaveBeenCalledWith(
      '/api/channel/routing-pool',
      expect.objectContaining({
        routing_group: 'standard',
        model: 'kimi-k3',
        revision: 'original',
        channels: [
          { channel_id: 2, priority: 2, weight: 0, enabled: true },
          { channel_id: 1, priority: 1, weight: 10, enabled: true },
        ],
      })
    )
  )
})

test('a failed save retains explicit zero priority, disabled state and the dirty draft', async () => {
  vi.spyOn(api, 'put').mockRejectedValue(new Error('conflicting pool revision'))
  const { onDirtyChange } = mountCard()
  fireEvent.change(
    screen.getByRole('spinbutton', { name: 'Priority for Alpha' }),
    { target: { value: '0' } }
  )
  await userEvent.click(
    screen.getByRole('switch', { name: 'Enable Beta in this pool' })
  )
  await userEvent.click(screen.getByRole('button', { name: 'Save' }))
  await waitFor(() =>
    expect(screen.getByRole('button', { name: 'Save' })).toBeEnabled()
  )
  expect(
    screen.getByRole('spinbutton', { name: 'Priority for Alpha' })
  ).toHaveValue(0)
  expect(
    screen.getByRole('switch', { name: 'Enable Beta in this pool' })
  ).not.toBeChecked()
  expect(onDirtyChange).toHaveBeenLastCalledWith(true)
})

test('invalid numeric input prevents saving and explains the allowed value contract', async () => {
  const put = vi.spyOn(api, 'put')
  mountCard()
  fireEvent.change(
    screen.getByRole('spinbutton', { name: 'Weight for Alpha' }),
    { target: { value: '-1' } }
  )
  const form = screen.getByRole('button', { name: 'Save' }).closest('form')
  if (!form) throw new Error('The save action must belong to its form')
  fireEvent.submit(form)
  expect(await screen.findByRole('alert')).toHaveTextContent(
    'Priority and weight must be whole numbers within the allowed range.'
  )
  expect(put).not.toHaveBeenCalled()
})

test('an empty explicit pool stays empty until the administrator adds an available channel', async () => {
  const pool = fixture()
  pool.channels = []
  vi.spyOn(api, 'get').mockResolvedValue({
    data: {
      success: true,
      data: {
        pool,
        candidates: [
          ...pool.candidates,
          {
            channel_id: 3,
            channel_name: 'Closed provider',
            priority: 999,
            weight: 1,
            available: false,
          },
        ],
      },
    },
  })
  mountCard(pool)
  expect(screen.getByRole('status')).toHaveTextContent(
    'This explicit pool has no channels.'
  )
  expect(screen.queryByRole('spinbutton')).not.toBeInTheDocument()
  await userEvent.click(screen.getByRole('button', { name: 'Add channel' }))
  const select = await screen.findByRole('combobox', { name: 'Select channel' })
  await waitFor(() => expect(select).toBeEnabled())
  await userEvent.click(select)
  expect(await screen.findByRole('option', { name: 'Alpha #1' })).toBeVisible()
  expect(
    screen.queryByRole('option', { name: 'Closed provider #3' })
  ).not.toBeInTheDocument()
  await userEvent.click(screen.getByRole('option', { name: 'Alpha #1' }))
  await userEvent.click(screen.getByRole('button', { name: 'Add' }))
  expect(
    screen.getByRole('spinbutton', { name: 'Priority for Alpha' })
  ).toHaveValue(100)
})

test('read-only cards expose configuration while disabling mutation, reordering and candidate fetches', async () => {
  const get = vi.spyOn(api, 'get')
  const { onDirtyChange } = mountCard(fixture(), false)
  expect(
    screen.getByRole('spinbutton', { name: 'Priority for Alpha' })
  ).toBeDisabled()
  expect(
    screen.getByRole('switch', { name: 'Enable Alpha in this pool' })
  ).toHaveAttribute('aria-disabled', 'true')
  expect(screen.getByRole('button', { name: 'Edit strategy' })).toBeDisabled()
  const drag = screen.getByRole('button', { name: 'Drag 2 to reorder' })
  expect(drag).toBeDisabled()
  fireEvent.keyDown(drag, { key: 'ArrowUp' })
  expect(
    screen.getByRole('spinbutton', { name: 'Priority for Alpha' })
  ).toHaveValue(100)
  expect(onDirtyChange).not.toHaveBeenCalledWith(true)
  expect(get).not.toHaveBeenCalled()
})

test('restoring defaults requires confirmation and deletes the explicit pool using its revision', async () => {
  const pool = fixture()
  const remove = vi
    .spyOn(api, 'delete')
    .mockResolvedValue({ data: { success: true } })
  const { client } = mountCard(pool)
  client.setQueryData(routingPoolKeys.all, [])
  await userEvent.click(
    screen.getByRole('button', {
      name: 'Restore channel defaults',
    })
  )
  expect(remove).not.toHaveBeenCalled()
  await userEvent.click(screen.getByRole('button', { name: 'Restore' }))
  await waitFor(() =>
    expect(remove).toHaveBeenCalledWith('/api/channel/routing-pool', {
      data: {
        routing_group: 'standard',
        model: 'kimi-k3',
        revision: 'original',
      },
    })
  )
})
