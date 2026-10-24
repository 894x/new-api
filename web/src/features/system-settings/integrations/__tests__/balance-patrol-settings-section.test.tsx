import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import {
  createMemoryHistory,
  createRootRoute,
  createRouter,
  RouterProvider,
} from '@tanstack/react-router'
import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { useState } from 'react'
import { beforeEach, expect, test, vi } from 'vitest'

import { api } from '@/lib/api'

import { SettingsPageProvider } from '../../components/settings-page-context'
import { BalancePatrolSettingsSection } from '../balance-patrol-settings-section'

const baseDefaults = {
  'balance_patrol_setting.channel_enabled': false,
  'balance_patrol_setting.channel_minutes': 30,
  'balance_patrol_setting.low_balance_threshold': 1,
  'balance_patrol_setting.disable_on_zero_balance': true,
  'balance_patrol_setting.user_enabled': false,
  'balance_patrol_setting.user_minutes': 30,
  'balance_patrol_setting.user_min_quota': 0,
  'balance_patrol_setting.user_list_limit': 20,
  'balance_patrol_setting.critical_balance_threshold': 0,
  'balance_patrol_setting.watchdog_enabled': false,
  'balance_patrol_setting.watchdog_minutes': 10,
  'balance_patrol_setting.watchdog_threshold': 10,
}

type FixtureProps = {
  defaults: typeof baseDefaults
}

function Fixture(props: FixtureProps) {
  const [container, setContainer] = useState<HTMLDivElement | null>(null)
  return (
    <>
      <div ref={setContainer} />
      <SettingsPageProvider actionsContainer={container}>
        <BalancePatrolSettingsSection defaultValues={props.defaults} />
      </SettingsPageProvider>
    </>
  )
}

async function renderSettings(defaults = baseDefaults) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  })
  const SectionFixture = () => <Fixture defaults={defaults} />
  const router = createRouter({
    routeTree: createRootRoute({ component: SectionFixture }),
    history: createMemoryHistory({ initialEntries: ['/'] }),
  })
  render(
    <QueryClientProvider client={client}>
      <RouterProvider router={router} />
    </QueryClientProvider>
  )
  return screen.findByRole('switch', {
    name: 'Enable channel balance patrol',
  })
}

beforeEach(() => {
  vi.spyOn(api, 'put').mockResolvedValue({ data: { success: true } })
})

test('renders the balance patrol settings with defaults', async () => {
  await renderSettings()
  expect(
    screen.getByRole('switch', { name: 'Enable channel balance patrol' })
  ).not.toBeChecked()
  expect(
    screen.getByRole('switch', { name: 'Disable channels at zero balance' })
  ).toBeChecked()
  expect(
    screen.getByRole('switch', { name: 'Enable user quota patrol' })
  ).not.toBeChecked()
  expect(
    screen.getByRole('spinbutton', {
      name: 'Channel patrol interval (minutes)',
    })
  ).toHaveValue(30)
  expect(
    screen.getByRole('spinbutton', {
      name: 'Low balance alert threshold (USD)',
    })
  ).toHaveValue(1)
  expect(
    screen.getByRole('spinbutton', {
      name: 'User low-balance threshold (quota)',
    })
  ).toHaveValue(0)
  expect(
    screen.getByRole('spinbutton', { name: 'Max users listed per alert' })
  ).toHaveValue(20)
})

test('saves only changed keys', async () => {
  const user = userEvent.setup()
  const toggle = await renderSettings()
  await user.click(toggle)
  await user.click(
    screen.getByRole('button', { name: 'Save balance patrol settings' })
  )
  await waitFor(() => expect(api.put).toHaveBeenCalledTimes(1))
  expect(api.put).toHaveBeenCalledWith('/api/option/', {
    key: 'balance_patrol_setting.channel_enabled',
    value: true,
  })
})

test('does not send anything when nothing changed', async () => {
  const user = userEvent.setup()
  await renderSettings()
  await user.click(
    screen.getByRole('button', { name: 'Save balance patrol settings' })
  )
  expect(api.put).not.toHaveBeenCalled()
})

test('rejects an out-of-range patrol interval without saving', async () => {
  const user = userEvent.setup()
  await renderSettings()
  const interval = screen.getByRole('spinbutton', {
    name: 'Channel patrol interval (minutes)',
  })
  fireEvent.change(interval, { target: { value: '0' } })
  await user.click(
    screen.getByRole('button', { name: 'Save balance patrol settings' })
  )
  expect(
    await screen.findByText(
      'Patrol interval must be between 1 and 10080 minutes'
    )
  ).toBeInTheDocument()
  expect(api.put).not.toHaveBeenCalled()
})

test('rejects an out-of-range list limit without saving', async () => {
  const user = userEvent.setup()
  await renderSettings()
  const listLimit = screen.getByRole('spinbutton', {
    name: 'Max users listed per alert',
  })
  fireEvent.change(listLimit, { target: { value: '99' } })
  await user.click(
    screen.getByRole('button', { name: 'Save balance patrol settings' })
  )
  expect(
    await screen.findByText('List limit must be between 0 and 50')
  ).toBeInTheDocument()
  expect(api.put).not.toHaveBeenCalled()
})
