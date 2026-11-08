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
  fireEvent,
  render,
  screen,
  waitFor,
  within,
} from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, expect, test, vi } from 'vitest'

import { api } from '@/lib/api'

import EarlyChunkMonitor from '../early-chunk-monitor'
import type {
  EarlyChunkGrid,
  EarlyChunkMetrics,
} from '../early-chunk-monitor-api'

const clients: QueryClient[] = []
afterEach(() => clients.splice(0).forEach((client) => client.clear()))

function fixture(): EarlyChunkGrid {
  const metrics: EarlyChunkMetrics = {
    samples: 24,
    content_samples: 23,
    completed: 22,
    failed: 1,
    cancelled: 1,
    average_ms: 5400,
    p50_ms: 5500,
    p90_ms: 5750,
    p95_ms: 6000,
    overflow: 0,
    decisions: 10,
    sent: 8,
    met: 9,
    sent_met: 7,
    sent_failed: 1,
    average_gap_ms: 420,
    updated_at_ms: 1794096000000,
  }
  return {
    model: 'k3',
    channel_id: 49,
    channel_name: 'Primary',
    scope: 'model_channel',
    backend: 'memory',
    token_bands: [
      { index: 0, min: 0, max: 1024 },
      { index: 53, min: 0, unknown: true },
    ],
    cache_bands: [
      { index: 0, min: 0, max: 1000 },
      { index: 20, min: 0, unknown: true },
    ],
    latency_bounds_ms: [250, 500, 750],
    regular_buckets: 1040,
    tracked_buckets: 1,
    max_buckets: 4096,
    payload_bytes: 1500,
    node_read_errors: 0,
    node_write_errors: 0,
    node_evictions: 0,
    probe_ms: 5000,
    config: {
      mode: 'off',
      lead_ratio: 0.15,
      min_lead_ms: 1000,
      max_lead_ms: 3000,
      earliest_ms: 1000,
      min_samples: 20,
      min_probability: 0.95,
      max_age_seconds: 900,
    },
    summary: metrics,
    cells: [
      {
        id: 0,
        token_band: 0,
        cache_band: 0,
        metrics,
        prediction: {
          pending: 20,
          soon: 19,
          probability: 0.95,
          merged_cells: 2,
          eligible: true,
          at_ms: 5000,
          estimated_ttft_ms: 6000,
          remaining_p50_ms: 1000,
          remaining_p90_ms: 1250,
          lead_ms: 1000,
          candidate_ms: 5000,
          reason: 'eligible',
        },
        histogram: [0, 0, 23],
      },
    ],
    generated_at_ms: 1794096000000,
  }
}

function mount() {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  clients.push(client)
  return render(
    <QueryClientProvider client={client}>
      <EarlyChunkMonitor
        model='k3'
        channels={[{ channel_id: 49, channel_name: 'Primary' }]}
      />
    </QueryClientProvider>
  )
}

test('populated grid cells expose quantiles, failures and conditional prediction through keyboard selection', async () => {
  vi.spyOn(api, 'get').mockResolvedValue({
    data: { success: true, data: fixture() },
  })
  const user = userEvent.setup()
  mount()
  const cell = await screen.findByRole('button', { name: '0–1k / 0–10%' })
  expect(screen.getByText(/pool and channel mode still applies/i)).toBeVisible()
  expect(within(cell).getByText('p90 5.75s')).toBeVisible()
  expect(within(cell).getByText('Advance 1s')).toBeVisible()
  expect(within(cell).getByText('Candidate 5s')).toBeVisible()
  cell.focus()
  await user.keyboard('{Enter}')
  expect(cell).toHaveAttribute('aria-pressed', 'true')
  const details = screen.getByRole('region', { name: 'Bucket details' })
  expect(within(details).getByText('Average TTFT')).toBeVisible()
  expect(within(details).getByText('Estimated first content')).toBeVisible()
  expect(within(details).getByText('Remaining p90')).toBeVisible()
  expect(within(details).getByText('1.25 s')).toBeVisible()
  expect(within(details).getByText('Failed after pre-emission')).toBeVisible()
  expect(within(details).getByText(/20 pending samples/)).toBeVisible()
  await user.keyboard('{Escape}')
  await waitFor(() =>
    expect(screen.queryByRole('dialog')).not.toBeInTheDocument()
  )
  expect(cell).toHaveFocus()
})

test('empty observations hide the grid and explain collection while off', async () => {
  const data = fixture()
  data.cells = []
  vi.spyOn(api, 'get').mockResolvedValue({ data: { success: true, data } })
  mount()
  expect(await screen.findByRole('status')).toHaveTextContent(
    'Off mode also collects samples'
  )
  expect(screen.queryByRole('table')).not.toBeInTheDocument()
  expect(
    screen.queryByRole('region', { name: 'Input tokens by prefix reuse ratio' })
  ).not.toBeInTheDocument()
  expect(
    screen.queryByRole('button', { name: '0–1k / 0–10%' })
  ).not.toBeInTheDocument()
})

