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
import assert from 'node:assert/strict'
import { describe, test } from 'node:test'

import * as analyticsLib from '../lib'

describe('performance analytics chart series', () => {
  test('does not force Unix timestamp axes to start at zero', () => {
    const buildPerformanceTimeAxis = (analyticsLib as Record<string, unknown>)
      .buildPerformanceTimeAxis as
      | ((
          formatTime: (ts: number) => string,
          textColor: string
        ) => Record<string, unknown>)
      | undefined

    assert.equal(typeof buildPerformanceTimeAxis, 'function')
    if (!buildPerformanceTimeAxis) return

    const axis = buildPerformanceTimeAxis(() => 'time', '#fff')
    assert.equal(axis.type, 'linear')
    assert.equal(axis.zero, false)
  })

  test('maps throughput and cache metrics onto scalar chart points', () => {
    const buildPerformanceMetricSeries = (
      analyticsLib as Record<string, unknown>
    ).buildPerformanceMetricSeries as
      | ((
          points: Array<Record<string, unknown>>,
          metric: 'rpm' | 'tpm' | 'cache_hit_rate',
          formatTime: (ts: number) => string
        ) => Array<Record<string, unknown>>)
      | undefined

    assert.equal(typeof buildPerformanceMetricSeries, 'function')
    if (!buildPerformanceMetricSeries) return

    assert.deepEqual(
      buildPerformanceMetricSeries(
        [
          { ts: 100, rpm: 12.5, tpm: 6400, cache_hit_rate: 35.25 },
          { ts: 200, rpm: 0, tpm: 0, cache_hit_rate: 0 },
        ],
        'cache_hit_rate',
        (ts) => `time-${ts}`
      ),
      [
        { ts: 100, time: 100, label: 'time-100', value: 35.25 },
        { ts: 200, time: 200, label: 'time-200', value: 0 },
      ]
    )
  })

  test('resolves the selected filter value to its display label', () => {
    const getPerformanceFilterLabel = (analyticsLib as Record<string, unknown>)
      .getPerformanceFilterLabel as
      | ((
          value: string | null,
          options: Array<{ value: string; label: string }>
        ) => string | undefined)
      | undefined

    assert.equal(typeof getPerformanceFilterLabel, 'function')
    if (!getPerformanceFilterLabel) return

    assert.equal(
      getPerformanceFilterLabel('2', [
        { value: '__all__', label: '全部用户' },
        { value: '2', label: 'alice-demo' },
      ]),
      'alice-demo'
    )
  })

  test('formats timestamps for the project Chinese language codes', () => {
    const formatPerformanceTimestamp = (analyticsLib as Record<string, unknown>)
      .formatPerformanceTimestamp as
      | ((timestamp: number, language?: string) => string)
      | undefined

    assert.equal(typeof formatPerformanceTimestamp, 'function')
    if (!formatPerformanceTimestamp) return

    assert.doesNotThrow(() => formatPerformanceTimestamp(1_700_000_000, 'zhCN'))
    assert.notEqual(formatPerformanceTimestamp(1_700_000_000, 'zhCN'), '')
  })

  test('keeps chart x values as Unix seconds for direct axis formatting', () => {
    const points = [
      {
        ts: 1_700_000_000,
        request_count: 8,
        success_rate: 100,
        rpm: 1.6,
        tpm: 2400,
        cache_hit_rate: 25,
        ttft: {
          p50_ms: 120,
          p90_ms: 500,
          p99_ms: 1000,
          sample_count: 8,
        },
        tpot: {
          p50_ms: 20,
          p90_ms: 40,
          p99_ms: 80,
          sample_count: 8,
        },
      },
    ]

    const series = analyticsLib.buildPerformanceSeries(
      points,
      'ttft',
      () => 'label'
    )

    assert.equal(series[0]?.time, 1_700_000_000)
  })

  test('expands each sampled bucket into P50, P90, and P99 curves', () => {
    const buildPerformanceSeries = (analyticsLib as Record<string, unknown>)
      .buildPerformanceSeries as
      | ((
          points: Array<Record<string, unknown>>,
          metric: 'ttft' | 'tpot',
          formatTime: (ts: number) => string
        ) => Array<Record<string, unknown>>)
      | undefined

    assert.equal(typeof buildPerformanceSeries, 'function')
    if (!buildPerformanceSeries) return

    assert.deepEqual(
      buildPerformanceSeries(
        [
          {
            ts: 100,
            ttft: {
              p50_ms: 120,
              p90_ms: 500,
              p99_ms: 1000,
              sample_count: 8,
            },
          },
          {
            ts: 200,
            ttft: {
              p50_ms: 0,
              p90_ms: 0,
              p99_ms: 0,
              sample_count: 0,
            },
          },
        ],
        'ttft',
        (ts) => `time-${ts}`
      ),
      [
        {
          ts: 100,
          time: 100,
          label: 'time-100',
          percentile: 'P50',
          value: 120,
        },
        {
          ts: 100,
          time: 100,
          label: 'time-100',
          percentile: 'P90',
          value: 500,
        },
        {
          ts: 100,
          time: 100,
          label: 'time-100',
          percentile: 'P99',
          value: 1000,
        },
      ]
    )
  })
})
