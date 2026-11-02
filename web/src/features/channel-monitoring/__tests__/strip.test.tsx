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
import {
  cleanup,
  fireEvent,
  render,
  screen,
  within,
} from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, expect, test } from 'vitest'

import type { ChannelMonitorSummary } from '../api'
import { ChannelMonitoringStrip } from '../monitoring-strip'

afterEach(cleanup)

function fixture(): ChannelMonitorSummary {
  return {
    channel_id: 24,
    enabled: true,
    registered: true,
    state: 'healthy',
    sample_count: 3,
    success_rate: 100,
    truncated: false,
    latest: {
      id: 1,
      channel_id: 24,
      model: 'model-a',
      endpoint_type: 'openai',
      created_at: 1700000000,
      state: 'healthy',
      latency_ms: 800,
      ttft_ms: 0,
      tokens_per_second: null,
      status_code: 200,
      error_code: '',
    },
    buckets: Array.from({ length: 96 }, (_, index) => ({
      timestamp: 1700000000 + index * 900,
      state: index === 94 ? 'unavailable' : 'healthy',
      count: index === 94 ? 7 : 3,
      success_count: index === 94 ? 5 : 3,
      latency_ms: 800,
    })),
  }
}

test('a measured zero first-response time is preserved and missing generation speed is omitted', () => {
  render(
    <ChannelMonitoringStrip
      channelName='Example'
      summary={fixture()}
      loading={false}
      error={false}
    />
  )
  expect(screen.getByText('0 ms')).toBeInTheDocument()
  expect(screen.queryByText('Generation speed')).not.toBeInTheDocument()
  expect(screen.getByText('100%')).toBeInTheDocument()
})

test('keyboard selection opens the matching historical probe bucket', async () => {
  render(
    <ChannelMonitoringStrip
      channelName='Example'
      summary={fixture()}
      loading={false}
      error={false}
    />
  )
  const timeline = screen.getByRole('button', {
    name: '24-hour probe history for Example',
  })
  fireEvent.keyDown(timeline, { key: 'ArrowLeft' })
  await userEvent.click(timeline)
  const dialog = await screen.findByRole('dialog', { name: 'Probe details' })
  expect(within(dialog).getByText('Connection failed')).toBeInTheDocument()
  expect(within(dialog).getByText('7')).toBeInTheDocument()
  expect(within(dialog).getByText('5')).toBeInTheDocument()
})

test('a failed monitoring query is shown as unavailable instead of a healthy state', () => {
  render(<ChannelMonitoringStrip channelName='Example' loading={false} error />)
  expect(screen.getByRole('status')).toHaveTextContent('Monitoring unavailable')
  expect(screen.queryByText('Connected')).not.toBeInTheDocument()
})

test('an unprobed channel shows a pending state without invented latency or availability', () => {
  const summary = {
    ...fixture(),
    state: 'pending',
    latest: null,
    success_rate: null,
    sample_count: 0,
    buckets: fixture().buckets.map((bucket) => ({
      ...bucket,
      state: 'empty',
      count: 0,
      success_count: 0,
      latency_ms: null,
    })),
  }
  render(
    <ChannelMonitoringStrip
      channelName='Example'
      summary={summary}
      loading={false}
      error={false}
    />
  )
  expect(screen.getByText('Waiting for first probe')).toBeInTheDocument()
  expect(screen.queryByText('Connected')).not.toBeInTheDocument()
  expect(screen.queryByText(/\d ms/)).not.toBeInTheDocument()
  expect(screen.queryByText('100%')).not.toBeInTheDocument()
})

test('an excluded response retains its HTTP status without becoming a configuration error or performance sample', () => {
  const summary = fixture()
  summary.state = 'excluded'
  summary.success_rate = null
  if (!summary.latest) throw new Error('Fixture must contain the latest probe')
  summary.latest.state = 'excluded'
  summary.latest.status_code = 400
  render(
    <ChannelMonitoringStrip
      channelName='Example'
      summary={summary}
      loading={false}
      error={false}
    />
  )
  expect(screen.getByText('Excluded by monitoring policy')).toBeInTheDocument()
  expect(screen.getByText('HTTP 400')).toBeInTheDocument()
  expect(
    screen.queryByText('Monitoring configuration error')
  ).not.toBeInTheDocument()
  expect(screen.queryByText(/\d ms/)).not.toBeInTheDocument()
  expect(screen.queryByText('100%')).not.toBeInTheDocument()
})

test('initial monitoring loading exposes a placeholder without a connection result', () => {
  render(<ChannelMonitoringStrip channelName='Example' loading error={false} />)
  expect(screen.getByLabelText('Loading monitoring data')).toBeInTheDocument()
  expect(screen.queryByText('Connected')).not.toBeInTheDocument()
  expect(screen.queryByText('Monitoring unavailable')).not.toBeInTheDocument()
})

test('disabled monitoring hides the timeline and has no fabricated availability', () => {
  const summary = {
    ...fixture(),
    state: 'disabled',
    enabled: false,
    success_rate: null,
  }
  render(
    <ChannelMonitoringStrip
      channelName='Example'
      summary={summary}
      loading={false}
      error={false}
    />
  )
  expect(screen.getByText('Monitoring disabled')).toBeInTheDocument()
  expect(
    screen.queryByRole('button', { name: '24-hour probe history for Example' })
  ).not.toBeInTheDocument()
  expect(screen.queryByText('100%')).not.toBeInTheDocument()
})
