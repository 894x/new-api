import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import type { ReactNode } from 'react'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import { api } from '@/lib/api'
import { useAuthStore } from '@/stores/auth-store'

import type { User } from '../../types'
import {
  UserRequestCapture,
  UserRequestCaptureCell,
} from '../user-request-capture'

vi.mock('@/lib/api', () => ({ api: { get: vi.fn(), put: vi.fn() } }))

function renderWithQueryClient(children: ReactNode) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  })
  return render(
    <QueryClientProvider client={client}>{children}</QueryClientProvider>
  )
}

function renderPolicy() {
  return renderWithQueryClient(<UserRequestCapture userId={42} />)
}

const listedUser: User = {
  id: 42,
  username: 'capture-user',
  display_name: 'Capture User',
  quota: 0,
  used_quota: 0,
  request_count: 0,
  group: 'default',
  status: 1,
  role: 1,
}

describe('Per-user request capture', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    useAuthStore.getState().auth.setUser({ id: 1, username: 'root', role: 100 })
  })

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

  it('provides the same policy toggle directly in the user list', async () => {
    vi.mocked(api.get).mockResolvedValue({
      data: { data: { enabled: false, available: true } },
    })
    vi.mocked(api.put).mockResolvedValue({ data: { success: true } })

    renderWithQueryClient(<UserRequestCaptureCell user={listedUser} />)

    const toggle = screen.getByRole('switch', {
      name: 'Save requests and responses: capture-user',
    })
    await waitFor(() =>
      expect(toggle).not.toHaveAttribute('aria-disabled', 'true')
    )
    fireEvent.click(toggle)

    await waitFor(() => expect(toggle).toBeChecked())
    expect(api.put).toHaveBeenCalledWith('/api/user/42/request-capture', {
      enabled: true,
    })
  })

  it('does not load a policy that the current administrator cannot change', () => {
    useAuthStore.getState().auth.setUser({ id: 2, username: 'admin', role: 10 })

    renderWithQueryClient(
      <UserRequestCaptureCell user={{ ...listedUser, role: 10 }} />
    )

    expect(screen.queryByRole('switch')).not.toBeInTheDocument()
    expect(api.get).not.toHaveBeenCalled()
  })
})
