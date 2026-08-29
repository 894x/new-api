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
import { fireEvent, render, screen } from '@testing-library/react'
import { useEffect } from 'react'
import { afterEach, describe, expect, test } from 'vitest'

import { api } from '@/lib/api'

import type { Channel } from '../../../types'
import { ChannelsProvider, useChannels } from '../../channels-provider'
import { BalanceQueryDialog } from '../balance-query-dialog'

type ApiGet = (url: string) => Promise<{ data: unknown }>
type MockableApi = { get: ApiGet }

const apiClient = api as unknown as MockableApi
const originalGet = apiClient.get

const channel: Channel = {
  id: 42,
  type: 100,
  key: '',
  status: 1,
  name: 'Advanced Custom',
  created_time: 0,
  test_time: 0,
  response_time: 0,
  other: '',
  balance: 8,
  balance_updated_time: 0,
  models: '',
  group: 'default',
  used_quota: 0,
  other_info: '',
  remark: '',
  max_input_tokens: 0,
  channel_info: {
    is_multi_key: false,
    multi_key_size: 0,
    multi_key_polling_index: 0,
    multi_key_mode: 'random',
  },
  settings: '{}',
}

function BalanceQueryHarness() {
  const { currentRow, setCurrentRow } = useChannels()

  useEffect(() => {
    setCurrentRow(channel)
  }, [setCurrentRow])

  if (!currentRow) return null

  return <BalanceQueryDialog open onOpenChange={() => undefined} />
}

afterEach(() => {
  apiClient.get = originalGet
})

describe('BalanceQueryDialog safe response summary', () => {
  test('shows a response_summary as a safe structural summary after querying balance', async () => {
    const responseSummary = '{"response_type":"object","field_count":4}'
    apiClient.get = async (url) => {
      expect(url).toBe('/api/channel/update_balance/42')
      return {
        data: {
          success: true,
          response_summary: responseSummary,
        },
      }
    }
    const queryClient = new QueryClient({
      defaultOptions: { queries: { retry: false } },
    })

    render(
      <QueryClientProvider client={queryClient}>
        <ChannelsProvider>
          <BalanceQueryHarness />
        </ChannelsProvider>
      </QueryClientProvider>
    )

    fireEvent.click(
      await screen.findByRole('button', { name: 'Update Balance' })
    )

    const summary = await screen.findByRole('textbox', {
      name: 'Safe response summary',
    })
    expect(summary).toHaveTextContent(responseSummary)
    expect(
      screen.getByText(
        'The upstream response is valid JSON, but it does not match the OpenAI credit_summary format. Only a safe structural summary is shown; the channel balance was not updated.'
      )
    ).toBeVisible()
    expect(screen.queryByText('Upstream JSON response')).not.toBeInTheDocument()
    expect(screen.queryByText('Current Balance')).not.toBeInTheDocument()
  })
})
