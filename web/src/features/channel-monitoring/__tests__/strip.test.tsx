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
      state_counts:
        index === 94
          ? { healthy: 4, slow: 1, unavailable: 2 }
          : { healthy: 3, slow: 0, unavailable: 0 },
      latency_ms: 800,
      performance: {
        duration_p50_ms: 800,
        ttft_p50_ms: 0,
        duration_count: 3,
        ttft_count: 3,
      },
    })),
  }
}

test('compact channel history hides the timeline footer and keeps its explanation in the tooltip', async () => {
  const props = {
    channelName: 'Example',
    loading: false,
    error: false,
    summary: fixture(),
  }
  const { rerender } = render(<ChannelMonitoringStrip {...props} />)
  expect(screen.getByText('24 hours ago')).toBeVisible()
  rerender(<ChannelMonitoringStrip {...props} showTimelineLabels={false} />)
  expect(screen.queryByText('24 hours ago')).not.toBeInTheDocument()
  expect(
    screen.queryByText('Bar height shows sample volume')
  ).not.toBeInTheDocument()
  expect(screen.queryByText('Now')).not.toBeInTheDocument()
  const timeline = screen.getByRole('button', {
    name: '24-hour channel history for Example',
  })
  await userEvent.hover(timeline)
  const tooltip = await screen.findByRole('tooltip')
  expect(
    within(tooltip).getByText('Bar height shows sample volume')
  ).toBeVisible()
})

test('a measured zero first-response time is preserved and missing generation speed is omitted', () => {
  render(
    <ChannelMonitoringStrip
      channelName='Example'
      summary={fixture()}
      loading={false}
      error={false}
    />
  )
  expect(screen.getByLabelText('P50 first response: ≈ 0 ms')).toHaveTextContent(
    'P50 ≈ 0 ms'
  )
  expect(screen.queryByText('Generation speed')).not.toBeInTheDocument()
  expect(screen.getByText('100%')).toBeInTheDocument()
  expect(screen.getByRole('img', { name: 'Connected' })).toBeInTheDocument()
  expect(screen.queryByText('First response')).not.toBeInTheDocument()
  expect(screen.queryByText('Success rate')).not.toBeInTheDocument()
})

test('cache hit rate follows generation speed and precedes success rate without a visible label', () => {
  const summary = fixture()
  if (summary.latest) summary.latest.tokens_per_second = 43.29
  summary.cache = { hit_rate: 82.46, truncated: false }
  render(
    <ChannelMonitoringStrip
      channelName='Example'
      summary={summary}
      loading={false}
      error={false}
    />
  )
  const speed = screen.getByLabelText('Latest generation speed: 43.29 t/s')
  const cache = screen.getByLabelText('Cache hit rate: 82.46%')
  const success = screen.getByLabelText('24h Success rate: 100%')
  expect(
    speed.compareDocumentPosition(cache) & Node.DOCUMENT_POSITION_FOLLOWING
  ).toBeTruthy()
  expect(
    cache.compareDocumentPosition(success) & Node.DOCUMENT_POSITION_FOLLOWING
  ).toBeTruthy()
  expect(cache.textContent).toBe('82.46%')
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
    name: '24-hour channel history for Example',
  })
  fireEvent.keyDown(timeline, { key: 'ArrowLeft' })
  await userEvent.click(timeline)
  const dialog = await screen.findByRole('dialog', {
    name: 'Monitoring details',
  })
  expect(
    within(dialog).getByText('Connection failed', { selector: 'dd' })
  ).toBeInTheDocument()
  expect(within(dialog).getByText('7')).toBeInTheDocument()
  expect(within(dialog).getByText('5')).toBeInTheDocument()
  expect(
    within(dialog).getByText('Latest successful request duration')
  ).toBeInTheDocument()
})

test('bar heights encode sample volume on a shared bounded scale while empty intervals stay flat', () => {
  const summary = fixture()
  summary.buckets[0] = { ...summary.buckets[0], count: 0, state: 'empty' }
  summary.buckets[1].count = 1
  summary.buckets[2].count = 100
  summary.buckets[3].count = 1000
  render(
    <ChannelMonitoringStrip
      channelName='Example'
      summary={summary}
      loading={false}
      error={false}
      maxSampleCount={1000}
    />
  )
  const timeline = screen.getByRole('button', {
    name: '24-hour channel history for Example',
  })
  const bars = timeline.querySelectorAll<HTMLElement>('[data-bucket]')
  const heights = [...bars]
    .slice(0, 4)
    .map((bar) => Number.parseFloat(bar.style.height))
  expect(heights[0]).toBe(2)
  expect(bars[0].querySelector('[data-state]')).toBeNull()
  expect(heights[1]).toBeGreaterThan(heights[0])
  expect(heights[2]).toBeGreaterThan(heights[1])
  expect(heights[3]).toBeGreaterThan(heights[2])
  expect(heights[3]).toBe(12)
  expect(timeline).toHaveAccessibleDescription('Bar height shows sample volume')
})

