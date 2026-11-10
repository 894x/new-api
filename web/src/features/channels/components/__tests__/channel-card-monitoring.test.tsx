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
import { StrictMode, type ReactNode } from 'react'
import { afterEach, beforeEach, expect, test, vi } from 'vitest'

import type { ChannelMonitorSummary } from '@/features/channel-monitoring/api'
import { api } from '@/lib/api'

import { getDefaultMonitoringModel } from '../../lib/channel-utils'
import { channelSchema, type Channel } from '../../types'
import { ChannelCardMonitoring } from '../channel-card-monitoring'

let client: QueryClient
const originalAdapter = api.defaults.adapter

beforeEach(() => {
  client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
})

afterEach(() => {
  cleanup()
  client.clear()
  api.defaults.adapter = originalAdapter
  vi.restoreAllMocks()
})

function channel(overrides: Partial<Channel> = {}): Channel {
  return channelSchema.parse({
    id: 24,
    type: 1,
    key: '',
    status: 1,
    name: 'Example channel',
    models: 'model-a,model-b',
    created_time: 0,
    test_time: 0,
    response_time: 0,
    balance_updated_time: 0,
    ...overrides,
  })
}

function summary(id = 24, state = 'healthy'): ChannelMonitorSummary {
  return {
    channel_id: id,
    enabled: true,
    registered: true,
    state,
    latest: null,
    sample_count: 3,
    success_rate: 100,
    truncated: false,
    buckets: Array.from({ length: 96 }, (_, index) => ({
      timestamp: 1700000000 + index * 900,
      state,
      count: state === 'empty' ? 0 : 3,
      success_count: state === 'empty' ? 0 : 3,
      latency_ms: state === 'empty' ? null : 800,
    })),
  }
}

function panel(children: ReactNode) {
  return <QueryClientProvider client={client}>{children}</QueryClientProvider>
}

test.each([
  {
    name: 'first explicit monitoring target takes precedence over the test model',
    overrides: {
      monitor_config: JSON.stringify({
        targets: [{ model: ' model-b ' }, { model: 'model-a' }],
      }),
      test_model: 'model-a',
    },
    expected: 'model-b',
  },
  {
    name: 'test model takes precedence over the model list',
    overrides: { test_model: ' model-b ' },
    expected: 'model-b',
  },
  {
    name: 'an empty test model falls back to the first configured model',
    overrides: { test_model: ' ', models: ', model-a,model-b,' },
    expected: 'model-a',
  },
  {
    name: 'malformed persisted configuration safely falls back to the test model',
    overrides: { monitor_config: '{broken', test_model: 'model-b' },
    expected: 'model-b',
  },
  {
    name: 'an invalid target model safely falls back to the test model',
    overrides: {
      monitor_config: '{"targets":[{"model":12}]}',
      test_model: 'model-b',
    },
    expected: 'model-b',
  },
  {
    name: 'no configured model leaves monitoring disabled',
    overrides: { models: ' , ', test_model: null },
    expected: undefined,
  },
  {
    name: 'the API model length limit uses UTF-8 bytes',
    overrides: { test_model: '模'.repeat(43) },
    expected: undefined,
  },
])('$name', ({ overrides, expected }) => {
  expect(getDefaultMonitoringModel(channel(overrides))).toBe(expected)
})

test('cards with the same default model share a batch and show their own channel data', async () => {
  const request = vi.spyOn(api, 'get').mockResolvedValue({
    data: {
      success: true,
      data: [summary(24), { ...summary(25), success_rate: 75 }],
    },
  })
  render(
    panel(
      <>
        <ChannelCardMonitoring
          channel={channel()}
          channelIds={[25, 24]}
          sensitiveVisible
        />
        <ChannelCardMonitoring
          channel={channel({ id: 25, name: 'Second channel' })}
          channelIds={[24, 25]}
          sensitiveVisible
        />
      </>
    )
  )
  const timelines = await screen.findAllByRole('button', {
    name: /24-hour channel history for/,
  })
  expect(timelines).toHaveLength(2)
  expect(request).toHaveBeenCalledTimes(1)
  expect(request).toHaveBeenCalledWith('/api/channel/monitoring', {
    params: { model: 'model-a', channel_ids: '24,25' },
    signal: expect.any(AbortSignal),
    disableDuplicate: true,
  })
  expect(screen.getByLabelText('24h Success rate: 100%')).toBeVisible()
  expect(screen.getByLabelText('24h Success rate: 75%')).toBeVisible()
  expect(screen.queryByText('24 hours ago')).not.toBeInTheDocument()

  await userEvent.click(timelines[0])
  const dialog = await screen.findByRole('dialog', {
    name: 'Monitoring details',
  })
  expect(within(dialog).getByText('Sample count')).toBeVisible()
  expect(within(dialog).getAllByText('3').length).toBeGreaterThan(0)
})

