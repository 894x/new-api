import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'

import { api } from '@/lib/api'

import { RequestCapture } from '../request-capture'

vi.mock('@/lib/api', () => ({ api: { get: vi.fn() } }))

function renderCapture() {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  return render(
    <QueryClientProvider client={client}>
      <RequestCapture requestId='request-42' />
    </QueryClientProvider>
  )
}

describe('Captured request details', () => {
  it('loads only metadata until requested and clears body when hidden', async () => {
    vi.mocked(api.get)
      .mockResolvedValueOnce({ data: { data: { status: 'partial' } } })
      .mockResolvedValueOnce({
        data: {
          data: {
            status: 'partial',
            parts: [
              {
                stage: 'upstream_response',
                attempt: 2,
                bytes: 100,
                complete: false,
                truncated: true,
                redacted: false,
                omitted: false,
                body: 'captured body',
                tail: 'usage tail',
              },
            ],
          },
        },
      })
    renderCapture()
    const view = await screen.findByRole('button', {
      name: 'View captured content',
    })
    expect(api.get).toHaveBeenCalledTimes(1)
    expect(screen.queryByText('captured body')).not.toBeInTheDocument()
    fireEvent.click(view)
    await waitFor(() =>
      expect(screen.getByText('captured body')).toBeInTheDocument()
    )
    expect(screen.getByText('usage tail')).toBeInTheDocument()
    expect(screen.getByText('Incomplete · Truncated')).toBeInTheDocument()
    expect(api.get).toHaveBeenLastCalledWith(
      '/api/log/request-capture/request-42',
      { params: { body: true } }
    )
    fireEvent.click(screen.getByRole('button', { name: 'Hide' }))
    expect(screen.queryByText('captured body')).not.toBeInTheDocument()
  })

  it('shows expired evidence without offering a body download', async () => {
    vi.mocked(api.get).mockResolvedValue({
      data: { data: { status: 'expired' } },
    })
    renderCapture()
    expect(await screen.findByText('Capture expired')).toBeInTheDocument()
    expect(
      screen.queryByRole('button', { name: 'View captured content' })
    ).not.toBeInTheDocument()
  })

  it('shows a body loading failure and allows retry', async () => {
    vi.mocked(api.get)
      .mockResolvedValueOnce({ data: { data: { status: 'ready' } } })
      .mockRejectedValueOnce(new Error('offline'))
    renderCapture()
    fireEvent.click(
      await screen.findByRole('button', { name: 'View captured content' })
    )
    expect(await screen.findByRole('alert')).toHaveTextContent(
      'Failed to load capture'
    )
    expect(
      screen.getByRole('button', { name: 'View captured content' })
    ).toBeEnabled()
  })
})
