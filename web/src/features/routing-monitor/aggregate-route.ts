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
import type {
  MonitorChannel,
  MonitorHistory,
  MonitorNode,
  MonitorRequest,
  MonitorRoute,
} from './api'

function combineMonitorMetric(
  left: number | null,
  leftCount: number | undefined,
  right: number | null,
  rightCount: number | undefined
) {
  if (
    leftCount === undefined ||
    rightCount === undefined ||
    (left !== null && leftCount <= 0) ||
    (right !== null && rightCount <= 0)
  ) {
    return { value: null, count: undefined }
  }
  const count = leftCount + rightCount
  return {
    value:
      count > 0
        ? ((left ?? 0) * leftCount + (right ?? 0) * rightCount) / count
        : null,
    count,
  }
}

// Counts are additive; quality uses the actual number of valid samples.
// Node targets remain independent, so the combined flow shows observed shares.
export function aggregateMonitorRoutes(
  nodes: MonitorNode[],
  generatedAt: number,
  selectRoute: (node: MonitorNode) => MonitorRoute | undefined
): MonitorRoute | undefined {
  const routes = nodes
    .filter((node) => generatedAt - node.updated_at <= 15000)
    .flatMap((node) => {
      const route = selectRoute(node)
      return route ? [{ node, route }] : []
    })
  if (routes.length === 0) return undefined
  const channels = new Map<number, MonitorChannel>()
  const history = new Map<number, MonitorHistory>()
  const result: MonitorRoute & { requests: MonitorRequest[] } = {
    ...routes[0].route,
    aggregate: true,
    selection_count: 0,
    channels: [],
    history: [],
    events: [],
    requests: [],
    requests_truncated: false,
    truncated: false,
  }
  for (const { node, route } of routes) {
    result.selection_count += route.selection_count
    result.last_seen = Math.max(result.last_seen, route.last_seen)
    result.dynamic ||= route.dynamic
    result.truncated ||= route.truncated
    result.requests_truncated ||= route.requests_truncated
    result.events.push(
      ...route.events.map((event) => ({
        ...event,
        node_id: node.id,
        node_name: node.name,
      }))
    )
    result.requests.push(
      ...(route.requests ?? []).map((request) => ({
        ...request,
        id: `${node.id}:${request.id}`,
      }))
    )
    for (const channel of route.channels) {
      const combined = channels.get(channel.channel_id)
      if (!combined) {
        channels.set(channel.channel_id, {
          ...channel,
          ttft_ms: channel.ttft_sample_count ? channel.ttft_ms : null,
          tpot_ms: channel.tpot_sample_count ? channel.tpot_ms : null,
          ttft_sample_count:
            channel.ttft_ms !== null && !channel.ttft_sample_count
              ? undefined
              : channel.ttft_sample_count,
          tpot_sample_count:
            channel.tpot_ms !== null && !channel.tpot_sample_count
              ? undefined
              : channel.tpot_sample_count,
        })
        continue
      }
      combined.selection_count += channel.selection_count
      combined.sample_count += channel.sample_count
      const ttft = combineMonitorMetric(
        combined.ttft_ms,
        combined.ttft_sample_count,
        channel.ttft_ms,
        channel.ttft_sample_count
      )
      combined.ttft_ms = ttft.value
      combined.ttft_sample_count = ttft.count
      const tpot = combineMonitorMetric(
        combined.tpot_ms,
        combined.tpot_sample_count,
        channel.tpot_ms,
        channel.tpot_sample_count
      )
      combined.tpot_ms = tpot.value
      combined.tpot_sample_count = tpot.count
      combined.active ||= channel.active
      combined.degraded ||= channel.degraded
      combined.unverified ||= channel.unverified
      combined.probe ||= channel.probe
      combined.ejected_until = Math.max(
        combined.ejected_until ?? 0,
        channel.ejected_until ?? 0
      )
      if (
        combined.effective_cost_discount !== channel.effective_cost_discount
      ) {
        combined.effective_cost_discount = null
      }
    }
    for (const bucket of route.history) {
      let combined = history.get(bucket.at)
      if (!combined) {
        combined = { at: bucket.at, selection_count: 0, channels: [] }
        history.set(bucket.at, combined)
      }
      combined.selection_count += bucket.selection_count
      for (const channel of bucket.channels) {
        const count = combined.channels.find(
          (item) => item.channel_id === channel.channel_id
        )
        if (count) count.selection_count += channel.selection_count
        else combined.channels.push({ ...channel })
      }
    }
  }
  for (const channel of channels.values()) {
    channel.selected_share =
      result.selection_count > 0
        ? channel.selection_count / result.selection_count
        : 0
    channel.target_share = channel.selected_share
  }
  for (const bucket of history.values()) {
    for (const channel of bucket.channels) {
      channel.selected_share =
        bucket.selection_count > 0
          ? channel.selection_count / bucket.selection_count
          : 0
    }
  }
  result.channels = [...channels.values()].sort(
    (a, b) => a.channel_id - b.channel_id
  )
  result.history = [...history.values()].sort((a, b) => a.at - b.at)
  result.events.sort(
    (a, b) => b.at - a.at || (a.node_id ?? '').localeCompare(b.node_id ?? '')
  )
  result.requests.sort((a, b) => a.at - b.at || a.id.localeCompare(b.id))
  if (result.requests.length > 512) {
    result.requests = result.requests.slice(-512)
    result.requests_truncated = true
  }
  return result
}
