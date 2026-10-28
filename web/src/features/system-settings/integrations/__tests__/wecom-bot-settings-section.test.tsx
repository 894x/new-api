import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import {
  createMemoryHistory,
  createRootRoute,
  createRouter,
  RouterProvider,
} from '@tanstack/react-router'
import { fireEvent, render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { useState } from 'react'
import { beforeEach, expect, test, vi } from 'vitest'

import { api } from '@/lib/api'

import { SettingsPageProvider } from '../../components/settings-page-context'
import { WecomBotSettingsSection } from '../wecom-bot-settings-section'

const baseDefaults = {
  'wecom_bot_setting.enabled': false,
  'wecom_bot_setting.error_notify_enabled': true,
  'wecom_bot_setting.error_cooldown_minutes': 10,
  'wecom_bot_setting.disable_notify_enabled': true,
  'wecom_bot_setting.enable_notify_enabled': true,
}

const botFixture = [
  {
    id: 'b1',
    name: 'Ops alert group',
    webhook_masked: '****e359',
    mention_userids: 'zhangsan',
    enabled: true,
  },
  {
    id: 'b2',
    name: 'Backup group',
    webhook_masked: '****abcd',
    mention_userids: '',
    enabled: false,
  },
]

type FixtureProps = {
  defaults: typeof baseDefaults
}

function Fixture(props: FixtureProps) {
  const [container, setContainer] = useState<HTMLDivElement | null>(null)
  return (
    <>
      <div ref={setContainer} />
      <SettingsPageProvider actionsContainer={container}>
        <WecomBotSettingsSection defaultValues={props.defaults} />
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
  return screen.findByRole('switch', { name: 'Enable WeCom bot alerts' })
}

beforeEach(() => {
  vi.spyOn(api, 'get').mockResolvedValue({
    data: { success: true, data: botFixture },
  })
  vi.spyOn(api, 'put').mockResolvedValue({ data: { success: true } })
  vi.spyOn(api, 'post').mockResolvedValue({ data: { success: true } })
  vi.spyOn(api, 'delete').mockResolvedValue({ data: { success: true } })
})

test('renders global settings defaults and the bot list', async () => {
  const toggle = await renderSettings()
  expect(toggle).not.toBeChecked()
  expect(
    screen.getByRole('spinbutton', { name: 'Error cooldown (minutes)' })
  ).toHaveValue(10)
  expect(
    screen.getByRole('switch', { name: 'Channel error alerts' })
  ).toBeChecked()
  expect(await screen.findByText('Ops alert group')).toBeInTheDocument()
  expect(screen.getByText('****e359')).toBeInTheDocument()
  expect(screen.getByText('Backup group')).toBeInTheDocument()
  expect(
    screen.getByRole('button', { name: 'Add bot' })
  ).toBeInTheDocument()
})

test('saves only changed global keys', async () => {
  const user = userEvent.setup()
  const toggle = await renderSettings()
  await user.click(toggle)
  await user.click(
    screen.getByRole('button', { name: 'Save WeCom bot settings' })
  )
  await waitFor(() => expect(api.put).toHaveBeenCalledTimes(1))
  expect(api.put).toHaveBeenCalledWith('/api/option/', {
    key: 'wecom_bot_setting.enabled',
    value: true,
  })
})

test('does not send anything when nothing changed', async () => {
  const user = userEvent.setup()
  await renderSettings()
  await user.click(
    screen.getByRole('button', { name: 'Save WeCom bot settings' })
  )
  expect(api.put).not.toHaveBeenCalled()
})

test('rejects an out-of-range cooldown without saving', async () => {
  const user = userEvent.setup()
  await renderSettings()
  const cooldown = screen.getByRole('spinbutton', {
    name: 'Error cooldown (minutes)',
  })
  fireEvent.change(cooldown, { target: { value: '0' } })
  await user.click(
    screen.getByRole('button', { name: 'Save WeCom bot settings' })
  )
  expect(
    await screen.findByText('Cooldown must be between 1 and 1440 minutes')
  ).toBeInTheDocument()
  expect(api.put).not.toHaveBeenCalled()
})

test('creates a bot through the management dialog', async () => {
  const user = userEvent.setup()
  await renderSettings()
  await user.click(screen.getByRole('button', { name: 'Add bot' }))
  const dialog = await screen.findByRole('dialog')
  await user.type(within(dialog).getByLabelText('Bot name'), 'Money ops group')
  await user.type(
    within(dialog).getByLabelText('Bot webhook key'),
    'key-123'
  )
  await user.click(within(dialog).getByRole('button', { name: 'Add' }))
  await waitFor(() =>
    expect(api.put).toHaveBeenCalledWith('/api/wecom-bot/', {
      name: 'Money ops group',
      webhook_api_key: 'key-123',
      mention_userids: '',
      enabled: true,
      subscriptions: [],
      message_style: 'markdown',
    })
  )
})

test('keeps the existing webhook when left blank on edit', async () => {
  const user = userEvent.setup()
  await renderSettings()
  const editButton = await screen.findAllByRole('button', { name: 'Edit' })
  await user.click(editButton[0])
  const dialog = await screen.findByRole('dialog')
  const nameInput = within(dialog).getByLabelText('Bot name')
  await user.clear(nameInput)
  await user.type(nameInput, 'Renamed group')
  await user.click(within(dialog).getByRole('button', { name: 'Update' }))
  await waitFor(() =>
    expect(api.post).toHaveBeenCalledWith('/api/wecom-bot/b1', {
      name: 'Renamed group',
      webhook_api_key: '',
      mention_userids: 'zhangsan',
      enabled: true,
      subscriptions: [],
      message_style: 'markdown',
    })
  )
})

test('deletes a bot after confirmation', async () => {
  const user = userEvent.setup()
  await renderSettings()
  const menuTriggers = await screen.findAllByRole('button', {
    name: 'Open menu',
  })
  await user.click(menuTriggers[0])
  await user.click(await screen.findByRole('menuitem', { name: 'Delete' }))
  await user.click(await screen.findByRole('button', { name: 'Continue' }))
  await waitFor(() =>
    expect(api.delete).toHaveBeenCalledWith('/api/wecom-bot/b1')
  )
})
