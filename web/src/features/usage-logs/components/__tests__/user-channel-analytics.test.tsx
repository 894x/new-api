import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { afterEach, beforeEach, expect, test, vi } from 'vitest'

import { api } from '@/lib/api'
import { useAuthStore } from '@/stores/auth-store'

import { UserChannelAnalytics } from '../user-channel-analytics'

const navigation = vi.hoisted(() => ({
  navigate: vi.fn(),
  search: {
    username: 'alice',
    startTime: 1793059200000,
    endTime: 1793145600000,
  },
}))

vi.mock('@tanstack/react-router', async (importOriginal) => ({
  ...(await importOriginal<typeof import('@tanstack/react-router')>()),
  getRouteApi: () => ({ useSearch: () => navigation.search }),
  useNavigate: () => navigation.navigate,
}))

beforeEach(() => {
  useAuthStore.setState({
    auth: {
      ...useAuthStore.getState().auth,
      user: { id: 1, username: 'admin', role: 100 },
    },
  })
  vi.spyOn(api, 'get').mockImplementation(async (url: string) => ({
    data: {
      success: true,
      data: url.includes('user_channel_stats')
        ? [
            {
              channel_id: 7,
              channel_name: 'Test channel',
              count: 1,
              prompt_tokens: 3,
              completion_tokens: 2,
              quota: 1,
              rpm: 1,
              tpm: 5,
              first_at: 1793059201,
              last_at: 1793059201,
            },
          ]
        : { items: [] },
    },
  }))
})

afterEach(() => {
  useAuthStore.setState(useAuthStore.getInitialState(), true)
})

test('opens channel details with the committed user and historical time window', async () => {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  render(
    <QueryClientProvider client={client}>
      <UserChannelAnalytics />
    </QueryClientProvider>
  )
  fireEvent.click(await screen.findByRole('button', { name: 'View details' }))
  await waitFor(() =>
    expect(navigation.navigate).toHaveBeenCalledWith({
      to: '/usage-logs/$section',
      params: { section: 'common' },
      search: { ...navigation.search, channel: '7' },
    })
  )
})
