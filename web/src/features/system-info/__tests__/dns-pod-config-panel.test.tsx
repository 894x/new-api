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
import { webcrypto } from 'node:crypto'

import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import {
  cleanup,
  render,
  screen,
  waitFor,
  within,
} from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { createInstance } from 'i18next'
import { Toaster } from 'sonner'
import { afterEach, expect, it, vi } from 'vitest'

import en from '@/i18n/locales/en.json'
import fr from '@/i18n/locales/fr.json'
import ja from '@/i18n/locales/ja.json'
import ru from '@/i18n/locales/ru.json'
import viLocale from '@/i18n/locales/vi.json'
import zhTW from '@/i18n/locales/zh-TW.json'
import zhCN from '@/i18n/locales/zh.json'
import { api } from '@/lib/api'

import { DNSPodConfigPanel } from '../components/dns-pod-config-panel'
import { dnsPodConfigChangeHash } from '../lib/dns-traffic'
import type { DNSPodConfigState, DNSPodConfigUpdate } from '../types'

const initial: DNSPodConfigState = {
  revision: 'a'.repeat(64),
  enabled: true,
  domain: 'xunlitec.com',
  subdomain: 'newapi',
  record_line: '默认',
  record_type: 'A',
  secret_id_set: true,
  secret_key_set: true,
  session_token_set: true,
  source: 'database',
  can_save: true,
}

function renderConfiguration(state = initial) {
  vi.stubGlobal('crypto', webcrypto)
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  })
  vi.spyOn(api, 'get').mockImplementation(async (url) => ({
    data: {
      success: true,
      data:
        url === '/api/system-info/dns-traffic/config'
          ? state
          : {
              scope: 'dns.config.update',
              methods: [{ method: 'password', available: true }],
              oauth_providers: [],
              password_encryption_enabled: false,
            },
    },
  }))
  vi.spyOn(api, 'post').mockResolvedValue({
    data: {
      success: true,
      data: {
        scope: 'dns.config.update',
        method: 'password',
        proof_token: 'configuration-proof',
        expires_at: Math.floor(Date.now() / 1000) + 300,
      },
    },
  })
  render(
    <QueryClientProvider client={client}>
      <DNSPodConfigPanel />
      <Toaster />
    </QueryClientProvider>
  )
  return client
}

async function confirmConfiguration(user: ReturnType<typeof userEvent.setup>) {
  await user.click(
    screen.getByRole('button', { name: 'Save DNSPod configuration' })
  )
  await user.click(
    within(await screen.findByRole('alertdialog')).getByRole('button', {
      name: 'Continue',
    })
  )
  const verification = await screen.findByRole('dialog')
  await user.type(
    within(verification).getByLabelText('Password', { selector: 'input' }),
    'root-password'
  )
  await user.click(within(verification).getByRole('button', { name: 'Verify' }))
}

afterEach(() => {
  cleanup()
  vi.restoreAllMocks()
  vi.unstubAllGlobals()
})

it.each([
  ['en', 'DNSPod configuration'],
  ['zhCN', 'DNSPod 配置'],
  ['zhTW', 'DNSPod 設定'],
  ['fr', 'Configuration DNSPod'],
  ['ja', 'DNSPod 設定'],
  ['ru', 'Настройки DNSPod'],
  ['vi', 'Cấu hình DNSPod'],
])(
  'loads DNSPod copy through the shipped translation resources in %s',
  async (language, heading) => {
    const translations = createInstance()
    await translations.init({
      resources: { en, zhCN, zhTW, fr, ja, ru, vi: viLocale },
      lng: language,
      fallbackLng: false,
      nsSeparator: false,
    })
    expect(translations.t('DNSPod configuration')).toBe(heading)
    expect(translations.exists('DNS traffic allocation')).toBe(true)
    expect(translations.exists('Save DNSPod configuration')).toBe(true)
    expect(
      translations.exists('Current DNS share: {{share}} · TTL: {{ttl}} seconds')
    ).toBe(true)
  }
)

it('saves a credential replacement after verification, keeps the other secrets blank and clears entered credentials', async () => {
  const client = renderConfiguration()
  const user = userEvent.setup()
  const secret = await screen.findByLabelText('SecretKey')
  expect(secret).toHaveValue('')
  expect(secret).toHaveAttribute(
    'placeholder',
    'Already set; leave blank to keep'
  )
  await user.type(secret, 'replacement-key')
  await user.click(
    screen.getByRole('switch', { name: 'Clear saved DNSPod session token' })
  )
  const put = vi.spyOn(api, 'put').mockResolvedValue({
    data: {
      success: true,
      data: {
        ...initial,
        revision: 'b'.repeat(64),
        session_token_set: false,
      },
    },
  })
  await confirmConfiguration(user)
  expect(
    await screen.findByText(
      'DNSPod configuration saved. DNS records were not changed.'
    )
  ).toBeVisible()
  expect(put).toHaveBeenCalledWith(
    '/api/system-info/dns-traffic/config',
    expect.objectContaining({
      secret_id: '',
      secret_key: 'replacement-key',
      session_token: '',
      clear_session_token: true,
      domain: 'xunlitec.com',
      subdomain: 'newapi',
    }),
    expect.objectContaining({
      headers: { 'X-Security-Proof': 'configuration-proof' },
    })
  )
  const verificationCall = vi.mocked(api.post).mock.calls[0]
  expect(JSON.stringify(verificationCall)).not.toContain('replacement-key')
  expect(verificationCall[1]).toMatchObject({
    scope: 'dns.config.update',
    context: {
      revision: initial.revision,
      change_hash: expect.stringMatching(/^[a-f0-9]{64}$/),
    },
  })
  expect(screen.getByLabelText('SecretKey')).toHaveValue('')
  expect(screen.getByLabelText('SecretId')).toHaveValue('')
  client.clear()
})

