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
import { CanceledError, type AxiosAdapter } from 'axios'
import { afterEach, expect, test } from 'vitest'

import { api } from '@/lib/api'

import { getRoutingMonitor, type MonitorSnapshot } from '../api'

const originalAdapter = api.defaults.adapter
afterEach(() => {
  api.defaults.adapter = originalAdapter
})

test('a cancelled observation request does not poison the next request for the same scope', async () => {
  const snapshot: MonitorSnapshot = {
    generated_at: 1000,
    legacy_enabled: false,
    coverage: 'local',
    scope: 'base',
    partial: false,
    nodes: [],
  }
  let markStarted!: () => void
  const started = new Promise<void>((resolve) => {
    markStarted = resolve
  })
  let attempts = 0
  const adapter: AxiosAdapter = async (config) => {
    attempts++
    if (attempts === 1) {
      return new Promise((_, reject) => {
        config.signal?.addEventListener?.('abort', () =>
          reject(new CanceledError('Cancelled observation', config))
        )
        markStarted()
      })
    }
    return {
      data: { success: true, data: snapshot },
      status: 200,
      statusText: 'OK',
      headers: {},
      config,
    }
  }
  api.defaults.adapter = adapter
  const firstController = new AbortController()
  const firstResult = getRoutingMonitor(
    undefined,
    firstController.signal
  ).catch((error: unknown) => error)
  await started
  firstController.abort()
  const next = getRoutingMonitor(undefined, new AbortController().signal)
  await expect(next).resolves.toEqual(snapshot)
  await expect(firstResult).resolves.toMatchObject({ code: 'ERR_CANCELED' })
})
