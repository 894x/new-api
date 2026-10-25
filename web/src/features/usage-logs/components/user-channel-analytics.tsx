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
import { getRouteApi, useNavigate } from '@tanstack/react-router'
import { Loader2 } from 'lucide-react'
import { useCallback, useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { EmptyState } from '@/components/empty-state'
import { Button } from '@/components/ui/button'
import { ComboboxInput } from '@/components/ui/combobox-input'
import { Skeleton } from '@/components/ui/skeleton'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { searchUsers } from '@/features/users/api'
import {
  formatLogQuota,
  formatNumber,
  formatTimestampToDate,
  formatTokens,
} from '@/lib/format'
import { ROLE } from '@/lib/roles'
import { requireServerSuccess } from '@/lib/server-error-message'
import { useAuthStore } from '@/stores/auth-store'

import { getUserChannelStats } from '../api'
import { StatBadge } from './common-logs-stats'
import { CompactDateTimeRangePicker } from './compact-date-time-range-picker'

const route = getRouteApi('/_authenticated/usage-logs/$section')

/**
 * 管理员视角的用户渠道分析：按用户名 + 时间范围查看各渠道的用量汇总。
 * RPM/TPM 与日志页顶部统计同口径，取最近 60 秒的实时速率。
 * 筛选条件与通用日志页一样落在 URL（username/startTime/endTime），刷新不丢失；
 * 输入框先改本地草稿，点搜索按钮或回车才提交查询。
 */
export function UserChannelAnalytics() {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const role = useAuthStore((state) => state.auth.user?.role ?? ROLE.GUEST)
  const searchParams = route.useSearch()

  const searchState = useMemo(
    () => ({
      sourceKey: [
        searchParams.username,
        searchParams.startTime,
        searchParams.endTime,
      ]
        .map((value) => String(value ?? ''))
        .join('\u001f'),
      username: searchParams.username ?? '',
      startTime: searchParams.startTime,
      endTime: searchParams.endTime,
    }),
    [searchParams.username, searchParams.startTime, searchParams.endTime]
  )
  const [draft, setDraft] = useState(() => searchState)
  const active = draft.sourceKey === searchState.sourceKey ? draft : searchState

  const handleSearch = useCallback(() => {
    const committed = {
      username: active.username.trim() || undefined,
      startTime: active.startTime,
      endTime: active.endTime,
    }
    void navigate({
      to: '/usage-logs/$section',
      params: { section: 'user-analytics' },
      search: committed,
      replace: true,
    })
    // 提交后把草稿键对齐到已提交键，浏览器前进/后退时才能正确回落到 URL 值
    setDraft((current) => ({
      ...current,
      sourceKey: [committed.username, committed.startTime, committed.endTime]
        .map((value) => String(value ?? ''))
        .join('\u001f'),
    }))
  }, [active, navigate])

  const username = active.username
  const range = {
    start: active.startTime ? new Date(active.startTime) : undefined,
    end: active.endTime ? new Date(active.endTime) : undefined,
  }
  // 查询以 URL 为准，输入中的草稿不触发请求
  const selectedUser = (searchParams.username ?? '').trim()

  const usersQuery = useQuery({
    queryKey: ['user-channel-analytics', 'user-options'],
    queryFn: () =>
      searchUsers({ keyword: '', p: 1, page_size: 50, sort_by: 'id' }),
    staleTime: 60_000,
    enabled: role >= ROLE.ADMIN,
  })
  const userOptions = useMemo(
    () =>
      (usersQuery.data?.data?.items ?? []).map((user) => ({
        label: user.username,
        value: user.username,
      })),
    [usersQuery.data]
  )

  const statsQuery = useQuery({
    queryKey: [
      'user-channel-analytics',
      'stats',
      selectedUser,
      searchParams.startTime,
      searchParams.endTime,
    ],
    queryFn: async () =>
      requireServerSuccess(
        await getUserChannelStats({
          username: selectedUser,
          start_timestamp: searchParams.startTime
            ? Math.floor(searchParams.startTime / 1000)
            : undefined,
          end_timestamp: searchParams.endTime
            ? Math.floor(searchParams.endTime / 1000)
            : undefined,
        })
      ),
    enabled: role >= ROLE.ADMIN && selectedUser !== '',
    placeholderData: (previous) => previous,
  })

  if (role < ROLE.ADMIN) {
    return (
      <EmptyState
        className='min-h-0 flex-1'
        title={t('Admin access required')}
      />
    )
  }

  const rows = statsQuery.data?.data ?? []
  const summary = rows.reduce(
    (acc, row) => {
      acc.requests += row.count
      acc.promptTokens += row.prompt_tokens
      acc.completionTokens += row.completion_tokens
      acc.quota += row.quota
      acc.rpm += row.rpm
      acc.tpm += row.tpm
      return acc
    },
    {
      requests: 0,
      promptTokens: 0,
      completionTokens: 0,
      quota: 0,
      rpm: 0,
      tpm: 0,
    }
  )

  const handleViewDetails = (channelId: number) => {
    void navigate({
      to: '/usage-logs/$section',
      params: { section: 'common' },
      search: { username: selectedUser, channel: String(channelId) },
    })
  }

  return (
    <div className='flex h-full min-h-0 flex-col gap-4 overflow-y-auto'>
      <div className='flex flex-nowrap items-center gap-2'>
        <ComboboxInput
          className='w-56 shrink-0'
          options={userOptions}
          value={username}
          onValueChange={(value) =>
            setDraft({
              sourceKey: searchState.sourceKey,
              username: value,
              startTime: active.startTime,
              endTime: active.endTime,
            })
          }
          onKeyDown={(e) => {
            if (e.key === 'Enter') handleSearch()
          }}
          placeholder={t('Search user')}
          emptyText={t('No users found')}
          allowCustomValue
          aria-label={t('Search user')}
        />
        <CompactDateTimeRangePicker
          className='w-96 shrink-0'
          start={range.start}
          end={range.end}
          onChange={(next) =>
            setDraft({
              sourceKey: searchState.sourceKey,
              username: active.username,
              startTime: next.start?.getTime(),
              endTime: next.end?.getTime(),
            })
          }
        />
        <Button
          type='button'
          className='shrink-0'
          onClick={handleSearch}
          disabled={statsQuery.isFetching}
          aria-busy={statsQuery.isFetching}
        >
          {statsQuery.isFetching && <Loader2 className='animate-spin' />}
          {t('Search')}
        </Button>
      </div>

      {selectedUser === '' ? (
        <EmptyState
          className='min-h-0 flex-1'
          title={t('Select a user to view per-channel usage')}
        />
      ) : (
        <>
          <div className='flex flex-wrap items-center gap-2'>
            <StatBadge
              label={t('Requests')}
              value={formatNumber(summary.requests)}
              accent='bg-sky-500/70'
            />
            <StatBadge
              label={t('Prompt Tokens')}
              value={formatTokens(summary.promptTokens)}
              accent='bg-violet-500/70'
            />
            <StatBadge
              label={t('Completion Tokens')}
              value={formatTokens(summary.completionTokens)}
              accent='bg-emerald-500/70'
            />
            <StatBadge
              label={t('Usage')}
              value={formatLogQuota(summary.quota)}
              accent='bg-amber-500/70'
            />
            <StatBadge
              label={t('RPM')}
              value={summary.rpm}
              accent='bg-rose-500/65'
            />
            <StatBadge
              label={t('TPM')}
              value={summary.tpm}
              accent='bg-slate-400/70'
            />
          </div>

          {statsQuery.isLoading && rows.length === 0 ? (
            <div className='space-y-2'>
              <Skeleton className='h-9 w-full' />
              <Skeleton className='h-9 w-full' />
              <Skeleton className='h-9 w-2/3' />
            </div>
          ) : (
            <div className='overflow-hidden rounded-lg border'>
              <Table>
                <TableHeader>
                  <TableRow>
                    <TableHead>{t('Channel')}</TableHead>
                    <TableHead className='text-right'>
                      {t('Requests')}
                    </TableHead>
                    <TableHead className='text-right'>{t('Share')}</TableHead>
                    <TableHead className='text-right'>{t('RPM')}</TableHead>
                    <TableHead className='text-right'>{t('TPM')}</TableHead>
                    <TableHead className='text-right'>
                      {t('Prompt Tokens')}
                    </TableHead>
                    <TableHead className='text-right'>
                      {t('Completion Tokens')}
                    </TableHead>
                    <TableHead className='text-right'>{t('Usage')}</TableHead>
                    <TableHead>{t('First Used')}</TableHead>
                    <TableHead>{t('Last Used')}</TableHead>
                    <TableHead />
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {rows.map((row) => (
                    <TableRow key={row.channel_id}>
                      <TableCell className='font-medium whitespace-nowrap'>
                        <span className='text-muted-foreground'>
                          #{row.channel_id}
                        </span>
                        {row.channel_name && (
                          <span className='ml-1.5'>{row.channel_name}</span>
                        )}
                      </TableCell>
                      <TableCell className='text-right tabular-nums'>
                        {formatNumber(row.count)}
                      </TableCell>
                      <TableCell className='text-right tabular-nums'>
                        {summary.requests > 0
                          ? `${((row.count / summary.requests) * 100).toFixed(1)}%`
                          : '-'}
                      </TableCell>
                      <TableCell className='text-right tabular-nums'>
                        {row.rpm}
                      </TableCell>
                      <TableCell className='text-right tabular-nums'>
                        {formatTokens(row.tpm)}
                      </TableCell>
                      <TableCell className='text-right tabular-nums'>
                        {formatTokens(row.prompt_tokens)}
                      </TableCell>
                      <TableCell className='text-right tabular-nums'>
                        {formatTokens(row.completion_tokens)}
                      </TableCell>
                      <TableCell className='text-right tabular-nums'>
                        {formatLogQuota(row.quota)}
                      </TableCell>
                      <TableCell className='whitespace-nowrap tabular-nums'>
                        {formatTimestampToDate(row.first_at)}
                      </TableCell>
                      <TableCell className='whitespace-nowrap tabular-nums'>
                        {formatTimestampToDate(row.last_at)}
                      </TableCell>
                      <TableCell>
                        <Button
                          variant='ghost'
                          size='sm'
                          onClick={() => handleViewDetails(row.channel_id)}
                        >
                          {t('View details')}
                        </Button>
                      </TableCell>
                    </TableRow>
                  ))}
                  {rows.length === 0 && !statsQuery.isLoading && (
                    <TableRow>
                      <TableCell colSpan={11} className='py-8'>
                        <EmptyState
                          className='min-h-0 p-3'
                          title={t('No data')}
                        />
                      </TableCell>
                    </TableRow>
                  )}
                </TableBody>
              </Table>
            </div>
          )}
          <p className='text-muted-foreground text-xs'>
            {t('RPM/TPM are realtime rates over the last 60 seconds')}
          </p>
        </>
      )}
    </div>
  )
}
