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

import { describe, expect, test } from 'vitest'

import { toIntlLocale } from '@/i18n/languages'

import { buildPerformanceChartOption } from '../chart-options'
import * as analyticsLib from '../lib'

describe('performance analytics chart series', () => {
  test.each([
    'request_body_avg_bytes',
    'cache_write_tokens',
    'cache_hit_rate',
  ] as const)(
    'preserves measured zeroes and omits unknown legacy %s samples',
    (metric) => {
      const points = [
        {
          ts: 100,
          [metric]: 0,
          request_body_samples: 1,
          cache_write_samples: 1,
          cache_read_samples: 1,
        },
        {
          ts: 200,
          [metric]: 0,
          request_body_samples: 0,
          cache_write_samples: 0,
          cache_read_samples: 0,
        },
      ] as unknown as import('../types').PerformanceAnalyticsPoint[]
      expect(
        analyticsLib.buildPerformanceMetricSeries(points, metric, String)
      ).toEqual([
        { ts: 100, time: 100, label: '100', percentile: 'AVG', value: 0 },
      ])
    }
  )
  test.each([
    'request_body_avg_bytes',
    'cache_write_tokens',
    'cache_hit_rate',
  ] as const)(
    'keeps the known aggregate for %s when an older API has no distributions',
    (metric) => {
      const points = [
        {
          ts: 100,
          [metric]: 12.25,
          request_body_samples: 1,
          cache_write_samples: 1,
          cache_read_samples: 1,
        },
      ] as unknown as import('../types').PerformanceAnalyticsPoint[]
      expect(
        analyticsLib.buildPerformanceMetricSeries(points, metric, String)
      ).toEqual([
        { ts: 100, time: 100, label: '100', percentile: 'AVG', value: 12.25 },
      ])
    }
  )

  test.each(['zhCN', 'zhTW', 'en', 'fr', 'ru', 'ja', 'vi', 'invalid-language'])(
    'preserves latency precision and measured zeroes in %s',
    (language) => {
      const locale = toIntlLocale(language)
      expect(analyticsLib.formatPerformanceLatency(24.125, locale)).toBe(
        `${Intl.NumberFormat(locale, { maximumFractionDigits: 3 }).format(24.125)} ms`
      )
      expect(analyticsLib.formatPerformanceLatency(0, locale)).toBe('0 ms')
      expect(analyticsLib.formatTtftSeconds(1241, locale)).toBe(
        `${Intl.NumberFormat(locale, { maximumFractionDigits: 3 }).format(1.241)} s`
      )
      expect(analyticsLib.formatPerformanceLatency(-1, locale)).toBe('—')
    }
  )

  test('keeps missing metric samples as gaps and retains decimal values in ECharts', () => {
    const option = buildPerformanceChartOption({
      title: 'RPM',
      values: [
        { ts: 100, time: 100, label: '100', value: 1.2345 },
        { ts: 300, time: 300, label: '300', value: 0 },
      ],
      timestamps: [100, 200, 300],
      start: 100,
      end: 300,
      dark: false,
      formatTime: String,
      formatValue: String,
    })
    const series = option.series as Array<{
      data: Array<[number, number | null]>
      connectNulls: boolean
    }>
    expect(series[0].data).toEqual([
      [100000, 1.2345],
      [200000, null],
      [300000, 0],
    ])
    expect(series[0].connectNulls).toBe(false)
    expect(option.dataZoom).toEqual([
      expect.objectContaining({ type: 'inside' }),
    ])
  })
  test.each(['request_body_avg_bytes', 'cache_hit_rate'] as const)(
    'keeps the %s distribution alongside the aggregate AVG',
    (metric) => {
      const points = [
        {
          ts: 100,
          [metric]: 999,
          metric_percentiles: {
            [metric]: {
              p50: 1,
              p90: 2,
              p95: 3,
              p99: 4,
              max: 5,
              sample_count: 10,
            },
          },
        },
      ] as unknown as import('../types').PerformanceAnalyticsPoint[]
      expect(
        analyticsLib
          .buildPerformanceMetricSeries(points, metric, String)
          .map((point) => [point.percentile, point.value])
      ).toEqual([
        ['AVG', 999],
        ['P50', 1],
        ['P90', 2],
        ['P95', 3],
        ['P99', 4],
        ['MAX', 5],
      ])
    }
  )

  test('omits unavailable maxima and non-finite percentiles instead of drawing zeroes', () => {
    const points = [
      {
        ts: 100,
        metric_percentiles: {
          cache_write_tokens: {
            p50: 0,
            p90: Infinity,
            p95: Number.NaN,
            p99: 10,
            max: null,
            sample_count: 1,
          },
        },
      },
    ] as unknown as import('../types').PerformanceAnalyticsPoint[]
    expect(
      analyticsLib
        .buildPerformanceMetricSeries(points, 'cache_write_tokens', String)
        .map((point) => [point.percentile, point.value])
    ).toEqual([
      ['P50', 0],
      ['P99', 10],
    ])
  })
  test('shows five independent cache-write curves and preserves known zero samples', () => {
    const points = [
      {
        ts: 100,
        metric_percentiles: {
          cache_write_tokens: {
            p50: 0,
            p90: 400,
            p95: 500,
            p99: 570,
            max: 573,
            sample_count: 20,
          },
        },
      },
      { ts: 200, cache_write_tokens: 999, cache_write_samples: 2 },
      {
        ts: 300,
        metric_percentiles: {
          cache_write_tokens: {
            p50: 0,
            p90: 0,
            p95: 0,
            p99: 0,
            max: 0,
            sample_count: 0,
          },
        },
      },
    ] as unknown as import('../types').PerformanceAnalyticsPoint[]
    expect(
      analyticsLib.buildPerformanceMetricSeries(
        points,
        'cache_write_tokens',
        String
      )
    ).toEqual([
      { ts: 100, time: 100, label: '100', percentile: 'P50', value: 0 },
      { ts: 100, time: 100, label: '100', percentile: 'P90', value: 400 },
      { ts: 100, time: 100, label: '100', percentile: 'P95', value: 500 },
      { ts: 100, time: 100, label: '100', percentile: 'P99', value: 570 },
      { ts: 100, time: 100, label: '100', percentile: 'MAX', value: 573 },
      { ts: 200, time: 200, label: '200', percentile: 'AVG', value: 499.5 },
    ])
  })

  test('formats body sizes without rounding small payloads to zero and accepts all interface languages', () => {
    for (const language of [
      'en',
      'zhCN',
      'zhTW',
      'fr',
      'ru',
      'ja',
      'vi',
      'invalid-language',
    ]) {
      const locale = toIntlLocale(language)
      assert.equal(analyticsLib.formatRequestBodySize(1, locale), '1 B')
      assert.equal(analyticsLib.formatRequestBodySize(1024, locale), '1 KiB')
      assert.equal(
        analyticsLib.formatRequestBodySize(1024 ** 2, locale),
        '1 MiB'
      )
    }
  })
  test('bounds a single timestamp to its nearby interval', () => {
    const axis = analyticsLib.buildPerformanceTimeAxis(
      String,
      '#fff',
      [1_800_000_000]
    )
    assert.equal(axis.min, 1_799_999_850)
    assert.equal(axis.max, 1_800_000_150)
  })
  test('preserves millisecond precision when displaying TTFT in seconds', () => {
    assert.equal(analyticsLib.formatTtftSeconds(50), '0.05 s')
    assert.equal(analyticsLib.formatTtftSeconds(1_241), '1.241 s')
    assert.equal(analyticsLib.formatTtftSeconds(0), '0 s')
    assert.equal(analyticsLib.formatTtftSeconds(Number.NaN), '—')
  })

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

  test('keeps throughput metrics as scalar chart points', () => {
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
        'rpm',
        (ts) => `time-${ts}`
      ),
      [
        { ts: 100, time: 100, label: 'time-100', value: 12.5 },
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
        error_count: 0,
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

  test('expands each sampled bucket into P50, P90, P95, P99, and MAX curves', () => {
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
              p95_ms: 800,
              p99_ms: 1000,
              max_ms: 1200,
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
          percentile: 'P95',
          value: 800,
        },
        {
          ts: 100,
          time: 100,
          label: 'time-100',
          percentile: 'P99',
          value: 1000,
        },
        {
          ts: 100,
          time: 100,
          label: 'time-100',
          percentile: 'MAX',
          value: 1200,
        },
      ]
    )
  })
})

