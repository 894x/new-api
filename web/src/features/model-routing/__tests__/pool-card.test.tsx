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
  act,
  fireEvent,
  render,
  screen,
  waitFor,
  within,
} from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { createInstance } from 'i18next'
import { I18nextProvider, initReactI18next } from 'react-i18next'
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

test('discount-first strategy saves model latency limits and preserves the existing pool members', async () => {
  const put = vi.spyOn(api, 'put').mockImplementation(async (_url, body) => ({
    data: { success: true, data: { ...(body as object), revision: 'saved' } },
  }))
  mountCard()
  await userEvent.click(screen.getByRole('button', { name: 'Edit strategy' }))
  await userEvent.click(
    screen.getByRole('combobox', { name: 'Scheduling strategy' })
  )
  await userEvent.click(
    await screen.findByRole('option', { name: 'Discount-first routing' })
  )
  expect(
    screen.getByRole('spinbutton', { name: 'Maximum first-token latency (ms)' })
  ).toHaveValue(15000)
  fireEvent.change(
    screen.getByRole('spinbutton', {
      name: 'Maximum first-token latency (ms)',
    }),
    { target: { value: '5000' } }
  )
  fireEvent.change(
    screen.getByRole('spinbutton', {
      name: 'Maximum time per output token (ms)',
    }),
    { target: { value: '100' } }
  )
  await userEvent.click(screen.getByRole('button', { name: 'Done' }))
  await userEvent.click(screen.getByRole('button', { name: 'Save' }))
  await waitFor(() =>
    expect(put).toHaveBeenCalledWith(
      '/api/channel/routing-pool',
      expect.objectContaining({
        mode: 'discount',
        max_ttft_ms: 5000,
        max_tpot_ms: 100,
        channels: fixture().channels,
      })
    )
  )
})

