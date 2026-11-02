import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, expect, test, vi } from 'vitest'

import { api } from '@/lib/api'

import { LogExportSettings } from '../log-export-settings'

afterEach(() => vi.restoreAllMocks())

test('the administrator can save a valid export cap and an invalid cap never reaches the API', async () => {
  vi.spyOn(api, 'get').mockResolvedValue({
    data: {
      success: true,
      data: [{ key: 'LogExportMaxRows', value: '100000' }],
    },
  })
  const put = vi
    .spyOn(api, 'put')
    .mockResolvedValue({ data: { success: true } })
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  render(
    <QueryClientProvider client={client}>
      <LogExportSettings />
    </QueryClientProvider>
  )
  const input = await screen.findByRole('spinbutton', {
    name: 'Maximum records per export',
  })
  expect(input).toHaveValue(100000)
  await userEvent.clear(input)
  await userEvent.type(input, '10000001')
  await userEvent.click(screen.getByRole('button', { name: 'Save' }))
  await waitFor(() => expect(input).toHaveAttribute('aria-invalid', 'true'))
  expect(put).not.toHaveBeenCalled()
  await userEvent.clear(input)
  await userEvent.type(input, '10000000')
  await userEvent.click(screen.getByRole('button', { name: 'Save' }))
  await waitFor(() =>
    expect(put).toHaveBeenCalledWith('/api/option/', {
      key: 'LogExportMaxRows',
      value: '10000000',
    })
  )
})
