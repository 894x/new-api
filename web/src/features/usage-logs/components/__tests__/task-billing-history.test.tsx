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
import { fireEvent, render, screen, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { beforeEach, expect, it, vi } from 'vitest'

import { api } from '@/lib/api'

import type { TaskBillingHistory } from '../../types'
import { TaskBillingHistoryButton } from '../task-billing-history'

vi.mock('@/lib/api', () => ({ api: { get: vi.fn() } }))

const history: TaskBillingHistory = {
  task_id: 'task_refunded',
  submit_request_id: 'original-submit-id',
  status: 'FAILURE',
  task_quota: 0,
  charged_quota: 506849,
  refunded_quota: 506849,
  net_quota: 0,
  reconciled: true,
  truncated: false,
  entries: [
    {
      request_id: 'original-submit-id',
      created_at: 1792415004,
      kind: 'initial_charge',
      quota: 506849,
    },
    {
      request_id: 'different-refund-id',
      created_at: 1792415204,
      kind: 'refund',
      quota: 506849,
    },
  ],
}

function renderHistory(
  props: Partial<Parameters<typeof TaskBillingHistoryButton>[0]> = {}
) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  return render(
    <QueryClientProvider client={client}>
      <TaskBillingHistoryButton
        taskId='task_refunded'
        userId={74}
        isAdmin={false}
        {...props}
      />
    </QueryClientProvider>
  )
}

beforeEach(() => {
  vi.mocked(api.get).mockReset()
})

it('loads on demand and connects a refund request to the original charge and confirmed net amount', async () => {
  vi.mocked(api.get).mockResolvedValue({
    data: { success: true, data: history },
  })
  renderHistory({ taskId: undefined, requestId: 'different-refund-id' })
  expect(api.get).not.toHaveBeenCalled()
  await userEvent.click(screen.getByRole('button', { name: 'Related billing' }))
  expect(await screen.findByText('Fully refunded')).toBeVisible()
  expect(screen.getByRole('combobox', { name: 'Search by' })).toHaveTextContent(
    'Request ID'
  )
  const records = screen.getByRole('list', { name: 'Billing records' })
  expect(within(records).getByText('original-submit-id')).toBeVisible()
  expect(within(records).getByText('different-refund-id')).toBeVisible()
  expect(api.get).toHaveBeenCalledWith(
    '/api/log/task-billing',
    expect.objectContaining({
      params: { request_id: 'different-refund-id', user_id: 74 },
    })
  )
  expect(
    screen.getByRole('button', { name: 'Copy submit request ID' })
  ).toBeEnabled()
})

it('shows the extra charge and positive net amount without claiming a refund', async () => {
  vi.mocked(api.get).mockResolvedValue({
    data: {
      success: true,
      data: {
        ...history,
        status: 'SUCCESS',
        task_quota: 214968,
        charged_quota: 214968,
        refunded_quota: 0,
        net_quota: 214968,
        entries: [
          history.entries[0],
          {
            request_id: 'extra-charge-id',
            created_at: 2,
            kind: 'additional_charge',
            quota: 49558,
          },
        ],
      },
    },
  })
  renderHistory({ requestId: 'extra-charge-id', taskId: undefined })
  await userEvent.click(screen.getByRole('button', { name: 'Related billing' }))
  expect(await screen.findByText('Additional task charge')).toBeVisible()
  expect(screen.queryByText('Fully refunded')).not.toBeInTheDocument()
})

it('supports manual request ID lookup without inheriting unrelated list filters', async () => {
  vi.mocked(api.get).mockResolvedValue({
    data: { success: true, data: history },
  })
  renderHistory({ taskId: undefined, userId: undefined, isAdmin: true })
  await userEvent.click(screen.getByRole('button', { name: 'Related billing' }))
  expect(api.get).not.toHaveBeenCalled()
  await userEvent.type(
    screen.getByRole('textbox', { name: 'Request ID' }),
    'different-refund-id'
  )
  await userEvent.click(screen.getByRole('button', { name: /^Search$/ }))
  expect(await screen.findByText('Fully refunded')).toBeVisible()
})

it('shows a loading state, handles failures and retries the same task', async () => {
  let fail: (reason: Error) => void = () => undefined
  vi.mocked(api.get).mockImplementationOnce(
    () =>
      new Promise((_resolve, reject) => {
        fail = reject
      })
  )
  renderHistory()
  await userEvent.click(screen.getByRole('button', { name: 'Related billing' }))
  expect(screen.getByText('Loading...')).toBeVisible()
  fail(new Error('offline'))
  expect(
    await screen.findByText('Failed to load related billing')
  ).toBeVisible()
  vi.mocked(api.get).mockResolvedValue({
    data: { success: true, data: history },
  })
  await userEvent.click(screen.getByRole('button', { name: 'Retry' }))
  expect(await screen.findByText('Fully refunded')).toBeVisible()
})

it('does not claim a refund for incomplete or missing history and wraps long identifiers', async () => {
  const longId = `request-${'a'.repeat(150)}`
  vi.mocked(api.get).mockResolvedValue({
    data: {
      success: true,
      data: {
        ...history,
        reconciled: false,
        truncated: true,
        entries: [{ ...history.entries[0], request_id: longId }],
      },
    },
  })
  renderHistory()
  await userEvent.click(screen.getByRole('button', { name: 'Related billing' }))
  expect(await screen.findByRole('alert')).toHaveTextContent(
    'Billing history is incomplete'
  )
  expect(screen.queryByText('Fully refunded')).not.toBeInTheDocument()
  expect(screen.getByText(longId)).toHaveClass('break-all')
  vi.mocked(api.get).mockResolvedValue({
    data: {
      success: true,
      data: { ...history, reconciled: false, entries: [] },
    },
  })
  fireEvent.click(screen.getByRole('button', { name: 'Refresh' }))
  expect(await screen.findByText('No billing records found')).toBeVisible()
  expect(screen.queryByText('Fully refunded')).not.toBeInTheDocument()
})
