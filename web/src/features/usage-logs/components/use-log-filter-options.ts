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
import { useQuery } from '@tanstack/react-query'
import type { Table } from '@tanstack/react-table'
import { useMemo } from 'react'

import type { ComboboxInputOption } from '@/components/ui/combobox-input'
import { getChannels } from '@/features/channels/api'
import { getApiKeys } from '@/features/keys/api'
import { getUsers } from '@/features/users/api'
import { getUserModels } from '@/lib/api'
import { requireServerSuccess } from '@/lib/server-error-message'

import type { LogCategory } from '../types'

const FILTER_OPTIONS_STALE_TIME = 5 * 60 * 1000
const FILTER_OPTIONS_PAGE_SIZE = 100

type LogRecord = Record<string, unknown>

function getLogOptions(
  logs: LogRecord[],
  field: string
): ComboboxInputOption[] {
  return mergeOptions(
    logs.flatMap((log) => {
      const value = log[field]
      if (
        (typeof value !== 'string' && typeof value !== 'number') ||
        String(value).trim() === ''
      ) {
        return []
      }
      const normalizedValue = String(value)
      return [{ value: normalizedValue, label: normalizedValue }]
    })
  )
}

function mergeOptions(
  ...optionGroups: ComboboxInputOption[][]
): ComboboxInputOption[] {
  const options = new Map<string, ComboboxInputOption>()
  for (const group of optionGroups) {
    for (const option of group) {
      if (!options.has(option.value)) options.set(option.value, option)
    }
  }
  return [...options.values()]
}

export function useLogFilterOptions<TData>(
  table: Table<TData>,
  logCategory: LogCategory,
  isAdmin: boolean
) {
  const tableData = table.options.data
  const logs = useMemo(
    () => tableData.map((log) => log as LogRecord),
    [tableData]
  )
  const isCommon = logCategory === 'common'
  const modelsQuery = useQuery({
    queryKey: ['usage-logs', 'filter-options', 'models'],
    queryFn: async () => {
      const result = requireServerSuccess(await getUserModels())
      return Array.isArray(result.data) ? result.data : []
    },
    enabled: isCommon,
    staleTime: FILTER_OPTIONS_STALE_TIME,
    meta: { errorToast: false },
  })
  const tokensQuery = useQuery({
    queryKey: ['usage-logs', 'filter-options', 'tokens'],
    queryFn: async () =>
      requireServerSuccess(
        await getApiKeys({ p: 1, size: FILTER_OPTIONS_PAGE_SIZE })
      ).data?.items ?? [],
    enabled: isCommon,
    staleTime: FILTER_OPTIONS_STALE_TIME,
    meta: { errorToast: false },
  })
  const usersQuery = useQuery({
    queryKey: ['usage-logs', 'filter-options', 'users'],
    queryFn: async () =>
      requireServerSuccess(
        await getUsers({
          p: 1,
          page_size: FILTER_OPTIONS_PAGE_SIZE,
          sort_by: 'username',
          sort_order: 'asc',
        })
      ).data?.items ?? [],
    enabled: isCommon && isAdmin,
    staleTime: FILTER_OPTIONS_STALE_TIME,
    meta: { errorToast: false },
  })
  const channelsQuery = useQuery({
    queryKey: ['usage-logs', 'filter-options', 'channels'],
    queryFn: async () =>
      requireServerSuccess(
        await getChannels({
          p: 1,
          page_size: FILTER_OPTIONS_PAGE_SIZE,
          id_sort: true,
        })
      ).data?.items ?? [],
    enabled: isAdmin,
    staleTime: FILTER_OPTIONS_STALE_TIME,
    meta: { errorToast: false },
  })

  return useMemo(
    () => ({
      modelOptions: mergeOptions(
        (modelsQuery.data ?? []).map((model) => ({
          value: model,
          label: model,
        })),
        getLogOptions(logs, 'model_name')
      ),
      tokenOptions: mergeOptions(
        (tokensQuery.data ?? []).map((token) => ({
          value: token.name,
          label: token.name,
        })),
        getLogOptions(logs, 'token_name')
      ),
      usernameOptions: mergeOptions(
        (usersQuery.data ?? []).map((user) => ({
          value: user.username,
          label: user.username,
        })),
        getLogOptions(logs, 'username')
      ),
      channelOptions: mergeOptions(
        (channelsQuery.data ?? []).map((channel) => ({
          value: String(channel.id),
          label: channel.name
            ? `${channel.name} (#${channel.id})`
            : `#${channel.id}`,
        })),
        logs.flatMap((log) => {
          if (
            (typeof log.channel !== 'number' &&
              typeof log.channel !== 'string') ||
            String(log.channel).trim() === ''
          ) {
            return []
          }
          const value = String(log.channel)
          const name =
            typeof log.channel_name === 'string' ? log.channel_name : ''
          return [
            {
              value,
              label: name ? `${name} (#${value})` : `#${value}`,
            },
          ]
        })
      ),
      requestIdOptions: getLogOptions(logs, 'request_id'),
      upstreamRequestIdOptions: getLogOptions(logs, 'upstream_request_id'),
      taskIdOptions: getLogOptions(
        logs,
        logCategory === 'drawing' ? 'mj_id' : 'task_id'
      ),
    }),
    [
      logs,
      logCategory,
      modelsQuery.data,
      tokensQuery.data,
      usersQuery.data,
      channelsQuery.data,
    ]
  )
}
