import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'

import { api } from '@/lib/api'

import { UserRequestCapture } from '../user-request-capture'

vi.mock('@/lib/api', () => ({ api: { get: vi.fn(), put: vi.fn() } }))

function renderPolicy() {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  })
  return render(
    <QueryClientProvider client={client}>
      <UserRequestCapture userId={42} />
    </QueryClientProvider>
  )
}

describe('Per-user request capture', () => {
  it('loads disabled policy and enables only the selected user', async () => {
    vi.mocked(api.get).mockResolvedValue({
      data: { data: { enabled: false, available: true } },
    })
    vi.mocked(api.put).mockResolvedValue({ data: { success: true } })
    renderPolicy()
    const toggle = screen.getByRole('switch', {
      name: 'Save requests and responses',
    })
    await waitFor(() =>
      expect(toggle).not.toHaveAttribute('aria-disabled', 'true')
    )
    expect(toggle).not.toBeChecked()
    fireEvent.click(toggle)
    await waitFor(() => expect(toggle).toBeChecked())
    expect(api.put).toHaveBeenCalledWith('/api/user/42/request-capture', {
      enabled: true,
    })
  })

  it('keeps the saved state and displays failure when an update fails', async () => {
    vi.mocked(api.get).mockResolvedValue({
      data: { data: { enabled: true, available: true } },
    })
    vi.mocked(api.put).mockRejectedValue(new Error('unavailable'))
    renderPolicy()
    const toggle = screen.getByRole('switch')
    await waitFor(() => expect(toggle).toBeChecked())
    fireEvent.click(toggle)
    expect(await screen.findByRole('alert')).toHaveTextContent(
      'Failed to save capture settings'
    )
    expect(toggle).toBeChecked()
  })

  it('disables the toggle and offers retry when policy loading fails', async () => {
    vi.mocked(api.get).mockRejectedValue(new Error('unavailable'))
    renderPolicy()
    expect(await screen.findByRole('alert')).toHaveTextContent(
      'Failed to load capture settings'
    )
    expect(screen.getByRole('switch')).toHaveAttribute('aria-disabled', 'true')
    expect(screen.getByRole('button', { name: 'Retry' })).toBeEnabled()
  })
})
