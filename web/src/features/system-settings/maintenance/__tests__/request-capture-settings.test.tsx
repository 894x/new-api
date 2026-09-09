import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'

import { api } from '@/lib/api'

import { RequestCaptureSettings } from '../request-capture-settings'

vi.mock('@/lib/api', () => ({ api: { get: vi.fn(), put: vi.fn() } }))

function renderSettings(value = '{"retention_days":3,"max_gib":10}') {
  vi.mocked(api.get).mockResolvedValue({
    data: { success: true, data: [{ key: 'RequestCaptureStorage', value }] },
  })
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  })
  return render(
    <QueryClientProvider client={client}>
      <RequestCaptureSettings />
    </QueryClientProvider>
  )
}

describe('Request capture storage settings', () => {
  it('loads persisted values and saves both fields in one option', async () => {
    vi.mocked(api.put).mockResolvedValue({ data: { success: true } })
    renderSettings('{"retention_days":7,"max_gib":20}')
    const days = await screen.findByRole('spinbutton', {
      name: 'Capture retention (days)',
    })
    const capacity = screen.getByRole('spinbutton', {
      name: 'Capture capacity per instance (GiB)',
    })
    expect(days).toHaveValue(7)
    expect(capacity).toHaveValue(20)
    fireEvent.change(days, { target: { value: '14' } })
    fireEvent.change(capacity, { target: { value: '50' } })
    fireEvent.click(
      screen.getByRole('button', { name: 'Save capture settings' })
    )
    await waitFor(() =>
      expect(api.put).toHaveBeenCalledWith('/api/option/', {
        key: 'RequestCaptureStorage',
        value: '{"retention_days":14,"max_gib":50}',
      })
    )
    expect(api.put).toHaveBeenCalledTimes(1)
  })

  it.each(['', '0', '-1', '1.5', '366'])(
    'rejects invalid retention %s without sending settings',
    async (value) => {
      renderSettings()
      const days = await screen.findByRole('spinbutton', {
        name: 'Capture retention (days)',
      })
      fireEvent.change(days, { target: { value } })
      fireEvent.click(
        screen.getByRole('button', { name: 'Save capture settings' })
      )
      expect(screen.getByRole('alert')).toHaveTextContent(
        'Enter whole numbers between 1 and 365.'
      )
      expect(days).toHaveAttribute('aria-invalid', 'true')
      expect(api.put).not.toHaveBeenCalled()
    }
  )

  it('keeps edited values visible after a failed save', async () => {
    vi.mocked(api.put).mockResolvedValue({
      data: { success: false, message: 'failed' },
    })
    renderSettings()
    const capacity = await screen.findByRole('spinbutton', {
      name: 'Capture capacity per instance (GiB)',
    })
    fireEvent.change(capacity, { target: { value: '30' } })
    fireEvent.click(
      screen.getByRole('button', { name: 'Save capture settings' })
    )
    expect(await screen.findByRole('alert')).toHaveTextContent(
      'Failed to save capture settings'
    )
    expect(capacity).toHaveValue(30)
  })

  it('does not expose editable defaults for malformed persisted settings', async () => {
    renderSettings('{broken')
    expect(await screen.findByRole('alert')).toHaveTextContent(
      'Failed to load capture settings'
    )
    expect(screen.queryByRole('spinbutton')).not.toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Retry' })).toBeEnabled()
  })
})