test('strict-mode remounting loads monitoring instead of reusing an aborted HTTP request', async () => {
  api.defaults.adapter = async (config) => ({
    config,
    status: 200,
    statusText: 'OK',
    headers: {},
    data: { success: true, data: [summary()] },
  })
  render(
    <StrictMode>
      {panel(<ChannelCardMonitoring channel={channel()} sensitiveVisible />)}
    </StrictMode>
  )
  expect(
    await screen.findByRole('button', {
      name: '24-hour channel history for Example channel',
    })
  ).toBeVisible()
  expect(screen.queryByText('Monitoring unavailable')).not.toBeInTheDocument()
})

test('a card requests the first explicit monitoring model and updates when its default changes', async () => {
  const request = vi.spyOn(api, 'get').mockResolvedValue({
    data: { success: true, data: [summary()] },
  })
  const initial = channel({
    monitor_config: '{"targets":[{"model":"model-b"}]}',
  })
  const { rerender } = render(
    panel(<ChannelCardMonitoring channel={initial} sensitiveVisible />)
  )
  await screen.findByRole('button', { name: /24-hour channel history/ })
  expect(screen.getByText('model-b')).toBeVisible()
  expect(request).toHaveBeenLastCalledWith('/api/channel/monitoring', {
    params: { model: 'model-b', channel_ids: '24' },
    signal: expect.any(AbortSignal),
    disableDuplicate: true,
  })

  rerender(
    panel(<ChannelCardMonitoring channel={channel()} sensitiveVisible />)
  )
  await waitFor(() => expect(request).toHaveBeenCalledTimes(2))
  expect(screen.getByText('model-a')).toBeVisible()
  expect(screen.queryByText('model-b')).not.toBeInTheDocument()
  expect(request).toHaveBeenLastCalledWith('/api/channel/monitoring', {
    params: { model: 'model-a', channel_ids: '24' },
    signal: expect.any(AbortSignal),
    disableDuplicate: true,
  })
})

test('a card without a model explains the empty state and makes no monitoring request', () => {
  const request = vi.spyOn(api, 'get')
  render(
    panel(
      <ChannelCardMonitoring
        channel={channel({ models: '' })}
        sensitiveVisible
      />
    )
  )
  expect(screen.getByRole('status')).toHaveTextContent('No models available')
  expect(request).not.toHaveBeenCalled()
})

test('a pending monitoring request shows loading and a failure shows unavailable', async () => {
  let rejectRequest: (reason: Error) => void = () => undefined
  vi.spyOn(api, 'get').mockImplementation(
    () =>
      new Promise((_resolve, reject) => {
        rejectRequest = reject
      })
  )
  render(panel(<ChannelCardMonitoring channel={channel()} sensitiveVisible />))
  expect(screen.getByLabelText('Loading monitoring data')).toBeVisible()
  rejectRequest(new Error('Monitoring request failed'))
  expect(await screen.findByRole('status')).toHaveTextContent(
    'Monitoring unavailable'
  )
  expect(
    screen.queryByLabelText('Loading monitoring data')
  ).not.toBeInTheDocument()
})

test.each([
  ['disabled', 'Monitoring disabled'],
  ['unregistered', 'This model is not monitored'],
  ['empty', null],
])(
  'a card with %s monitoring preserves the shared monitoring state',
  async (state, label) => {
    vi.spyOn(api, 'get').mockResolvedValue({
      data: { success: true, data: [summary(24, state)] },
    })
    render(
      panel(<ChannelCardMonitoring channel={channel()} sensitiveVisible />)
    )
    if (label) {
      expect(await screen.findByText(label)).toBeVisible()
      expect(
        screen.queryByRole('button', { name: /24-hour channel history/ })
      ).not.toBeInTheDocument()
    } else {
      expect(
        await screen.findByRole('button', { name: /24-hour channel history/ })
      ).toBeVisible()
      expect(
        screen.getByRole('img', { name: 'No monitoring data' })
      ).toBeInTheDocument()
    }
  }
)

test('hiding sensitive information masks the model, channel name and monitoring dialog', async () => {
  vi.spyOn(api, 'get').mockResolvedValue({
    data: { success: true, data: [summary()] },
  })
  const { rerender } = render(
    panel(<ChannelCardMonitoring channel={channel()} sensitiveVisible />)
  )
  await screen.findByRole('button', {
    name: /24-hour channel history for Example channel/,
  })
  rerender(
    panel(
      <ChannelCardMonitoring channel={channel()} sensitiveVisible={false} />
    )
  )
  expect(screen.queryByText('model-a')).not.toBeInTheDocument()
  expect(screen.queryByTitle('model-a')).not.toBeInTheDocument()
  const timeline = screen.getByRole('button', {
    name: '24-hour channel history for ••••',
  })
  await userEvent.click(timeline)
  const dialog = await screen.findByRole('dialog', {
    name: 'Monitoring details',
  })
  expect(dialog).not.toHaveTextContent('Example channel')
  expect(dialog).not.toHaveTextContent('model-a')
})
