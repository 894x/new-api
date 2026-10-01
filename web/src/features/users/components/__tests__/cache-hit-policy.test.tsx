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
import { beforeEach, describe, expect, it, vi } from 'vitest'

import { api } from '@/lib/api'
import { handleServerError } from '@/lib/handle-server-error'

import { UserCacheHitPolicyDialog } from '../user-cache-hit-policy-dialog'

vi.mock('@/lib/api', () => ({ api: { get: vi.fn(), put: vi.fn() } }))
vi.mock('@/lib/handle-server-error', () => ({ handleServerError: vi.fn() }))

const config = {
  revision: 'v1',
  policies: { 'policy-model': { enabled: true, min_bps: 4000, max_bps: 6000 } },
  daily: {
    day: '2026-10-01',
    input_tokens: 1000,
    real_cache_tokens: 900,
    bill_cache_tokens: 500,
  },
  statistics_error: '',
}

function renderDialog() {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  })
  return render(
    <QueryClientProvider client={client}>
      <UserCacheHitPolicyDialog
        userId={42}
        username='Test user'
        open
        onOpenChange={vi.fn()}
      />
    </QueryClientProvider>
  )
}

async function selectModel() {
  await screen.findByRole('combobox', { name: 'Model' })
  fireEvent.change(screen.getByRole('combobox', { name: 'Model' }), {
    target: { value: 'policy-model' },
  })
  await screen.findByRole('switch', { name: 'Enable cache billing policy' })
}

describe('cache hit policy configuration', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    vi.mocked(api.get).mockImplementation(async (url) => ({
      data: {
        success: true,
        data: url.includes('channel-routing')
          ? { models: ['policy-model'] }
          : config,
      },
    }))
    vi.mocked(api.put).mockResolvedValue({ data: { success: true } })
  })

  it('saves the selected model range and revision while showing real and billed daily usage', async () => {
    renderDialog()
    await selectModel()
    expect(screen.getByText('Real cache hit rate')).toBeVisible()
    expect(screen.getByText('90%')).toBeVisible()
    expect(screen.getByText('50%')).toBeVisible()
    expect(screen.getByRole('switch')).toBeChecked()
    fireEvent.change(
      screen.getByRole('spinbutton', { name: 'Random target minimum (%)' }),
      { target: { value: '20.25' } }
    )
    fireEvent.click(screen.getByRole('button', { name: 'Save' }))
    await waitFor(() =>
      expect(api.put).toHaveBeenCalledWith('/api/user/42/cache-hit-policy', {
        model: 'policy-model',
        revision: 'v1',
        enabled: true,
        min_bps: 2025,
        max_bps: 6000,
      })
    )
  })

  it('rejects an inverted range without submitting and keeps values after a failed save', async () => {
    renderDialog()
    await selectModel()
    fireEvent.change(
      screen.getByRole('spinbutton', { name: 'Random target minimum (%)' }),
      { target: { value: '80' } }
    )
    fireEvent.click(screen.getByRole('button', { name: 'Save' }))
    expect(
      await screen.findByText('Minimum must not exceed maximum')
    ).toBeVisible()
    expect(api.put).not.toHaveBeenCalled()
    const maximum = screen.getByRole('spinbutton', {
      name: 'Random target maximum (%)',
    })
    expect(maximum).toHaveAttribute('aria-invalid', 'true')
    fireEvent.change(maximum, { target: { value: '90' } })
    vi.mocked(api.put).mockRejectedValueOnce(new Error('revision conflict'))
    fireEvent.click(screen.getByRole('button', { name: 'Save' }))
    await waitFor(() => expect(handleServerError).toHaveBeenCalled())
    expect(maximum).toHaveValue(90)
    expect(screen.getByRole('button', { name: 'Save' })).toBeEnabled()
  })

  it('shows Redis fallback and hides misleading daily rates when statistics are unavailable', async () => {
    vi.mocked(api.get).mockResolvedValue({
      data: {
        success: true,
        data: { ...config, models: [], statistics_error: 'redis unavailable' },
      },
    })
    renderDialog()
    await selectModel()
    expect(
      screen.getByText(
        'Daily statistics unavailable; cache billing uses real usage when Redis is unavailable'
      )
    ).toBeVisible()
    expect(screen.queryByText('Real cache hit rate')).not.toBeInTheDocument()
  })
})