test('only axes with samples remain visible while holes between populated axes stay empty', async () => {
  const data = fixture()
  data.token_bands.splice(
    1,
    0,
    { index: 1, min: 1024, max: 2048 },
    { index: 2, min: 2048, max: 3072 }
  )
  data.cache_bands.splice(
    1,
    0,
    { index: 1, min: 1000, max: 2000 },
    { index: 2, min: 2000, max: 3000 }
  )
  data.cells.push(
    { ...data.cells[0], id: 44, token_band: 2, cache_band: 2 },
    {
      ...data.cells[0],
      id: 22,
      token_band: 1,
      cache_band: 1,
      metrics: { ...data.summary, samples: 0, content_samples: 0 },
    }
  )
  vi.spyOn(api, 'get').mockResolvedValue({ data: { success: true, data } })
  mount()
  const table = await screen.findByRole('table')
  expect(
    within(table)
      .getAllByRole('columnheader')
      .map((header) => header.textContent)
  ).toEqual(['Input tokens', '0–10%', '20–30%'])
  expect(
    within(table)
      .getAllByRole('rowheader')
      .map((header) => header.textContent)
  ).toEqual(['0–1k', '2–3k'])
  expect(
    within(table).getByRole('cell', { name: '0–1k / 20–30%: No Data' })
  ).toHaveTextContent('—')
  expect(
    within(table).getByRole('cell', { name: '2–3k / 0–10%: No Data' })
  ).toHaveTextContent('—')
  await userEvent.click(
    within(table).getByRole('button', { name: '2–3k / 20–30%' })
  )
  expect(
    screen.getByRole('region', { name: 'Bucket details' })
  ).toHaveTextContent('2–3k / 20–30%')
})

test('refresh removes expired axes and keeps diagnostic cells that only have failures', async () => {
  const first = fixture()
  const second = fixture()
  second.cells = [
    {
      ...second.cells[0],
      id: 1133,
      token_band: 53,
      cache_band: 20,
      metrics: {
        ...second.summary,
        samples: 1,
        content_samples: 0,
        completed: 0,
        failed: 1,
        cancelled: 0,
        p50_ms: null,
        p90_ms: null,
        p95_ms: null,
      },
    },
  ]
  vi.spyOn(api, 'get')
    .mockResolvedValueOnce({ data: { success: true, data: first } })
    .mockResolvedValue({ data: { success: true, data: second } })
  mount()
  await screen.findByRole('button', { name: '0–1k / 0–10%' })
  fireEvent.click(screen.getByRole('button', { name: 'Refresh' }))
  expect(
    await screen.findByRole('button', { name: 'Unknown / Unknown' })
  ).toBeVisible()
  const table = screen.getByRole('table')
  expect(
    within(table)
      .getAllByRole('columnheader')
      .map((header) => header.textContent)
  ).toEqual(['Input tokens', 'Unknown'])
  expect(
    within(table)
      .getAllByRole('rowheader')
      .map((header) => header.textContent)
  ).toEqual(['Unknown'])
  expect(
    screen.queryByRole('button', { name: '0–1k / 0–10%' })
  ).not.toBeInTheDocument()
})

test('elapsed-time edits only request new predictions after valid apply', async () => {
  const get = vi
    .spyOn(api, 'get')
    .mockResolvedValue({ data: { success: true, data: fixture() } })
  mount()
  await screen.findByRole('button', { name: '0–1k / 0–10%' })
  const elapsed = screen.getByLabelText('Elapsed time (ms)')
  fireEvent.change(elapsed, { target: { value: '5000' } })
  expect(get).toHaveBeenCalledTimes(1)
  fireEvent.click(screen.getByRole('button', { name: 'Apply' }))
  await waitFor(() =>
    expect(get).toHaveBeenLastCalledWith(
      '/api/channel/early-chunk-grid',
      expect.objectContaining({
        params: { model: 'k3', channel_id: 49, probe_ms: 5000 },
      })
    )
  )
  fireEvent.change(elapsed, { target: { value: '300001' } })
  expect(screen.getByRole('button', { name: 'Apply' })).toBeDisabled()
  expect(elapsed).toHaveAttribute('aria-invalid', 'true')
})

test('storage failures hide stale measurements and can be retried', async () => {
  const get = vi
    .spyOn(api, 'get')
    .mockResolvedValueOnce({ data: { success: false, message: 'unavailable' } })
    .mockResolvedValue({ data: { success: true, data: fixture() } })
  mount()
  expect(
    await screen.findByText('Prediction statistics unavailable')
  ).toBeVisible()
  expect(
    screen.queryByRole('button', { name: '0–1k / 0–10%' })
  ).not.toBeInTheDocument()
  fireEvent.click(screen.getByRole('button', { name: 'Retry' }))
  expect(
    await screen.findByRole('button', { name: '0–1k / 0–10%' })
  ).toBeVisible()
  expect(get).toHaveBeenCalledTimes(2)
})

test('candidate preview is automatic by default and allows probing long waits', async () => {
  const data = fixture()
  data.probe_ms = -1
  const get = vi
    .spyOn(api, 'get')
    .mockResolvedValue({ data: { success: true, data } })
  mount()
  await screen.findByRole('button', { name: '0–1k / 0–10%' })
  expect(get).toHaveBeenCalledWith(
    '/api/channel/early-chunk-grid',
    expect.objectContaining({
      params: { model: 'k3', channel_id: 49, probe_ms: -1 },
    })
  )
  expect(screen.getByPlaceholderText('Per-bucket candidate')).toBeVisible()
  expect(
    screen.getByText(/Each bucket is evaluated at its candidate time/)
  ).toBeVisible()
  fireEvent.change(screen.getByLabelText('Elapsed time (ms)'), {
    target: { value: '60000' },
  })
  fireEvent.click(screen.getByRole('button', { name: 'Apply' }))
  await waitFor(() =>
    expect(get).toHaveBeenLastCalledWith(
      '/api/channel/early-chunk-grid',
      expect.objectContaining({
        params: { model: 'k3', channel_id: 49, probe_ms: 60000 },
      })
    )
  )
})
