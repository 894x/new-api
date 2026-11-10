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

import { buildDynamicRoutingFormValues } from '@/features/system-settings/models/lib/dynamic-routing-schema'
import { DYNAMIC_ROUTING_FLAT_DEFAULTS } from '@/features/system-settings/types'
import { api } from '@/lib/api'
import { useAuthStore } from '@/stores/auth-store'

import { routingPoolKeys, type RoutingPoolView } from '../api'
import { PoolCard } from '../pool-card'
import { getSettingsField } from './settings-queries'

const clients: QueryClient[] = []
afterEach(() => {
  clients.forEach((client) => client.clear())
  useAuthStore.getState().auth.setUser(null)
})

function fixture(): RoutingPoolView {
  return {
    routing_group: 'standard',
    model: 'kimi-k3',
    mode: 'priority',
    revision: 'original',
    tuning_defaults: buildDynamicRoutingFormValues(
      DYNAMIC_ROUTING_FLAT_DEFAULTS
    ).dynamic_routing_setting,
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

test('channel enable switches stay visible with settings closed and save only the selected toggle', async () => {
  const pool = fixture()
  const put = vi.spyOn(api, 'put').mockImplementation(async (_url, body) => ({
    data: { success: true, data: { ...(body as object), revision: 'saved' } },
  }))
  mountCard(pool)
  expect(
    screen.getByRole('switch', { name: 'Enable Alpha in this pool' })
  ).toBeVisible()
  const beta = screen.getByRole('switch', { name: 'Enable Beta in this pool' })
  expect(beta).toBeChecked()
  await userEvent.click(beta)
  expect(beta).not.toBeChecked()
  expect(beta.closest('[data-order-item]')).toHaveClass('bg-warning/5')
  expect(
    within(beta.closest('[data-order-item]') as HTMLElement).getByText(
      'Disabled'
    )
  ).toHaveClass('text-warning')
  const alpha = screen.getByRole('switch', {
    name: 'Enable Alpha in this pool',
  })
  expect(alpha.closest('[data-order-item]')).not.toHaveClass('bg-warning/5')
  await userEvent.click(beta)
  expect(beta).toBeChecked()
  expect(beta.closest('[data-order-item]')).not.toHaveClass('bg-warning/5')
  expect(
    within(beta.closest('[data-order-item]') as HTMLElement).queryByText(
      'Disabled'
    )
  ).not.toBeInTheDocument()
  await userEvent.click(beta)
  expect(beta).not.toBeChecked()
  expect(
    screen.getByRole('button', { name: 'Settings: Beta' })
  ).toHaveAttribute('aria-expanded', 'false')
  expect(screen.queryByRole('dialog')).not.toBeInTheDocument()
  await userEvent.click(screen.getByRole('button', { name: 'Save' }))
  await waitFor(() =>
    expect(put).toHaveBeenCalledWith(
      '/api/channel/routing-pool',
      expect.objectContaining({
        channels: [pool.channels[0], { ...pool.channels[1], enabled: false }],
      })
    )
  )
})

test('an unavailable channel uses error emphasis while its pool switch remains enabled', () => {
  const pool = fixture()
  pool.candidates[0].available = false
  mountCard(pool)
  const channel = screen
    .getByRole('switch', { name: 'Enable Alpha in this pool' })
    .closest('[data-order-item]')
  expect(channel).toHaveClass('bg-destructive/5')
  expect(within(channel as HTMLElement).getByText('Unavailable')).toHaveClass(
    'text-destructive'
  )
})

test('equal-priority channels share a group and header weights save without changing their priorities', async () => {
  const pool = fixture()
  pool.channels[1].priority = 100
  const put = vi.spyOn(api, 'put').mockImplementation(async (_url, body) => ({
    data: { success: true, data: { ...(body as object), revision: 'saved' } },
  }))
  mountCard(pool)
  const group = screen.getByRole('group', { name: 'Priority 100' })
  expect(within(group).getByText('Alpha')).toBeVisible()
  expect(within(group).getByText('Beta')).toBeVisible()
  expect(screen.getAllByRole('group', { name: /^Priority / })).toHaveLength(1)
  expect(screen.getByRole('button', { name: 'Save' })).toBeDisabled()
  const weight = within(group).getByRole('spinbutton', {
    name: 'Weight for Beta',
  })
  fireEvent.change(weight, { target: { value: '30' } })
  expect(
    screen.getByRole('button', { name: 'Settings: Beta' })
  ).toHaveAttribute('aria-expanded', 'false')
  await userEvent.click(screen.getByRole('button', { name: 'Save' }))
  await waitFor(() =>
    expect(put).toHaveBeenCalledWith(
      '/api/channel/routing-pool',
      expect.objectContaining({
        channels: [pool.channels[0], { ...pool.channels[1], weight: 30 }],
      })
    )
  )
})

test('moving a priority group preserves tied membership and channel weights', async () => {
  const pool = fixture()
  pool.channels.push({
    channel_id: 3,
    priority: 100,
    weight: 30,
    enabled: false,
  })
  pool.candidates.push({
    channel_id: 3,
    channel_name: 'Gamma',
    priority: 100,
    weight: 30,
    available: true,
  })
  pool.channels.push({ channel_id: 4, priority: 20, weight: 9, enabled: true })
  pool.candidates.push({
    channel_id: 4,
    channel_name: 'Delta',
    priority: 20,
    weight: 9,
    available: true,
  })
  const put = vi.spyOn(api, 'put').mockImplementation(async (_url, body) => ({
    data: { success: true, data: { ...(body as object), revision: 'saved' } },
  }))
  mountCard(pool)
  const handle = screen.getByRole('button', {
    name: 'Drag Priority 80 to reorder',
  })
  handle.focus()
  fireEvent.keyDown(handle, { key: 'ArrowUp' })
  expect(
    screen.getByRole('button', { name: 'Drag Priority 100 to reorder' })
  ).toHaveFocus()
  const group = screen.getByRole('group', { name: 'Priority 80' })
  expect(within(group).getByText('Alpha')).toBeVisible()
  expect(within(group).getByText('Gamma')).toBeVisible()
  await userEvent.click(screen.getByRole('button', { name: 'Save' }))
  await waitFor(() =>
    expect(put).toHaveBeenCalledWith(
      '/api/channel/routing-pool',
      expect.objectContaining({
        channels: [
          { ...pool.channels[1], priority: 100 },
          { ...pool.channels[0], priority: 80 },
          { ...pool.channels[2], priority: 80 },
          pool.channels[3],
        ],
      })
    )
  )
})

test('a channel drag handle moves only that channel into the adjacent priority group', async () => {
  const pool = fixture()
  pool.channels[1].priority = 100
  pool.channels.push({ channel_id: 3, priority: 80, weight: 5, enabled: true })
  pool.candidates.push({
    channel_id: 3,
    channel_name: 'Gamma',
    priority: 80,
    weight: 5,
    available: true,
  })
  const put = vi.spyOn(api, 'put').mockImplementation(async (_url, body) => ({
    data: { success: true, data: { ...(body as object), revision: 'saved' } },
  }))
  mountCard(pool)
  const handle = screen.getByRole('button', { name: 'Drag 1 to reorder' })
  expect(handle).toBeVisible()
  fireEvent.keyDown(handle, { key: 'ArrowDown' })
  expect(
    within(screen.getByRole('group', { name: 'Priority 100' })).getByText(
      'Beta'
    )
  ).toBeVisible()
  const target = screen.getByRole('group', { name: 'Priority 80' })
  expect(within(target).getByText('Alpha')).toBeVisible()
  expect(within(target).getByText('Gamma')).toBeVisible()
  await userEvent.click(screen.getByRole('button', { name: 'Save' }))
  await waitFor(() =>
    expect(put).toHaveBeenCalledWith(
      '/api/channel/routing-pool',
      expect.objectContaining({
        channels: [
          pool.channels[1],
          pool.channels[2],
          { ...pool.channels[0], priority: 80 },
        ],
      })
    )
  )
})

test('editing channel priority keeps its popover focused and regroups after the editor closes', async () => {
  mountCard()
  const priority = getSettingsField('spinbutton', {
    name: 'Priority for Alpha',
  })
  await userEvent.clear(priority)
  await userEvent.type(priority, '80')
  expect(priority).toHaveFocus()
  expect(screen.getByRole('dialog', { name: 'Settings: Alpha' })).toBeVisible()
  expect(
    within(screen.getByRole('group', { name: 'Priority 100' })).getByText(
      'Alpha'
    )
  ).toBeVisible()
  await userEvent.keyboard('{Escape}')
  expect(screen.getByRole('button', { name: 'Settings: Alpha' })).toHaveFocus()
  expect(
    screen.queryByRole('group', { name: 'Priority 100' })
  ).not.toBeInTheDocument()
  const group = screen.getByRole('group', { name: 'Priority 80' })
  expect(within(group).getByText('Alpha')).toBeVisible()
  expect(within(group).getByText('Beta')).toBeVisible()
})

test('pool and channel gears open floating settings outside the card and Escape returns focus', async () => {
  const put = vi.spyOn(api, 'put')
  mountCard(fixture(), true)
  const pool = screen.getByRole('button', { name: 'Settings: kimi-k3' })
  const alpha = screen.getByRole('button', { name: 'Settings: Alpha' })
  const beta = screen.getByRole('button', { name: 'Settings: Beta' })
  expect(pool).toHaveAttribute('aria-expanded', 'false')
  expect(alpha).toHaveAttribute('aria-expanded', 'false')
  expect(
    screen.queryByRole('combobox', { name: 'Prefix cache routing' })
  ).not.toBeInTheDocument()
  expect(
    screen.queryByRole('spinbutton', { name: 'Priority for Alpha' })
  ).not.toBeInTheDocument()
  expect(screen.getAllByLabelText('Cache hit rate: —')).toHaveLength(2)

  await userEvent.click(pool)
  const poolPanel = screen.getByRole('dialog', { name: 'Settings: kimi-k3' })
  expect(pool.closest('[data-slot="card"]')).not.toContainElement(poolPanel)
  expect(
    getSettingsField('combobox', { name: 'Prefix cache routing' })
  ).toBeVisible()
  expect(
    getSettingsField('combobox', { name: 'Early empty chunk' })
  ).toBeVisible()
  expect(alpha).toHaveAttribute('aria-expanded', 'false')
  await userEvent.keyboard('{Escape}')
  expect(pool).toHaveAttribute('aria-expanded', 'false')
  expect(pool).toHaveFocus()

  act(() => alpha.focus())
  await userEvent.keyboard('{Enter}')
  expect(alpha).toHaveAttribute('aria-expanded', 'true')
  const channelPanel = screen.getByRole('dialog', { name: 'Settings: Alpha' })
  expect(alpha.closest('li')).not.toContainElement(channelPanel)
  expect(
    getSettingsField('spinbutton', { name: 'Priority for Alpha' })
  ).toBeVisible()
  expect(
    getSettingsField('combobox', { name: 'Early chunk for Alpha' })
  ).toBeVisible()
  expect(beta).toHaveAttribute('aria-expanded', 'false')
  expect(
    screen.queryByRole('spinbutton', { name: 'Priority for Beta' })
  ).not.toBeInTheDocument()
  expect(put).not.toHaveBeenCalled()
  expect(screen.getByRole('button', { name: 'Save' })).toBeDisabled()
  await userEvent.keyboard('{Escape}')
  expect(alpha).toHaveAttribute('aria-expanded', 'false')
  expect(alpha).toHaveFocus()
})

test('closing the channel popover preserves explicit zero and saves the draft while hidden', async () => {
  const put = vi.spyOn(api, 'put').mockImplementation(async (_url, body) => ({
    data: { success: true, data: { ...(body as object), revision: 'saved' } },
  }))
  mountCard(fixture(), true)
  let trigger = screen.getByRole('button', { name: 'Settings: Alpha' })
  await userEvent.click(trigger)
  fireEvent.change(
    getSettingsField('spinbutton', { name: 'Priority for Alpha' }),
    {
      target: { value: '0' },
    }
  )
  trigger.focus()
  await userEvent.keyboard(' ')
  trigger = screen.getByRole('button', { name: 'Settings: Alpha' })
  expect(trigger).toHaveAttribute('aria-expanded', 'false')
  expect(trigger).toHaveFocus()
  expect(
    screen.queryByRole('spinbutton', { name: 'Priority for Alpha' })
  ).not.toBeInTheDocument()
  await userEvent.click(trigger)
  expect(
    getSettingsField('spinbutton', { name: 'Priority for Alpha' })
  ).toHaveValue(0)
  await userEvent.click(trigger)
  await userEvent.click(screen.getByRole('button', { name: 'Save' }))
  await waitFor(() =>
    expect(put).toHaveBeenCalledWith(
      '/api/channel/routing-pool',
      expect.objectContaining({
        channels: [
          { channel_id: 1, priority: 0, weight: 10, enabled: true },
          { channel_id: 2, priority: 80, weight: 0, enabled: true },
        ],
      })
    )
  )
})

test('saving an invalid value after closing the channel popover reopens its settings', async () => {
  const put = vi.spyOn(api, 'put')
  mountCard(fixture(), true)
  let trigger = screen.getByRole('button', { name: 'Settings: Alpha' })
  await userEvent.click(trigger)
  fireEvent.change(
    getSettingsField('spinbutton', { name: 'Priority for Alpha' }),
    {
      target: { value: '-1' },
    }
  )
  await userEvent.click(trigger)
  trigger = screen.getByRole('button', { name: 'Settings: Alpha' })
  await userEvent.click(screen.getByRole('button', { name: 'Save' }))
  await waitFor(() => expect(trigger).toHaveAttribute('aria-expanded', 'true'))
  expect(
    getSettingsField('spinbutton', { name: 'Priority for Alpha' })
  ).toBeInvalid()
  expect(put).not.toHaveBeenCalled()
})

test.each([
  ['Enabled', true, false],
  ['Disabled', false, true],
  ['Inherit global', undefined, true],
] as const)(
  'prefix routing choice %s saves in a priority pool without scheduling defaults',
  async (label, expected, initial) => {
    const pool = fixture()
    pool.tuning = { chat_prefix_enabled: initial }
    delete pool.tuning_defaults
    const put = vi.spyOn(api, 'put').mockImplementation(async (_url, body) => ({
      data: { success: true, data: { ...(body as object), revision: 'saved' } },
    }))
    mountCard(pool)
    const selector = getSettingsField('combobox', {
      name: 'Prefix cache routing',
    })
    await userEvent.click(selector)
    await userEvent.click(screen.getByRole('option', { name: label }))
    await userEvent.click(screen.getByRole('button', { name: 'Save' }))
    await waitFor(() =>
      expect(put).toHaveBeenLastCalledWith(
        '/api/channel/routing-pool',
        expect.objectContaining({
          tuning: expect.objectContaining(
            expected === undefined ? {} : { chat_prefix_enabled: expected }
          ),
        })
      )
    )
    const body = put.mock.calls.at(-1)?.[1] as {
      tuning: { chat_prefix_enabled?: boolean }
    }
    expect(body.tuning.chat_prefix_enabled).toBe(expected)
  }
)

test('prefix routing override is disabled for read-only pools', () => {
  mountCard(fixture(), false)
  expect(
    getSettingsField('combobox', { name: 'Prefix cache routing' })
  ).toBeDisabled()
})

test('early chunks save the pool mode and only that pool channel override without scheduling defaults', async () => {
  const pool = fixture()
  delete pool.tuning_defaults
  const put = vi.spyOn(api, 'put').mockImplementation(async (_url, body) => ({
    data: { success: true, data: { ...(body as object), revision: 'saved' } },
  }))
  mountCard(pool)
  await userEvent.click(
    getSettingsField('combobox', { name: 'Early empty chunk' })
  )
  await userEvent.click(screen.getByRole('option', { name: 'Observe only' }))
  await userEvent.click(
    getSettingsField('combobox', { name: 'Early chunk for Alpha' })
  )
  await userEvent.click(screen.getByRole('option', { name: 'Enabled' }))
  expect(
    getSettingsField('combobox', { name: 'Early chunk for Beta' })
  ).toHaveTextContent('Inherit pool')
  await userEvent.click(screen.getByRole('button', { name: 'Save' }))
  await waitFor(() =>
    expect(put).toHaveBeenCalledWith(
      '/api/channel/routing-pool',
      expect.objectContaining({
        routing_group: 'standard',
        model: 'kimi-k3',
        tuning: {
          chat_early_chunk: { mode: 'observe', channels: { 1: 'enabled' } },
        },
      })
    )
  )
})

test('read-only pools cannot change early chunk or channel modes', async () => {
  mountCard(fixture(), false)
  expect(
    getSettingsField('combobox', { name: 'Early empty chunk' })
  ).toBeDisabled()
  expect(
    getSettingsField('combobox', { name: 'Early chunk for Alpha' })
  ).toBeDisabled()
})

test('pool parameters open beside the return action and save only explicit overrides while preserving the channel draft', async () => {
  useAuthStore.getState().auth.setUser({ id: 1, username: 'root', role: 100 })
  const pool = fixture()
  pool.mode = 'smart'
  mockMonitorReads(pool)
  const put = vi.spyOn(api, 'put').mockImplementation(async (_url, body) => ({
    data: { success: true, data: { ...(body as object), revision: 'saved' } },
  }))
  mountCard(pool)
  fireEvent.change(
    getSettingsField('spinbutton', { name: 'Priority for Alpha' }),
    { target: { value: '7' } }
  )
  await userEvent.click(screen.getByRole('button', { name: 'Monitoring view' }))
  const monitor = screen.getByRole('dialog', {
    name: 'kimi-k3 · standard · Default configuration',
  })
  const trigger = within(monitor).getByRole('button', {
    name: 'Scheduling parameters',
  })
  const back = within(monitor).getByRole('button', {
    name: 'Back to card view',
  })
  expect(
    trigger.compareDocumentPosition(back) & Node.DOCUMENT_POSITION_FOLLOWING
  ).toBeTruthy()
  await userEvent.click(trigger)
  const panel = screen.getByRole('dialog', { name: 'Scheduling parameters' })
  expect(trigger).toHaveAttribute('aria-expanded', 'true')
  const cooldown = within(panel).getByRole('spinbutton', {
    name: 'Hard failure cooldown (seconds)',
  })
  expect(cooldown).toHaveValue(null)
  expect(cooldown).toHaveAttribute('placeholder', '30')
  fireEvent.change(cooldown, { target: { value: '120' } })
  fireEvent.change(
    within(panel).getByRole('spinbutton', {
      name: 'Recovery observation time (seconds)',
    }),
    { target: { value: '90' } }
  )
  await userEvent.click(within(panel).getByRole('button', { name: 'Save' }))
  await waitFor(() =>
    expect(put).toHaveBeenCalledWith(
      '/api/channel/routing-pool',
      expect.objectContaining({
        tuning: {
          hard_failure_cooldown_seconds: 120,
          recovery_observation_seconds: 90,
        },
        channels: [
          { channel_id: 1, priority: 7, weight: 10, enabled: true },
          { channel_id: 2, priority: 80, weight: 0, enabled: true },
        ],
      })
    )
  )
  await userEvent.keyboard('{Escape}')
  expect(monitor).toBeVisible()
  expect(trigger).toHaveFocus()
})

test('pool tuning rejects conflicting values and restores inheritance without changing other pool settings', async () => {
  useAuthStore.getState().auth.setUser({ id: 1, username: 'root', role: 100 })
  const pool = fixture()
  pool.mode = 'smart'
  pool.tuning = {
    hard_failure_cooldown_seconds: 120,
    chat_prefix_enabled: false,
  }
  mockMonitorReads(pool)
  const put = vi.spyOn(api, 'put').mockImplementation(async (_url, body) => ({
    data: { success: true, data: { ...(body as object), revision: 'saved' } },
  }))
  mountCard(pool)
  await userEvent.click(screen.getByRole('button', { name: 'Monitoring view' }))
  await userEvent.click(
    screen.getByRole('button', { name: 'Scheduling parameters' })
  )
  const panel = screen.getByRole('dialog', { name: 'Scheduling parameters' })
  fireEvent.change(
    within(panel).getByRole('spinbutton', {
      name: 'Maximum hard failure cooldown (seconds)',
    }),
    { target: { value: '60' } }
  )
  await userEvent.click(within(panel).getByRole('button', { name: 'Save' }))
  expect(
    await within(panel).findByText(
      'Maximum hard failure cooldown cannot be below the base cooldown'
    )
  ).toBeVisible()
  expect(put).not.toHaveBeenCalled()
  await userEvent.click(
    within(panel).getByRole('button', { name: 'Use global defaults' })
  )
  await userEvent.click(within(panel).getByRole('button', { name: 'Save' }))
  await waitFor(() =>
    expect(put).toHaveBeenCalledWith(
      '/api/channel/routing-pool',
      expect.objectContaining({
        tuning: { chat_prefix_enabled: false },
        channels: pool.channels,
      })
    )
  )
})

test('saving with an invalid parameter in a closed panel reopens the parameter editor', async () => {
  useAuthStore.getState().auth.setUser({ id: 1, username: 'root', role: 100 })
  const pool = fixture()
  pool.mode = 'smart'
  mockMonitorReads(pool)
  const put = vi.spyOn(api, 'put')
  mountCard(pool)
  await userEvent.click(screen.getByRole('button', { name: 'Monitoring view' }))
  const monitor = screen.getByRole('dialog', {
    name: 'kimi-k3 · standard · Default configuration',
  })
  await userEvent.click(
    within(monitor).getByRole('button', { name: 'Scheduling parameters' })
  )
  const panel = screen.getByRole('dialog', { name: 'Scheduling parameters' })
  fireEvent.change(
    within(panel).getByRole('spinbutton', {
      name: 'Hard failure cooldown (seconds)',
    }),
    { target: { value: '0' } }
  )
  await userEvent.keyboard('{Escape}')
  await userEvent.click(within(monitor).getByRole('button', { name: 'Save' }))
  const reopened = await screen.findByRole('dialog', {
    name: 'Scheduling parameters',
  })
  expect(
    within(reopened).getByRole('spinbutton', {
      name: 'Hard failure cooldown (seconds)',
    })
  ).toHaveAttribute('aria-invalid', 'true')
  expect(put).not.toHaveBeenCalled()
})

test('inherited probe precision survives interface language changes and invalid locale fallback', async () => {
  useAuthStore.getState().auth.setUser({ id: 1, username: 'root', role: 100 })
  const pool = fixture()
  pool.mode = 'smart'
  mockMonitorReads(pool)
  const instance = createInstance()
  await instance.use(initReactI18next).init({
    lng: 'en',
    fallbackLng: 'en',
    resources: { en: { translation: {} } },
  })
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  clients.push(client)
  render(
    <I18nextProvider i18n={instance}>
      <QueryClientProvider client={client}>
        <PoolCard pool={pool} canWrite onDirtyChange={() => {}} />
      </QueryClientProvider>
    </I18nextProvider>
  )
  await userEvent.click(screen.getByRole('button', { name: 'Monitoring view' }))
  await userEvent.click(
    screen.getByRole('button', { name: 'Scheduling parameters' })
  )
  const panel = screen.getByRole('dialog', { name: 'Scheduling parameters' })
  for (const [language, value] of [
    ['zhCN', '0.015'],
    ['zhTW', '0.015'],
    ['en', '0.015'],
    ['fr', '0,015'],
    ['ru', '0,015'],
    ['ja', '0.015'],
    ['vi', '0,015'],
    [
      'invalid_locale!',
      new Intl.NumberFormat(undefined, { maximumFractionDigits: 3 }).format(
        0.015
      ),
    ],
  ]) {
    await act(() => instance.changeLanguage(language))
    expect(within(panel).getByText(`Global default: ${value}`)).toBeVisible()
  }
})

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

function mockMonitorReads(pool: RoutingPoolView, failed = false) {
  return vi.spyOn(api, 'get').mockImplementation(async (url) => {
    if (url === '/api/channel/monitoring') {
      return {
        data: {
          success: true,
          data: [
            {
              channel_id: 1,
              state: 'disabled',
              buckets: [],
              cache: { hit_rate: 75.25, truncated: false },
            },
          ],
        },
      }
    }
    if (url === '/api/channel/routing-pool') {
      return {
        data: {
          success: true,
          data: {
            pool,
            candidates: pool.candidates,
            effective_mode: pool.mode,
          },
        },
      }
    }
    if (url === '/api/channel/routing-monitor') {
      if (failed) throw new Error('monitor unavailable')
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
    throw new Error(`Unexpected monitor request: ${url}`)
  })
}

test('opening and returning from a pool monitor retains the same dirty editor and channel monitoring statistics', async () => {
  useAuthStore.getState().auth.setUser({ id: 1, username: 'root', role: 100 })
  const pool = fixture()
  const get = mockMonitorReads(pool)
  const put = vi.spyOn(api, 'put').mockImplementation(async (_url, body) => ({
    data: { success: true, data: { ...(body as object), revision: 'saved' } },
  }))
  const { onDirtyChange } = mountCard(pool)
  const priority = getSettingsField('spinbutton', {
    name: 'Priority for Alpha',
  })
  fireEvent.change(priority, { target: { value: '7' } })
  await userEvent.click(
    getSettingsField('switch', { name: 'Enable Beta in this pool' })
  )
  const trigger = screen.getByRole('button', { name: 'Monitoring view' })
  await userEvent.click(trigger)
  const monitor = screen.getByRole('dialog', {
    name: 'kimi-k3 · standard · Default configuration',
  })
  expect(
    getSettingsField('spinbutton', { name: 'Priority for Alpha' })
  ).toHaveValue(7)
  expect(
    await within(monitor).findByLabelText('Cache hit rate: 75.25%')
  ).toBeVisible()
  expect(
    await within(monitor).findByText(/No (?:adaptive )?routing observations/)
  ).toBeVisible()
  expect(
    monitor.querySelector('[data-routing-monitor-overview]')?.parentElement
  ).toHaveClass('xl:grid-rows-[auto_1fr]')
  const pause = within(monitor).getByRole('button', {
    name: 'Pause live updates',
  })
  expect(
    monitor.querySelector('[data-routing-monitor-header]')
  ).toContainElement(pause)
  expect(
    monitor.querySelector('[data-routing-monitor-live]')
  ).not.toContainElement(pause)
  expect(get).toHaveBeenCalledWith('/api/channel/routing-pool', {
    params: { routing_group: 'standard', model: 'kimi-k3' },
  })
  const editor = trigger.closest('[data-pool-editor]')
  expect(editor).toHaveClass('xl:row-start-2')
  await userEvent.click(
    within(monitor).getByRole('button', { name: 'TTFT prediction grid' })
  )
  expect(editor).toHaveClass('xl:row-start-1', 'xl:row-span-2')
  expect(editor).not.toHaveClass('xl:row-start-2')
  expect(
    await within(monitor).findByText('Prediction statistics unavailable')
  ).toBeVisible()
  expect(
    getSettingsField('spinbutton', { name: 'Priority for Alpha' })
  ).toHaveValue(7)
  await userEvent.click(
    within(monitor).getByRole('button', { name: 'Routing observations' })
  )
  expect(
    await within(monitor).findByText(/No (?:adaptive )?routing observations/)
  ).toBeVisible()
  expect(editor).toHaveClass('xl:row-start-2')
  await userEvent.click(
    within(monitor).getByRole('button', { name: 'Back to card view' })
  )
  expect(screen.queryByRole('dialog')).not.toBeInTheDocument()
  expect(trigger).toHaveFocus()
  expect(
    getSettingsField('spinbutton', { name: 'Priority for Alpha' })
  ).toHaveValue(7)
  expect(
    getSettingsField('switch', { name: 'Enable Beta in this pool' })
  ).not.toBeChecked()
  expect(onDirtyChange).toHaveBeenLastCalledWith(true)
  await userEvent.click(screen.getByRole('button', { name: 'Save' }))
  await waitFor(() =>
    expect(put).toHaveBeenCalledWith(
      '/api/channel/routing-pool',
      expect.objectContaining({
        channels: [
          { channel_id: 1, priority: 7, weight: 10, enabled: true },
          { channel_id: 2, priority: 80, weight: 0, enabled: false },
        ],
      })
    )
  )
})

test('read-only pool monitoring keeps the original configuration visible when live data fails', async () => {
  useAuthStore.getState().auth.setUser({ id: 1, username: 'root', role: 100 })
  mockMonitorReads(fixture(), true)
  mountCard(fixture(), false)
  await userEvent.click(screen.getByRole('button', { name: 'Monitoring view' }))
  const monitor = screen.getByRole('dialog', {
    name: 'kimi-k3 · standard · Default configuration',
  })
  expect(
    await within(monitor).findByText('Unable to refresh routing observations')
  ).toBeVisible()
  expect(
    getSettingsField('spinbutton', { name: 'Priority for Alpha' })
  ).toBeDisabled()
  expect(within(monitor).getByRole('button', { name: 'Save' })).toBeDisabled()
  expect(
    await within(monitor).findByLabelText('Cache hit rate: 75.25%')
  ).toBeVisible()
  await userEvent.click(
    within(monitor).getByRole('button', { name: 'Back to card view' })
  )
  expect(screen.getByRole('button', { name: 'Monitoring view' })).toHaveFocus()
})

test('discount-first strategy saves model latency limits and preserves the existing pool members', async () => {
  const put = vi.spyOn(api, 'put').mockImplementation(async (_url, body) => ({
    data: { success: true, data: { ...(body as object), revision: 'saved' } },
  }))
  mountCard()
  await userEvent.click(getSettingsField('button', { name: 'Edit strategy' }))
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
  await userEvent.click(getSettingsField('button', { name: 'Edit strategy' }))
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

test('keyboard reordering swaps priority groups while preserving zero weights', async () => {
  const pool = fixture()
  const put = vi.spyOn(api, 'put').mockImplementation(async (_url, body) => ({
    data: { success: true, data: { ...(body as object), revision: 'saved' } },
  }))
  const { onDirtyChange } = mountCard(pool)
  fireEvent.keyDown(
    screen.getByRole('button', { name: 'Drag Priority 80 to reorder' }),
    {
      key: 'ArrowUp',
    }
  )
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
          { channel_id: 2, priority: 100, weight: 0, enabled: true },
          { channel_id: 1, priority: 80, weight: 10, enabled: true },
        ],
      })
    )
  )
})

