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
import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import { DateTimePicker } from '@/components/datetime-picker'
import { ErrorState } from '@/components/error-state'
import { LoadingState } from '@/components/loading-state'
import { Button } from '@/components/ui/button'
import { Label } from '@/components/ui/label'
import dayjs from '@/lib/dayjs'
import { ROLE } from '@/lib/roles'
import { useAuthStore } from '@/stores/auth-store'

import { getGrossProfit } from './api'
import { GrossProfitResults } from './results'

function grossProfitDateRange(days: number) {
  return {
    start: dayjs()
      .subtract(days - 1, 'day')
      .startOf('day')
      .toDate(),
    end: dayjs().startOf('day').toDate(),
  }
}

export function GrossProfitAnalysis() {
  const { t } = useTranslation()
  const isRoot = useAuthStore(
    (state) => state.auth.user?.role === ROLE.SUPER_ADMIN
  )
  const [range, setRange] = useState(() => grossProfitDateRange(7))
  const [draft, setDraft] = useState<{ start?: Date; end?: Date }>(range)
  const valid =
    draft.start != null && draft.end != null && draft.end >= draft.start
  const query = useQuery({
    queryKey: ['gross-profit', range.start.getTime(), range.end.getTime()],
    queryFn: ({ signal }) =>
      getGrossProfit(
        dayjs(range.start).format('YYYY-MM-DD'),
        dayjs(range.end).add(1, 'day').format('YYYY-MM-DD'),
        signal
      ),
    enabled: isRoot,
    refetchInterval: (state) =>
      state.state.data &&
      (!state.state.data.aggregation.backfill_complete ||
        state.state.data.aggregation.pending_events > 0)
        ? 2000
        : false,
  })
  if (!isRoot) return null
  return (
    <div className='space-y-4'>
      <div className='flex flex-wrap items-end gap-3'>
        <div className='space-y-1.5'>
          <Label htmlFor='gross-profit-start'>{t('Start date')}</Label>
          <DateTimePicker
            id='gross-profit-start'
            aria-label={t('Start date')}
            dateOnly
            value={draft.start}
            onChange={(value) => setDraft({ ...draft, start: value })}
          />
        </div>
        <div className='space-y-1.5'>
          <Label htmlFor='gross-profit-end'>{t('End date')}</Label>
          <DateTimePicker
            id='gross-profit-end'
            aria-label={t('End date')}
            dateOnly
            value={draft.end}
            onChange={(value) => setDraft({ ...draft, end: value })}
          />
        </div>
        <Button
          disabled={!valid || query.isFetching}
          onClick={() => {
            if (!draft.start || !draft.end) return
            if (
              range.start.getTime() === draft.start.getTime() &&
              range.end.getTime() === draft.end.getTime()
            ) {
              void query.refetch()
            } else {
              setRange({ start: draft.start, end: draft.end })
            }
          }}
        >
          {t('Apply')}
        </Button>
        {[7, 30, 90].map((days) => (
          <Button
            key={days}
            variant='outline'
            onClick={() => {
              const next = grossProfitDateRange(days)
              setDraft(next)
              setRange(next)
            }}
          >
            {t('Last {{days}} days', { days })}
          </Button>
        ))}
      </div>
      {!valid && (
        <p role='alert' className='text-destructive text-sm'>
          {t('Select a valid date range')}
        </p>
      )}
      {query.isPending && <LoadingState />}
      {query.isError && (
        <ErrorState
          description={query.error.message}
          onRetry={() => void query.refetch()}
        />
      )}
      {query.isSuccess && (
        <>
          <p className='text-muted-foreground text-sm'>
            {t('Daily totals use {{timezone}}.', {
              timezone: query.data.timezone,
            })}
          </p>
          {!query.data.aggregation.backfill_complete && (
            <p role='status' className='text-muted-foreground text-sm'>
              {t(
                'Historical aggregation is in progress. Totals may be incomplete.'
              )}
            </p>
          )}
          {query.data.aggregation.pending_events > 0 && (
            <p role='status' className='text-muted-foreground text-sm'>
              {t('{{count}} calls are pending aggregation.', {
                count: query.data.aggregation.pending_events,
              })}
            </p>
          )}
          <GrossProfitResults report={query.data} />
        </>
      )}
    </div>
  )
}
