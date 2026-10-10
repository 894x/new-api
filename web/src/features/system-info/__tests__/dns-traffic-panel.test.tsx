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
  cleanup,
  render,
  screen,
  waitFor,
  within,
} from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { Toaster } from 'sonner'
import { afterEach, expect, it, vi } from 'vitest'

import { api } from '@/lib/api'

import { DNSTrafficPanel } from '../components/dns-traffic-panel'
import type { DNSTrafficState } from '../types'

const initial: DNSTrafficState = {
  configured: true,
  can_apply: true,
  domain: 'example.com',
  subdomain: 'api',
  record_line: '默认',
  record_type: 'A',
  revision: 'a'.repeat(64),
  records: [
    {
      id: '11',
      ip: '192.0.2.1',
      weight: 50,
      ttl: 600,
      enabled: true,
      remark: 'old',
      updated_on: '',
    },
    {
      id: '12',
      ip: '192.0.2.2',
      weight: 50,
      ttl: 600,
      enabled: true,
      remark: 'new',
      updated_on: '',
    },
    {
      id: '13',
      ip: '192.0.2.3',
      weight: 50,
      ttl: 600,
      enabled: false,
      remark: 'paused',
      updated_on: '',
    },
  ],
}

function renderPanel(state = initial) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  })
  vi.spyOn(api, 'get').mockImplementation(async (url) => {
    if (url === '/api/system-info/dns-traffic') {
      return { data: { success: true, data: state } }
    }
    if (url === '/api/system-info/dns-traffic/config') {
      return {
        data: {
          success: true,
          data: {
            revision: 'c'.repeat(64),
            enabled: true,
            domain: 'example.com',
            subdomain: 'api',
            record_line: '默认',
            record_type: 'A',
            secret_id_set: true,
            secret_key_set: true,
            session_token_set: false,
            source: 'database',
            can_save: true,
          },
        },
      }
    }
    return {
      data: {
        success: true,
        data: {
          scope: 'dns.traffic.update',
          methods: [{ method: 'password', available: true }],
          oauth_providers: [],
          password_encryption_enabled: false,
        },
      },
    }
  })
  render(
    <QueryClientProvider client={client}>
      <DNSTrafficPanel />
      <Toaster />
    </QueryClientProvider>
  )
  return client
}

async function enterAllocation() {
  const user = userEvent.setup()
  const oldWeight = await screen.findByRole('spinbutton', {
    name: 'DNS weight for 192.0.2.1',
  })
  await user.clear(oldWeight)
  await user.type(oldWeight, '70')
  const newWeight = screen.getByRole('spinbutton', {
    name: 'DNS weight for 192.0.2.2',
  })
  await user.clear(newWeight)
  await user.type(newWeight, '30')
  return user
}

async function confirmAndVerify(user: ReturnType<typeof userEvent.setup>) {
  await user.click(screen.getByRole('button', { name: 'Apply DNS weights' }))
  const confirmation = await screen.findByRole('alertdialog')
  await user.click(
    within(confirmation).getByRole('button', { name: 'Confirm DNS changes' })
  )
  const password = await screen.findByLabelText('Password', {
    selector: 'input',
  })
  await user.type(password, 'verification-password')
  await user.click(screen.getByRole('button', { name: 'Verify' }))
}

afterEach(() => {
  cleanup()
  vi.restoreAllMocks()
  localStorage.clear()
  sessionStorage.clear()
})

it('previews the allocation and applies only enabled weights after confirmation and verification', async () => {
  const client = renderPanel()
  vi.spyOn(api, 'post').mockResolvedValue({
    data: {
      success: true,
      data: {
        scope: 'dns.traffic.update',
        method: 'password',
        proof_token: 'one-use-proof',
        expires_at: Math.floor(Date.now() / 1000) + 300,
      },
    },
  })
  const saved = {
    ...initial,
    revision: 'b'.repeat(64),
    records: initial.records.map((record) => {
      if (record.id === '11') return { ...record, weight: 70 }
      if (record.id === '12') return { ...record, weight: 30 }
      return record
    }),
  }
  const put = vi.spyOn(api, 'put').mockImplementation(
    () =>
      new Promise((resolve) => {
        finishUpdate = () => resolve({ data: { success: true, data: saved } })
      })
  )
  let finishUpdate: () => void = () => {
    throw new Error('DNS update did not start')
  }
  const user = await enterAllocation()
  expect(screen.getByText('Planned DNS share: 70%')).toBeVisible()
  expect(
    screen.queryByRole('spinbutton', { name: /192.0.2.3/ })
  ).not.toBeInTheDocument()
  expect(put).not.toHaveBeenCalled()
  await confirmAndVerify(user)
  await waitFor(() =>
    expect(put).toHaveBeenCalledExactlyOnceWith(
      '/api/system-info/dns-traffic',
      {
        revision: initial.revision,
        weights: [
          { id: '11', weight: 70 },
          { id: '12', weight: 30 },
        ],
      },
      expect.objectContaining({
        headers: { 'X-Security-Proof': 'one-use-proof' },
      })
    )
  )
  expect(
    screen.getByRole('button', { name: 'Refresh DNS records' })
  ).toBeDisabled()
  expect(
    screen.getByRole('spinbutton', { name: 'DNS weight for 192.0.2.1' })
  ).toBeDisabled()
  finishUpdate()
  expect(
    await screen.findByText(
      'DNS weights applied and verified. DNS caches may still use the previous allocation.'
    )
  ).toBeVisible()
  expect(
    screen.getByRole('button', { name: 'Apply DNS weights' })
  ).toBeDisabled()
  client.clear()
})

