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
import { act, fireEvent, render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { useState } from 'react'
import { afterEach, expect, test, vi } from 'vitest'

import { api } from '@/lib/api'
import { ROLE } from '@/lib/roles'
import { useAuthStore } from '@/stores/auth-store'

import { matrixQueryKeys } from '../api'
import { MatrixWorkspace } from '../components/matrix-workspace'
import { matrixSearchSchema, type MatrixData } from '../types'
import { createMatrixFixture, createMatrixQueryFixture } from './fixtures'

let queryFixture: ReturnType<typeof createMatrixQueryFixture>
afterEach(() => {
  queryFixture?.client.clear()
  useAuthStore.getState().auth.setUser(null)
})

function Workspace() {
  const [search, setSearch] = useState(matrixSearchSchema.parse({}))
  return <MatrixWorkspace search={search} onSearchChange={setSearch} />
}

function renderWorkspace() {
  queryFixture = createMatrixQueryFixture()
  render(<Workspace />, { wrapper: queryFixture.wrapper })
}

function rootUser() {
  useAuthStore
    .getState()
    .auth.setUser({ id: 1, username: 'root', role: ROLE.SUPER_ADMIN })
}

test('defaults to enabled channels and sends search, group and both page controls to the aggregate endpoint', async () => {
  rootUser()
  const fixture = createMatrixFixture()
  fixture.model_total = 26
  fixture.channel_total = 11
  const get = vi
    .spyOn(api, 'get')
    .mockResolvedValue({ data: { success: true, data: fixture } })
  renderWorkspace()
  await screen.findByRole('table')
  expect(get).toHaveBeenLastCalledWith(
    '/api/channel/model-matrix',
    expect.objectContaining({
      params: expect.objectContaining({
        status: 'enabled',
        model_page: 1,
        channel_page: 1,
      }),
    })
  )
  fireEvent.click(screen.getByRole('button', { name: 'Next models' }))
  await waitFor(() =>
    expect(get).toHaveBeenLastCalledWith(
      '/api/channel/model-matrix',
      expect.objectContaining({
        params: expect.objectContaining({ model_page: 2 }),
      })
    )
  )
  await waitFor(() =>
    expect(screen.getByRole('button', { name: 'Next channels' })).toBeEnabled()
  )
  fireEvent.click(screen.getByRole('button', { name: 'Next channels' }))
  await waitFor(() =>
    expect(get).toHaveBeenLastCalledWith(
      '/api/channel/model-matrix',
      expect.objectContaining({
        params: expect.objectContaining({ channel_page: 2 }),
      })
    )
  )
  fireEvent.change(screen.getByLabelText('Model'), {
    target: { value: 'model-a' },
  })
  fireEvent.change(screen.getByLabelText('Channel'), {
    target: { value: 'Alpha' },
  })
  fireEvent.change(screen.getByLabelText('Group'), {
    target: { value: 'default' },
  })
  fireEvent.click(screen.getByRole('button', { name: 'Search' }))
  await waitFor(() =>
    expect(get).toHaveBeenLastCalledWith(
      '/api/channel/model-matrix',
      expect.objectContaining({
        params: expect.objectContaining({
          model: 'model-a',
          channel: 'Alpha',
          group: 'default',
          model_page: 1,
          channel_page: 1,
        }),
      })
    )
  )
  fireEvent.click(screen.getByRole('button', { name: 'All' }))
  await waitFor(() =>
    expect(get).toHaveBeenLastCalledWith(
      '/api/channel/model-matrix',
      expect.objectContaining({
        params: expect.objectContaining({ status: 'all' }),
      })
    )
  )
})

test('opens a cell by keyboard, refreshes saved values and restores focus when closed', async () => {
  rootUser()
  const fixture = createMatrixFixture()
  let finishRefresh!: (value: {
    data: { success: boolean; data: MatrixData }
  }) => void
  const pendingRefresh = new Promise<{
    data: { success: boolean; data: MatrixData }
  }>((resolve) => {
    finishRefresh = resolve
  })
  const get = vi
    .spyOn(api, 'get')
    .mockResolvedValueOnce({
      data: { success: true, data: structuredClone(fixture) },
    })
    .mockReturnValueOnce(pendingRefresh)
  vi.spyOn(api, 'patch').mockResolvedValue({
    data: { success: true, data: [] },
  })
  renderWorkspace()
  const cell = await screen.findByRole('button', {
    name: 'Configure model-a on Alpha (#7)',
  })
  cell.focus()
  await userEvent.keyboard('{Enter}')
  expect(await screen.findByRole('dialog')).toBeVisible()
  fireEvent.change(screen.getByLabelText('RPM'), { target: { value: '42' } })
  fireEvent.click(screen.getByRole('button', { name: 'Save' }))
  await waitFor(() => expect(get).toHaveBeenCalledTimes(2))
  expect(screen.getByRole('dialog')).toBeVisible()
  expect(screen.getByRole('button', { name: 'Saving...' })).toBeDisabled()
  fixture.cells[0].effective_rpm = 42
  fixture.cells[0].rpm_override = 42
  await act(async () => {
    finishRefresh({ data: { success: true, data: fixture } })
  })
  await waitFor(() =>
    expect(screen.queryByRole('dialog')).not.toBeInTheDocument()
  )
  await waitFor(() => expect(cell).toHaveTextContent('42'))
  await waitFor(() => expect(cell).toHaveFocus())
})

test('refreshes cached matrix on return even inside the global fresh-cache period', async () => {
  rootUser()
  queryFixture = createMatrixQueryFixture()
  queryFixture.client.setQueryDefaults(matrixQueryKeys.all, {
    staleTime: 10000,
  })
  const oldData = createMatrixFixture()
  queryFixture.client.setQueryData(
    matrixQueryKeys.list(matrixSearchSchema.parse({})),
    oldData
  )
  const newData = createMatrixFixture()
  newData.channels = [newData.channels[1]]
  newData.cells = [newData.cells[1]]
  newData.models = ['model-b']
  newData.channel_total = 1
  newData.model_total = 1
  const get = vi
    .spyOn(api, 'get')
    .mockResolvedValue({ data: { success: true, data: newData } })
  render(<Workspace />, { wrapper: queryFixture.wrapper })
  await waitFor(() => expect(get).toHaveBeenCalled())
  await waitFor(() =>
    expect(
      screen.queryByRole('columnheader', { name: /Alpha/ })
    ).not.toBeInTheDocument()
  )
  expect(screen.getByRole('columnheader', { name: /Beta/ })).toBeVisible()
})

test('shows a retryable query error and then an actionable empty state', async () => {
  rootUser()
  const fixture = createMatrixFixture()
  fixture.models = []
  fixture.cells = []
  fixture.model_total = 0
  vi.spyOn(api, 'get')
    .mockRejectedValueOnce(new Error('network unavailable'))
    .mockResolvedValue({ data: { success: true, data: fixture } })
  renderWorkspace()
  expect(await screen.findByRole('alert')).toHaveTextContent(
    'Failed to load model-channel matrix.'
  )
  fireEvent.click(screen.getByRole('button', { name: 'Retry' }))
  expect(
    await screen.findByText('No matching model-channel configurations')
  ).toBeVisible()
  expect(
    screen.getByText('Change the filters or configure models in Channels.')
  ).toBeVisible()
})

test('does not fetch or render channel configuration without channel read permission', () => {
  useAuthStore
    .getState()
    .auth.setUser({ id: 2, username: 'admin', role: ROLE.ADMIN })
  const get = vi.spyOn(api, 'get')
  renderWorkspace()
  expect(screen.getByText("You don't have necessary permission")).toBeVisible()
  expect(get).not.toHaveBeenCalled()
  expect(screen.queryByRole('table')).not.toBeInTheDocument()
})