test('AVG curves use measured bucket totals while cache percentiles retain per-request distributions', () => {
  const points = [
    {
      ts: 100,
      cache_read_samples: 2,
      cache_write_samples: 2,
      cache_write_tokens: 125,
      request_body_samples: 2,
      request_body_avg_bytes: 12.5,
      cache_hit_rate: 10,
      metric_percentiles: {
        cache_hit_rate: {
          p50: 50,
          p90: 90,
          p95: 95,
          p99: 99,
          max: 100,
          sample_count: 2,
        },
      },
      ttft: {
        p50_ms: 100,
        p90_ms: 200,
        p99_ms: 300,
        avg_ms: 150.5,
        sample_count: 2,
      },
      tpot: { p50_ms: 2, p90_ms: 8, p99_ms: 8, avg_ms: 2.6, sample_count: 2 },
    },
  ] as unknown as import('../types').PerformanceAnalyticsPoint[]
  const cache = analyticsLib.buildPerformanceMetricSeries(
    points,
    'cache_hit_rate',
    String
  )
  expect(cache.map((point) => [point.percentile, point.value])).toEqual([
    ['AVG', 10],
    ['P50', 50],
    ['P90', 90],
    ['P95', 95],
    ['P99', 99],
    ['MAX', 100],
  ])
  expect(
    analyticsLib.buildPerformanceMetricSeries(
      points,
      'cache_write_tokens',
      String
    )[0]
  ).toMatchObject({ percentile: 'AVG', value: 62.5 })
  expect(
    analyticsLib.buildPerformanceMetricSeries(
      points,
      'request_body_avg_bytes',
      String
    )[0]
  ).toMatchObject({ percentile: 'AVG', value: 12.5 })
  expect(
    analyticsLib.buildPerformanceSeries(points, 'ttft', String)[0]
  ).toMatchObject({ percentile: 'AVG', value: 150.5 })
  expect(
    analyticsLib.buildPerformanceSeries(points, 'tpot', String)[0]
  ).toMatchObject({ percentile: 'AVG', value: 2.6 })
})