test('a mixed interval stacks normal, slow, failed and excluded samples by their actual proportions', () => {
  const summary = fixture()
  summary.buckets[95] = {
    ...summary.buckets[95],
    count: 10,
    success_count: 8,
    state_counts: { healthy: 6, slow: 2, unavailable: 1, excluded: 1 },
  }
  render(
    <ChannelMonitoringStrip
      channelName='Example'
      summary={summary}
      loading={false}
      error={false}
    />
  )
  const timeline = screen.getByRole('button', {
    name: '24-hour channel history for Example',
  })
  const column = timeline.querySelector('[data-bucket="95"]')
  expect(
    column?.querySelector<HTMLElement>('[data-state="healthy"]')?.style.height
  ).toBe('60%')
  expect(
    column?.querySelector<HTMLElement>('[data-state="slow"]')?.style.height
  ).toBe('20%')
  expect(
    column?.querySelector<HTMLElement>('[data-state="unavailable"]')?.style
      .height
  ).toBe('10%')
  expect(
    column?.querySelector<HTMLElement>('[data-state="excluded"]')?.style.height
  ).toBe('10%')
})

test('history hover shows only the interval and short outcome counts', async () => {
  const summary = fixture()
  summary.buckets[95] = {
    ...summary.buckets[95],
    count: 31,
    success_count: 28,
    state_counts: { healthy: 21, slow: 7, unavailable: 3 },
  }
  render(
    <ChannelMonitoringStrip
      channelName='Example'
      summary={summary}
      loading={false}
      error={false}
    />
  )
  await userEvent.hover(
    screen.getByRole('button', { name: '24-hour channel history for Example' })
  )
  const tooltip = await screen.findByRole('tooltip')
  expect(tooltip).toHaveTextContent('Healthy 21')
  expect(tooltip).toHaveTextContent('Slow 7')
  expect(tooltip).toHaveTextContent('Failed 3')
  expect(tooltip).not.toHaveTextContent('probes succeeded')
  expect(tooltip).not.toHaveTextContent('Connected')
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
      performance: null,
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
  expect(
    screen.getByRole('img', { name: 'Waiting for first observation' })
  ).toBeInTheDocument()
  expect(screen.queryByText('Connected')).not.toBeInTheDocument()
  expect(screen.queryByText(/\d ms/)).not.toBeInTheDocument()
  expect(screen.queryByText('100%')).not.toBeInTheDocument()
})

test('an excluded response retains its HTTP status without becoming a configuration error or performance sample', () => {
  const summary = fixture()
  summary.state = 'excluded'
  summary.success_rate = null
  summary.buckets[95].performance = null
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
  expect(
    screen.getByRole('img', { name: 'Excluded by monitoring policy' })
  ).toBeInTheDocument()
  expect(screen.getByText('HTTP 400')).toBeInTheDocument()
  expect(
    screen.queryByText('Monitoring configuration error')
  ).not.toBeInTheDocument()
  expect(screen.queryByText(/\d ms/)).not.toBeInTheDocument()
  expect(screen.queryByText('100%')).not.toBeInTheDocument()
})

test('current-interval P50 uses aggregate data rather than the latest request or the selected historical interval', () => {
  const summary = fixture()
  summary.buckets[95].performance = {
    duration_p50_ms: 2000,
    ttft_p50_ms: 1234,
    duration_count: 20,
    ttft_count: 17,
  }
  render(
    <ChannelMonitoringStrip
      channelName='Example'
      summary={summary}
      loading={false}
      error={false}
    />
  )
  const metric = screen.getByLabelText('P50 first response: ≈ 1,234 ms')
  expect(metric).toHaveAttribute(
    'title',
    expect.stringContaining('Successful samples: 17')
  )
  expect(metric).toHaveAttribute(
    'title',
    expect.stringContaining('Current 15-minute interval:')
  )
  fireEvent.keyDown(
    screen.getByRole('button', { name: '24-hour channel history for Example' }),
    { key: 'ArrowLeft' }
  )
  expect(metric).toHaveTextContent('1,234 ms')
})

