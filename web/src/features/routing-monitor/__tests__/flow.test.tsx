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
import { act, render } from '@testing-library/react'
import { afterEach, expect, test, vi } from 'vitest'

import type { MonitorRoute } from '../api'
import { RequestParticles, RoutingFlow } from '../flow'

afterEach(() => {
  vi.useRealTimers()
  vi.restoreAllMocks()
})

test('a monitor panel uses a compact three-column flow when its own width reaches 22rem', () => {
  const matchMedia = window.matchMedia
  vi.spyOn(window, 'matchMedia').mockImplementation((query) => ({
    ...matchMedia(query),
    matches: false,
  }))
  const route: MonitorRoute = {
    group: 'standard',
    model: 'base-model',
    policy: '',
    mode: 'priority',
    dynamic: false,
    last_seen: 0,
    window_seconds: 60,
    selection_count: 0,
    truncated: false,
    channels: [],
    history: [],
    events: [],
  }
  const view = render(<RoutingFlow route={route} at={0} paused={false} panel />)
  const diagram = view.container.querySelector('[data-routing-flow-layout]')
  expect(diagram).toHaveClass(
    'grid-cols-1',
    '@min-[22rem]/monitor-panel:grid-cols-[4rem_6.5rem_minmax(0,16rem)]'
  )
  expect(diagram).not.toHaveClass(
    'md:grid-cols-[110px_165px_minmax(210px,260px)]'
  )
})

test('each physical request animates once along its own channel and repeated snapshots do not replay it', async () => {
  vi.useFakeTimers()
  const paths = new Map([
    [49, 'M 0 0 L 20 20'],
    [55, 'M 0 0 L 20 40'],
  ])
  const at = Date.now()
  const requests = [{ id: 'node:1', channel_id: 49, at }]
  const view = render(
    <svg>
      <RequestParticles
        requests={requests}
        at={at}
        paths={paths}
        disabled={false}
      />
    </svg>
  )
  expect(view.container.querySelectorAll('circle')).toHaveLength(1)
  expect(view.container.querySelector('circle')).toHaveAttribute(
    'data-routing-channel-id',
    '49'
  )
  expect(view.container.querySelector('animateMotion')).toHaveAttribute(
    'path',
    paths.get(49)
  )
  expect(view.container.querySelector('animateMotion')).toHaveAttribute(
    'repeatCount',
    '1'
  )
  await act(async () => {
    await vi.advanceTimersByTimeAsync(4100)
  })
  expect(view.container.querySelectorAll('circle')).toHaveLength(0)
  view.rerender(
    <svg>
      <RequestParticles
        requests={[...requests]}
        at={at + 4100}
        paths={paths}
        disabled={false}
      />
    </svg>
  )
  expect(view.container.querySelectorAll('circle')).toHaveLength(0)
  view.rerender(
    <svg>
      <RequestParticles
        requests={[
          ...requests,
          { id: 'node:2', channel_id: 55, at: at + 4200 },
        ]}
        at={at + 4200}
        paths={paths}
        disabled={false}
      />
    </svg>
  )
  expect(view.container.querySelectorAll('circle')).toHaveLength(1)
  expect(view.container.querySelector('circle')).toHaveAttribute(
    'data-routing-channel-id',
    '55'
  )
})

test('no requests, expired requests and paused updates do not create circulating dots', () => {
  const at = Date.now()
  const paths = new Map([[49, 'M 0 0 L 20 20']])
  const view = render(
    <svg>
      <RequestParticles requests={[]} at={at} paths={paths} disabled={false} />
    </svg>
  )
  expect(view.container.querySelectorAll('circle')).toHaveLength(0)
  view.rerender(
    <svg>
      <RequestParticles
        requests={[{ id: 'old', channel_id: 49, at: at - 16000 }]}
        at={at}
        paths={paths}
        disabled={false}
      />
    </svg>
  )
  expect(view.container.querySelectorAll('circle')).toHaveLength(0)
  const requests = [{ id: 'paused', channel_id: 49, at }]
  view.rerender(
    <svg>
      <RequestParticles requests={requests} at={at} paths={paths} disabled />
    </svg>
  )
  view.rerender(
    <svg>
      <RequestParticles
        requests={requests}
        at={at}
        paths={paths}
        disabled={false}
      />
    </svg>
  )
  expect(view.container.querySelectorAll('circle')).toHaveLength(0)
})

test('a polling batch preserves request arrival spacing instead of stacking every dot at once', async () => {
  vi.useFakeTimers()
  const at = Date.now()
  const paths = new Map([[49, 'M 0 0 L 20 20']])
  const requests = [
    { id: 'first', channel_id: 49, at },
    { id: 'second', channel_id: 49, at: at + 1000 },
  ]
  const view = render(
    <svg>
      <RequestParticles
        requests={requests}
        at={at + 1000}
        paths={paths}
        disabled={false}
      />
    </svg>
  )
  expect(view.container.querySelectorAll('circle')).toHaveLength(1)
  await act(async () => {
    await vi.advanceTimersByTimeAsync(1000)
  })
  expect(view.container.querySelectorAll('circle')).toHaveLength(2)
})

test('the rendered flow sends one real request to its channel even when its target share is zero', () => {
  const matchMedia = window.matchMedia
  vi.spyOn(window, 'matchMedia').mockImplementation((query) => ({
    ...matchMedia(query),
    matches: false,
  }))
  vi.spyOn(HTMLElement.prototype, 'getBoundingClientRect').mockImplementation(
    function (this: HTMLElement) {
      if (this.hasAttribute('data-flow-source')) {
        return new DOMRect(0, 100, 110, 100)
      }
      if (this.hasAttribute('data-flow-policy')) {
        return new DOMRect(230, 100, 165, 100)
      }
      if (this.hasAttribute('data-flow-channel')) {
        const index = [...(this.parentElement?.children ?? [])].indexOf(this)
        return new DOMRect(500, index * 200, 260, 100)
      }
      return new DOMRect(0, 0, 800, 400)
    }
  )
  const at = Date.now()
  const route: MonitorRoute = {
    group: 'standard',
    model: 'kimi-k3',
    policy: '',
    mode: 'smart',
    dynamic: true,
    last_seen: at,
    window_seconds: 60,
    selection_count: 10,
    truncated: false,
    channels: [49, 55].map((id) => ({
      channel_id: id,
      active: true,
      priority: 1,
      weight: 100,
      effective_cost_discount: 0.5,
      target_share: id === 49 ? 1 : 0,
      selection_count: 5,
      selected_share: 0.5,
      probe: false,
      degraded: false,
      unverified: false,
      ttft_ms: 800,
      tpot_ms: 20,
      sample_count: 3,
    })),
    history: [],
    events: [],
    requests: [{ id: 'real-55', at, channel_id: 55 }],
  }
  const view = render(<RoutingFlow route={route} at={at} paused={false} />)
  expect(view.container.querySelectorAll('circle')).toHaveLength(1)
  expect(view.container.querySelector('circle')).toHaveAttribute(
    'data-routing-channel-id',
    '55'
  )
  const branch = view.container.querySelector('path[id$="channel-55"]')
  expect(branch).not.toBeNull()
  expect(
    view.container.querySelector('animateMotion')?.getAttribute('path')
  ).toContain(branch?.getAttribute('d'))
})