it('cancels verification and clears entered credentials without sending a configuration write', async () => {
  const client = renderConfiguration()
  const user = userEvent.setup()
  await user.type(await screen.findByLabelText('SecretKey'), 'discarded-key')
  const put = vi.spyOn(api, 'put')
  await user.click(
    screen.getByRole('button', { name: 'Save DNSPod configuration' })
  )
  await user.click(
    within(await screen.findByRole('alertdialog')).getByRole('button', {
      name: 'Continue',
    })
  )
  await user.click(
    within(await screen.findByRole('dialog')).getByRole('button', {
      name: 'Cancel',
    })
  )
  await waitFor(() =>
    expect(screen.getByLabelText('SecretKey')).toHaveValue('')
  )
  expect(put).not.toHaveBeenCalled()
  client.clear()
})

it('rejects invalid hostnames before confirmation and requires credentials when enabling an unconfigured installation', async () => {
  const client = renderConfiguration({
    ...initial,
    secret_id_set: false,
    secret_key_set: false,
  })
  const user = userEvent.setup()
  const hostname = await screen.findByRole('textbox', {
    name: 'DNSPod hostname',
  })
  await user.clear(hostname)
  await user.type(hostname, 'https://newapi')
  await user.click(
    screen.getByRole('button', { name: 'Save DNSPod configuration' })
  )
  await waitFor(() => expect(hostname).toHaveAttribute('aria-invalid', 'true'))
  expect(screen.getByLabelText('SecretKey')).toHaveAttribute(
    'aria-invalid',
    'true'
  )
  expect(screen.queryByRole('alertdialog')).not.toBeInTheDocument()
  client.clear()
})

it('keeps the form and refresh locked while saving and shows conflicts without reporting success', async () => {
  const client = renderConfiguration()
  const user = userEvent.setup()
  const hostname = await screen.findByRole('textbox', {
    name: 'DNSPod hostname',
  })
  await user.clear(hostname)
  await user.type(hostname, 'api')
  let resolveWrite!: (result: unknown) => void
  vi.spyOn(api, 'put').mockImplementation(
    () =>
      new Promise((resolve) => {
        resolveWrite = resolve
      })
  )
  await confirmConfiguration(user)
  await waitFor(() => expect(api.put).toHaveBeenCalled())
  expect(hostname).toBeDisabled()
  expect(
    screen.getByRole('button', { name: 'Refresh configuration' })
  ).toBeDisabled()
  resolveWrite({
    data: { success: false, code: 'DNS_CONFIG_CONFLICT', message: 'conflict' },
  })
  expect(await screen.findByRole('alert')).toHaveTextContent(
    'DNSPod configuration changed. Refresh and review it before saving again.'
  )
  expect(
    screen.queryByText(
      'DNSPod configuration saved. DNS records were not changed.'
    )
  ).not.toBeInTheDocument()
  expect(hostname).toBeEnabled()
  client.clear()
})

it('disables saving and explains the missing shared encryption key', async () => {
  const client = renderConfiguration({
    ...initial,
    can_save: false,
    blocked_reason: 'DNS_ENCRYPTION_KEY_REQUIRED',
  })
  expect(await screen.findByRole('alert')).toHaveTextContent(
    'Set the same stable CRYPTO_SECRET or SESSION_SECRET'
  )
  expect(
    screen.getByRole('button', { name: 'Save DNSPod configuration' })
  ).toBeDisabled()
  client.clear()
})

it('binds a non-ASCII DNS line and every credential field to the same canonical digest as the server', async () => {
  vi.stubGlobal('crypto', webcrypto)
  const update: DNSPodConfigUpdate = {
    revision: 'a'.repeat(64),
    enabled: true,
    domain: 'xunlitec.com',
    subdomain: 'newapi',
    record_line: '默认',
    record_type: 'A',
    secret_id: 'new-id',
    secret_key: 'new-key',
    session_token: '',
    clear_session_token: false,
  }
  expect(await dnsPodConfigChangeHash(update)).toBe(
    '3d5c9b5b3cfe37dbd1e8cc8ab7603e768d1ab99c3ac106a7de1172b8e83751e7'
  )
  expect(
    await dnsPodConfigChangeHash({ ...update, secret_key: 'changed-key' })
  ).not.toBe(await dnsPodConfigChangeHash(update))
})