test('duration P50 is used when no valid TTFT samples exist', () => {
  const summary = fixture()
  summary.buckets[95].performance = {
    duration_p50_ms: 800,
    ttft_p50_ms: null,
    duration_count: 3,
    ttft_count: 0,
  }
  render(
    <ChannelMonitoringStrip
      channelName='Example'
      summary={summary}
      loading={false}
      error={false}
    />
  )
  expect(
    screen.getByLabelText('P50 request duration: ≈ 800 ms')
  ).toHaveAttribute('title', expect.stringContaining('Successful samples: 3'))
})

test('missing current-interval distributions do not fall back to an older interval or a last-request value', () => {
  const summary = fixture()
  summary.buckets[95].performance = null
  render(
    <ChannelMonitoringStrip
      channelName='Example'
      summary={summary}
      loading={false}
      error={false}
    />
  )
  expect(screen.getByLabelText('P50 request duration: —')).toHaveAttribute(
    'title',
    expect.stringContaining('No complete performance data for this interval')
  )
  expect(screen.queryByText(/≈/)).not.toBeInTheDocument()
})

test.each([
  [4999, 'Connected'],
  [5000, 'Slow response'],
  [9999, 'Slow response'],
  [10000, 'Very slow response'],
])(
  'a P50 of %i ms uses the %s state at the shared 5 s and 10 s boundaries',
  (latency, state) => {
    const summary = fixture()
    summary.buckets[95].performance = {
      duration_p50_ms: latency,
      ttft_p50_ms: latency,
      duration_count: 3,
      ttft_count: 3,
    }
    render(
      <ChannelMonitoringStrip
        channelName='Example'
        summary={summary}
        loading={false}
        error={false}
      />
    )
    expect(screen.getByRole('img', { name: state })).toBeInTheDocument()
  }
)

test('a failed latest request remains red even when the current interval has successful P50 samples', () => {
  const summary = fixture()
  summary.state = 'unavailable'
  if (!summary.latest) {
    throw new Error('Fixture must contain the latest observation')
  }
  summary.latest = { ...summary.latest, state: 'unavailable', status_code: 503 }
  summary.buckets[95].state_counts = {
    healthy: 1,
    very_slow: 1,
    unavailable: 1,
  }
  render(
    <ChannelMonitoringStrip
      channelName='Example'
      summary={summary}
      loading={false}
      error={false}
    />
  )
  expect(
    screen.getByRole('img', { name: 'Connection failed' })
  ).toBeInTheDocument()
  expect(screen.getByText('HTTP 503')).toBeInTheDocument()
  expect(
    screen.getByLabelText('P50 first response: ≈ 0 ms')
  ).toBeInTheDocument()
  const timeline = screen.getByRole('button', {
    name: '24-hour channel history for Example',
  })
  const column = timeline.querySelector('[data-bucket="95"]')
  expect(column?.querySelector('[data-state="healthy"]')).toHaveClass(
    'bg-emerald-500/80'
  )
  expect(column?.querySelector('[data-state="very_slow"]')).toHaveClass(
    'bg-orange-500/90'
  )
  expect(column?.querySelector('[data-state="unavailable"]')).toHaveClass(
    'bg-rose-500/80'
  )
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
    screen.queryByRole('button', {
      name: '24-hour channel history for Example',
    })
  ).not.toBeInTheDocument()
  expect(screen.queryByText('100%')).not.toBeInTheDocument()
})

test('combined source counts appear in details while the metric line and hover remain compact', async () => {
  const summary = fixture()
  summary.buckets[95].source_counts = { probe: 1, traffic: 2 }
  render(
    <ChannelMonitoringStrip
      channelName='Example'
      summary={summary}
      loading={false}
      error={false}
    />
  )
  expect(screen.queryByText('Active probes')).not.toBeInTheDocument()
  const timeline = screen.getByRole('button', {
    name: '24-hour channel history for Example',
  })
  await userEvent.hover(timeline)
  expect(await screen.findByRole('tooltip')).not.toHaveTextContent(
    'Live traffic'
  )
  await userEvent.click(timeline)
  const dialog = await screen.findByRole('dialog', {
    name: 'Monitoring details',
  })
  expect(
    within(dialog).getByText('Active probes').nextElementSibling
  ).toHaveTextContent('1')
  expect(
    within(dialog).getByText('Live traffic').nextElementSibling
  ).toHaveTextContent('2')
})
