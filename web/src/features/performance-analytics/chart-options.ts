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
import type { EChartsCoreOption } from 'echarts/core'

import { PERFORMANCE_CURVE_COLORS, PERFORMANCE_CURVE_LABELS } from './lib'
import type { PerformanceMetricChartDatum } from './types'

export function buildPerformanceChartOption(props: {
  title: string
  values: PerformanceMetricChartDatum[]
  timestamps: number[]
  start: number
  end: number
  dark: boolean
  formatTime: (ts: number) => string
  formatValue: (value: number) => string
  formatTooltipValue?: (value: number) => string
}): EChartsCoreOption {
  let names = [props.title]
  if (props.values.some((point) => point.series)) {
    names = [
      ...new Set(props.values.map((point) => point.series ?? props.title)),
    ]
  } else if (props.values.some((point) => point.percentile)) {
    names = [...PERFORMANCE_CURVE_LABELS]
  }
  const text = props.dark ? '#a1a1aa' : '#64748b'
  const grid = props.dark ? '#ffffff14' : '#0f172a0f'
  const timestamps = [...new Set(props.timestamps)].sort((a, b) => a - b)
  return {
    backgroundColor: 'transparent',
    color: PERFORMANCE_CURVE_COLORS,
    animationDuration: 350,
    animationDurationUpdate: 300,
    aria: { enabled: true, description: props.title },
    grid: { top: 42, left: 12, right: 16, bottom: 16, containLabel: true },
    legend: {
      top: 0,
      right: 4,
      itemWidth: 14,
      itemHeight: 3,
      textStyle: { color: text, fontSize: 11 },
      data: names,
    },
    tooltip: {
      trigger: 'axis',
      renderMode: 'richText',
      confine: true,
      backgroundColor: props.dark ? '#18181b' : '#fff',
      borderColor: grid,
      textStyle: { color: props.dark ? '#fafafa' : '#0f172a', fontSize: 12 },
      axisPointer: { type: 'cross', label: { precision: 3 } },
      valueFormatter: (value: unknown) =>
        typeof value === 'number'
          ? (props.formatTooltipValue ?? props.formatValue)(value)
          : '—',
    },
    xAxis: {
      type: 'time',
      min: props.start * 1000,
      max: props.end * 1000,
      axisLine: { show: false },
      axisTick: { show: false },
      splitLine: { show: false },
      axisLabel: {
        color: text,
        fontSize: 10,
        hideOverlap: true,
        formatter: (value: number) => props.formatTime(value / 1000),
      },
      axisPointer: {
        label: {
          formatter: (params: { value: number }) =>
            props.formatTime(params.value / 1000),
        },
      },
    },
    yAxis: {
      type: 'value',
      min: 0,
      splitNumber: 4,
      axisLabel: {
        color: text,
        fontSize: 10,
        formatter: (value: number) => props.formatValue(value),
      },
      axisPointer: {
        label: {
          formatter: (params: { value: number }) =>
            props.formatValue(params.value),
        },
      },
      splitLine: { lineStyle: { color: grid, type: 'dashed' } },
    },
    dataZoom: [
      {
        id: 'inside',
        type: 'inside',
        xAxisIndex: 0,
        filterMode: 'none',
        zoomOnMouseWheel: 'ctrl',
        moveOnMouseMove: true,
        preventDefaultMouseMove: false,
      },
    ],
    series: names.map((name, index) => {
      const values = new Map(
        props.values
          .filter(
            (point) =>
              (point.series ?? point.percentile ?? props.title) === name
          )
          .map((point) => [point.ts, point.value])
      )
      return {
        id: name,
        name,
        type: 'line',
        smooth: false,
        connectNulls: false,
        showSymbol: timestamps.length <= 2,
        symbolSize: 5,
        lineStyle: {
          width: name === 'MAX' ? 1.5 : 2,
          type: name === 'MAX' ? 'dashed' : 'solid',
        },
        emphasis: { focus: 'series' },
        itemStyle: { color: PERFORMANCE_CURVE_COLORS[index] },
        data: timestamps.map((ts) => [ts * 1000, values.get(ts) ?? null]),
      }
    }),
  }
}