it.each(['0', '101', '1.5', ''])(
  'rejects invalid weight %s before confirmation or a DNS write',
  async (value) => {
    const client = renderPanel()
    const put = vi.spyOn(api, 'put')
    const user = userEvent.setup()
    const input = await screen.findByRole('spinbutton', {
      name: 'DNS weight for 192.0.2.1',
    })
    await user.clear(input)
    if (value) await user.type(input, value)
    await user.click(screen.getByRole('button', { name: 'Apply DNS weights' }))
    expect(
      await screen.findByText(
        'Enter an integer weight from 1 to 100 for each enabled IP.'
      )
    ).toBeVisible()
    expect(input).toHaveAttribute('aria-invalid', 'true')
    expect(screen.queryByRole('alertdialog')).not.toBeInTheDocument()
    expect(put).not.toHaveBeenCalled()
    client.clear()
  }
)

it('keeps provider failure visible and does not report HTTP 200 business failure as success', async () => {
  const client = renderPanel()
  vi.spyOn(api, 'post').mockResolvedValue({
    data: {
      success: true,
      data: {
        scope: 'dns.traffic.update',
        method: 'password',
        proof_token: 'proof',
        expires_at: Math.floor(Date.now() / 1000) + 300,
      },
    },
  })
  const put = vi.spyOn(api, 'put').mockResolvedValue({
    data: {
      success: false,
      code: 'DNS_VERIFY_FAILED',
      message: 'verification failed',
    },
  })
  const user = await enterAllocation()
  await confirmAndVerify(user)
  expect(await screen.findByRole('alert')).toHaveTextContent(
    'some records may have changed'
  )
  expect(
    screen.queryByText(
      'DNS weights applied and verified. DNS caches may still use the previous allocation.'
    )
  ).not.toBeInTheDocument()
  expect(put).toHaveBeenCalledTimes(1)
  client.clear()
})

it('cancels verification without sending a DNS update', async () => {
  const client = renderPanel()
  const put = vi.spyOn(api, 'put')
  const user = await enterAllocation()
  await user.click(screen.getByRole('button', { name: 'Apply DNS weights' }))
  await user.click(
    within(await screen.findByRole('alertdialog')).getByRole('button', {
      name: 'Confirm DNS changes',
    })
  )
  const verification = await screen.findByRole('dialog')
  await user.click(within(verification).getByRole('button', { name: 'Cancel' }))
  await waitFor(() =>
    expect(
      screen.getByRole('button', { name: 'Apply DNS weights' })
    ).toBeEnabled()
  )
  expect(put).not.toHaveBeenCalled()
  client.clear()
})

it('shows missing configuration and disables applying an insufficient record set', async () => {
  const client = renderPanel({
    ...initial,
    configured: false,
    can_apply: false,
    records: [],
  })
  expect(
    await screen.findByText(/Configure and enable DNSPod below/)
  ).toBeVisible()
  expect(
    screen.queryByRole('button', { name: 'Apply DNS weights' })
  ).not.toBeInTheDocument()
  client.setQueryData(['system-info', 'dns-traffic'], {
    ...initial,
    can_apply: false,
    blocked_reason: 'DNS_INSTANCE_COUNT_INVALID',
    records: [initial.records[0]],
  })
  expect(await screen.findByRole('status')).toHaveTextContent(
    'Configure 2 to 16 enabled records'
  )
  expect(screen.getByRole('spinbutton')).toBeDisabled()
  expect(
    screen.getByRole('button', { name: 'Apply DNS weights' })
  ).toBeDisabled()
  client.clear()
})