test('a failed save retains explicit zero priority, disabled state and the dirty draft', async () => {
  vi.spyOn(api, 'put').mockRejectedValue(new Error('conflicting pool revision'))
  const { onDirtyChange } = mountCard()
  fireEvent.change(
    getSettingsField('spinbutton', { name: 'Priority for Alpha' }),
    { target: { value: '0' } }
  )
  await userEvent.click(
    getSettingsField('switch', { name: 'Enable Beta in this pool' })
  )
  await userEvent.click(screen.getByRole('button', { name: 'Save' }))
  await waitFor(() =>
    expect(screen.getByRole('button', { name: 'Save' })).toBeEnabled()
  )
  expect(
    getSettingsField('spinbutton', { name: 'Priority for Alpha' })
  ).toHaveValue(0)
  expect(
    getSettingsField('switch', { name: 'Enable Beta in this pool' })
  ).not.toBeChecked()
  expect(onDirtyChange).toHaveBeenLastCalledWith(true)
})

test('invalid numeric input prevents saving and explains the allowed value contract', async () => {
  const put = vi.spyOn(api, 'put')
  mountCard()
  fireEvent.change(
    getSettingsField('spinbutton', { name: 'Weight for Alpha' }),
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
  await userEvent.click(screen.getByRole('button', { name: 'Settings: Alpha' }))
  expect(
    getSettingsField('spinbutton', { name: 'Priority for Alpha' })
  ).toHaveValue(100)
})

test('read-only cards expose configuration while disabling mutation, reordering and candidate fetches', async () => {
  const get = vi.spyOn(api, 'get')
  const { onDirtyChange } = mountCard(fixture(), false)
  expect(
    getSettingsField('spinbutton', { name: 'Priority for Alpha' })
  ).toBeDisabled()
  expect(
    getSettingsField('switch', { name: 'Enable Alpha in this pool' })
  ).toHaveAttribute('aria-disabled', 'true')
  expect(getSettingsField('button', { name: 'Edit strategy' })).toBeDisabled()
  const drag = screen.getByRole('button', {
    name: 'Drag Priority 80 to reorder',
  })
  expect(drag).toBeDisabled()
  fireEvent.keyDown(drag, { key: 'ArrowUp' })
  expect(
    getSettingsField('spinbutton', { name: 'Priority for Alpha' })
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
    getSettingsField('spinbutton', { name: 'Priority for Alpha', hidden: true })
  ).toHaveValue(100)
  await userEvent.click(
    within(confirmation).getByRole('button', { name: 'Cancel' })
  )
  await waitFor(() =>
    expect(screen.queryByRole('alertdialog')).not.toBeInTheDocument()
  )
  expect(
    getSettingsField('spinbutton', { name: 'Priority for Alpha' })
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
    getSettingsField('spinbutton', { name: 'Weight for Beta' })
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
      getSettingsField('spinbutton', { name: 'Priority for Alpha' })
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