test('invalid discount latency limits prevent persistence and explain the validation error', async () => {
  const put = vi.spyOn(api, 'put')
  const pool = fixture()
  pool.mode = 'discount'
  mountCard(pool)
  await userEvent.click(screen.getByRole('button', { name: 'Edit strategy' }))
  fireEvent.change(
    screen.getByRole('spinbutton', {
      name: 'Maximum time per output token (ms)',
    }),
    { target: { value: '-1' } }
  )
  await userEvent.click(screen.getByRole('button', { name: 'Done' }))
  await userEvent.click(screen.getByRole('button', { name: 'Save' }))
  expect(put).not.toHaveBeenCalled()
  expect(await screen.findByRole('alert')).toHaveTextContent(
    'Enter valid positive latency limits in milliseconds.'
  )
  await waitFor(() =>
    expect(screen.getByRole('button', { name: 'Save' })).toBeEnabled()
  )
})

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
  vi.spyOn(api, 'get').mockImplementation(async (url) => {
    if (url === '/api/channel/monitoring') {
      return { data: { success: true, data: [] } }
    }
    return {
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
    }
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
  expect(get).toHaveBeenCalledTimes(1)
  expect(get).toHaveBeenCalledWith(
    '/api/channel/monitoring',
    expect.objectContaining({
      params: { model: 'kimi-k3', channel_ids: '1,2' },
    })
  )
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

test('removing a channel requires confirmation and changes only the draft until saving', async () => {
  const put = vi.spyOn(api, 'put').mockImplementation(async (_url, body) => ({
    data: { success: true, data: { ...(body as object), revision: 'saved' } },
  }))
  const deleteChannel = vi.spyOn(api, 'delete')
  mountCard()
  await userEvent.click(screen.getByRole('button', { name: 'Remove 1' }))
  const confirmation = await screen.findByRole('alertdialog', {
    name: 'Remove channel from pool',
  })
  expect(
    within(confirmation).getByText(
      'Remove channel "Alpha" from this routing pool? Save the card to apply this change.'
    )
  ).toBeVisible()
  expect(
    screen.getByRole('spinbutton', { name: 'Priority for Alpha', hidden: true })
  ).toHaveValue(100)
  await userEvent.click(
    within(confirmation).getByRole('button', { name: 'Cancel' })
  )
  await waitFor(() =>
    expect(screen.queryByRole('alertdialog')).not.toBeInTheDocument()
  )
  expect(
    screen.getByRole('spinbutton', { name: 'Priority for Alpha' })
  ).toHaveValue(100)
  expect(screen.getByRole('button', { name: 'Save' })).toBeDisabled()
  expect(put).not.toHaveBeenCalled()

  await userEvent.click(screen.getByRole('button', { name: 'Remove 1' }))
  await userEvent.click(
    within(await screen.findByRole('alertdialog')).getByRole('button', {
      name: 'Remove',
    })
  )
  await waitFor(() =>
    expect(screen.queryByRole('alertdialog')).not.toBeInTheDocument()
  )
  expect(
    screen.queryByRole('spinbutton', { name: 'Priority for Alpha' })
  ).not.toBeInTheDocument()
  expect(
    screen.getByRole('spinbutton', { name: 'Weight for Beta' })
  ).toHaveValue(0)
  expect(screen.getByRole('button', { name: 'Save' })).toBeEnabled()
  expect(put).not.toHaveBeenCalled()
  expect(deleteChannel).not.toHaveBeenCalled()
  await userEvent.click(screen.getByRole('button', { name: 'Save' }))
  await waitFor(() =>
    expect(put).toHaveBeenCalledWith(
      '/api/channel/routing-pool',
      expect.objectContaining({
        channels: [{ channel_id: 2, priority: 80, weight: 0, enabled: true }],
      })
    )
  )
})

test('channel names expose their full text in a tooltip', async () => {
  const pool = fixture()
  const name = 'Primary provider for the international customer routing pool'
  pool.candidates[0].channel_name = name
  mountCard(pool)
  await userEvent.hover(screen.getByText(name))
  await waitFor(() => {
    const labels = screen.getAllByText(name)
    expect(labels).toHaveLength(2)
    expect(labels[1]).toBeVisible()
  })
})

test('configured discounts preserve zero and small fractions while unknown discounts stay hidden', () => {
  const pool = fixture()
  pool.candidates[0].effective_cost_discount = 0.003125
  pool.candidates[1].effective_cost_discount = 0
  mountCard(pool)
  expect(
    screen.getByLabelText('Upstream cost discount: 0.003125×').textContent
  ).toBe('0.3125%')
  expect(screen.getByLabelText('Upstream cost discount: 0×').textContent).toBe(
    '0%'
  )
  const discount = screen.getByLabelText('Upstream cost discount: 0.003125×')
  expect(
    screen.getByText('Alpha').compareDocumentPosition(discount) &
      Node.DOCUMENT_POSITION_FOLLOWING
  ).toBeTruthy()
  expect(
    discount.compareDocumentPosition(
      screen.getByRole('spinbutton', { name: 'Priority for Alpha' })
    ) & Node.DOCUMENT_POSITION_FOLLOWING
  ).toBeTruthy()
})

test('unset and invalid discounts are omitted instead of displaying a default discount', () => {
  const pool = fixture()
  pool.candidates[0].effective_cost_discount = null
  pool.candidates[1].effective_cost_discount = -0.1
  mountCard(pool)
  expect(screen.queryByTitle('Upstream cost discount')).not.toBeInTheDocument()
})

test.each([
  ['en', '0.07', '7%'],
  ['zhCN', '0.07', '7%'],
  ['zhTW', '0.07', '7%'],
  ['fr', '0,07', '7\u00a0%'],
  ['ru', '0,07', '7\u00a0%'],
  ['ja', '0.07', '7%'],
  ['vi', '0,07', '7%'],
  ['invalid_locale', '0.07', '7%'],
])(
  'switching to %s updates discount numbers without floating-point artifacts',
  async (language, multiplier, percent) => {
    const instance = createInstance()
    const translation = { 'Upstream cost discount': 'Upstream cost discount' }
    await instance.use(initReactI18next).init({
      lng: 'en',
      fallbackLng: false,
      resources: { en: { translation }, [language]: { translation } },
    })
    const pool = fixture()
    pool.candidates[0].effective_cost_discount = 0.07
    const client = new QueryClient({
      defaultOptions: { queries: { retry: false } },
    })
    clients.push(client)
    render(
      <I18nextProvider i18n={instance}>
        <QueryClientProvider client={client}>
          <PoolCard
            pool={pool}
            canWrite={false}
            onDirtyChange={() => undefined}
          />
        </QueryClientProvider>
      </I18nextProvider>
    )
    await act(async () => {
      await instance.changeLanguage(language)
    })
    expect(
      screen.getByLabelText(`Upstream cost discount: ${multiplier}×`)
        .textContent
    ).toBe(percent)
  }
)

test('channel cache metrics remain visible when probe monitoring is disabled and measured zero stays distinct from missing data', async () => {
  vi.spyOn(api, 'get').mockResolvedValue({
    data: {
      success: true,
      data: [
        {
          channel_id: 1,
          state: 'disabled',
          buckets: [],
          cache: { hit_rate: 0, truncated: false },
        },
        {
          channel_id: 2,
          state: 'disabled',
          buckets: [],
          cache: { hit_rate: null, truncated: false },
        },
      ],
    },
  })
  mountCard()
  expect(await screen.findByLabelText('Cache hit rate: 0%')).toBeVisible()
  expect(screen.getByLabelText('Cache hit rate: —')).toBeVisible()
  expect(screen.getAllByText('Monitoring disabled')).toHaveLength(2)
})

test('cache metrics display only numbers with partial-data details in the tooltip and retain discounts on monitoring failure', async () => {
  const get = vi.spyOn(api, 'get').mockResolvedValue({
    data: {
      success: true,
      data: [
        {
          channel_id: 1,
          state: 'disabled',
          buckets: [],
          cache: { hit_rate: 75.25, truncated: true },
        },
      ],
    },
  })
  const pool = fixture()
  pool.candidates[0].effective_cost_discount = 0.3
  const { client } = mountCard(pool)
  const metric = await screen.findByLabelText('Cache hit rate: 75.25%')
  expect(metric.textContent).toBe('75.25%')
  expect(metric).toHaveAttribute(
    'title',
    expect.stringContaining('Cache hit rate')
  )
  expect(metric).toHaveAttribute(
    'title',
    expect.stringContaining('Partial cache data')
  )
  get.mockRejectedValue(new Error('monitoring unavailable'))
  await client.invalidateQueries({ queryKey: ['channel-monitoring'] })
  await waitFor(() =>
    expect(
      screen.queryByLabelText('Cache hit rate: 75.25%')
    ).not.toBeInTheDocument()
  )
  expect(screen.getByLabelText('Upstream cost discount: 0.3×')).toBeVisible()
  expect(screen.getAllByLabelText('Cache hit rate: —')).toHaveLength(2)
})
