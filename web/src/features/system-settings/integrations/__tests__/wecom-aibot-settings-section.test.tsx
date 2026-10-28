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
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { useState } from 'react'
import { toast } from 'sonner'
import { beforeEach, expect, test, vi } from 'vitest'

import { api } from '@/lib/api'

import { SettingsPageProvider } from '../../components/settings-page-context'
import { WecomAIBotSettingsSection } from '../wecom-aibot-settings-section'

const baseConfig = {
  enabled: false,
  bot_id: 'bot-001',
  admin_userids: '',
  report_enabled: false,
  report_hour: 9,
  secret_masked: '****e359',
  channel_id: 3,
  model: 'gpt-4o-mini',
  max_tool_rounds: 5,
  session_rounds: 10,
  session_ttl_minutes: 10,
  allow_all_users: false,
  user_whitelist: 'alice, bob',
  group_enabled: true,
  welcome_text: '',
}

const baseStatus = {
  enabled: false,
  holding: false,
  state: 'standby',
  last_error: '',
  connected_at: 0,
  last_pong_at: 0,
}

function Fixture() {
  const [container, setContainer] = useState<HTMLDivElement | null>(null)
  return (
    <>
      <div ref={setContainer} />
      <SettingsPageProvider actionsContainer={container}>
        <WecomAIBotSettingsSection />
      </SettingsPageProvider>
    </>
  )
}

async function renderSettings() {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  })
  const router = createRouter({
    routeTree: createRootRoute({ component: Fixture }),
    history: createMemoryHistory({ initialEntries: ['/'] }),
  })
  render(
    <QueryClientProvider client={client}>
      <RouterProvider router={router} />
    </QueryClientProvider>
  )
  await screen.findByRole('switch', { name: 'Enable the WeCom AI bot' })
  // 配置经独立端点异步加载后表单才会重置：等白名单文本域出现取值再断言
  await screen.findByDisplayValue('alice, bob')
}

beforeEach(() => {
  vi.spyOn(api, 'put').mockResolvedValue({ data: { success: true } })
  vi.spyOn(api, 'get').mockImplementation(async (url: string) => {
    if (url === '/api/wecom-aibot/config') {
      return { data: { success: true, data: baseConfig } }
    }
    if (url === '/api/wecom-aibot/status') {
      return { data: { success: true, data: baseStatus } }
    }
    throw new Error(`unexpected GET ${url}`)
  })
})

test('renders the loaded config and connection status', async () => {
  await renderSettings()
  expect(
    screen.getByRole('switch', { name: 'Enable the WeCom AI bot' })
  ).not.toBeChecked()
  expect(screen.getByRole('textbox', { name: 'BotID' })).toHaveValue('bot-001')
  expect(
    screen.getByRole('spinbutton', { name: 'Model channel ID' })
  ).toHaveValue(3)
  expect(screen.getByLabelText('Secret')).toBeInTheDocument()
  expect(
    await screen.findByText('Standby (another instance holds the connection)')
  ).toBeInTheDocument()
})

test('saves edited fields and a newly typed secret through the dedicated endpoint', async () => {
  const user = userEvent.setup()
  await renderSettings()
  const botId = screen.getByRole('textbox', { name: 'BotID' })
  await user.clear(botId)
  await user.type(botId, 'bot-002')
  await user.type(screen.getByLabelText('Secret'), 'new-secret')
  await user.click(
    screen.getByRole('button', { name: 'Save WeCom AI bot settings' })
  )
  await waitFor(() => expect(api.put).toHaveBeenCalledTimes(1))
  expect(api.put).toHaveBeenCalledWith('/api/wecom-aibot/config', {
    enabled: false,
    bot_id: 'bot-002',
    secret: 'new-secret',
    channel_id: 3,
    model: 'gpt-4o-mini',
    max_tool_rounds: 5,
    session_rounds: 10,
    session_ttl_minutes: 10,
    allow_all_users: false,
    user_whitelist: 'alice, bob',
    group_enabled: true,
    welcome_text: '',
    admin_userids: '',
    report_enabled: false,
    report_hour: 9,
    report_targets: '',
  })
})

test('blocks enabling without the required bot credentials', async () => {
  const user = userEvent.setup()
  vi.mocked(api.get).mockImplementation(async (url: string) => {
    if (url === '/api/wecom-aibot/config') {
      return {
        data: {
          success: true,
          data: { ...baseConfig, bot_id: '', secret_masked: '' },
        },
      }
    }
    return { data: { success: true, data: baseStatus } }
  })
  await renderSettings()
  await user.click(
    screen.getByRole('switch', { name: 'Enable the WeCom AI bot' })
  )
  await user.click(
    screen.getByRole('button', { name: 'Save WeCom AI bot settings' })
  )
  expect(
    await screen.findByText('BotID is required when the bot is enabled')
  ).toBeInTheDocument()
  expect(
    screen.getByText('Secret is required when the bot is enabled')
  ).toBeInTheDocument()
  expect(api.put).not.toHaveBeenCalled()
})

test('keeps the edited secret when the server rejects the configuration', async () => {
  const success = vi.spyOn(toast, 'success')
  const error = vi.spyOn(toast, 'error')
  vi.mocked(api.put).mockResolvedValue({
    data: { success: false, message: 'Configuration rejected' },
  })
  const user = userEvent.setup()
  await renderSettings()
  await user.type(screen.getByLabelText('Secret'), 'new-secret')
  await user.click(
    screen.getByRole('button', { name: 'Save WeCom AI bot settings' })
  )
  await waitFor(() => expect(error).toHaveBeenCalled())
  expect(success).not.toHaveBeenCalled()
  expect(screen.getByLabelText('Secret')).toHaveValue('new-secret')
})
