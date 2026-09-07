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
import {
  fireEvent,
  render,
  screen,
  waitFor,
  within,
} from '@testing-library/react'
import i18n from 'i18next'
import { afterEach, expect, test, vi } from 'vitest'

import { api } from '@/lib/api'

import { MatrixCellEditor } from '../components/matrix-cell-editor'
import { createMatrixFixture, createMatrixQueryFixture } from './fixtures'

let queryFixture: ReturnType<typeof createMatrixQueryFixture>
afterEach(() => queryFixture?.client.clear())

function renderEditor(canWrite = true) {
  queryFixture = createMatrixQueryFixture()
  const close = vi.fn()
  const fixture = createMatrixFixture()
  render(
    <MatrixCellEditor
      cell={fixture.cells[0]}
      canWrite={canWrite}
      trigger={null}
      onClose={close}
    />,
    { wrapper: queryFixture.wrapper }
  )
  return { close, fixture }
}

test.each(['zhCN', 'zhTW'])(
  'renders effective values in the %s interface language',
  async (language) => {
    await i18n.changeLanguage(language)
    try {
      renderEditor()
      expect(screen.getByText('Effective: 500')).toBeVisible()
    } finally {
      await i18n.changeLanguage('en')
    }
  }
)

test('saves explicit zero and null inheritance for only the selected channel-model pair', async () => {
  const patch = vi
    .spyOn(api, 'patch')
    .mockResolvedValue({ data: { success: true, data: [] } })
  const { close } = renderEditor()
  expect(
    screen.getByText('Upstream model', { exact: false })
  ).toHaveTextContent('provider-a')
  expect(screen.getByLabelText('RPM')).toHaveValue('')
  expect(screen.getByLabelText('TPM')).toHaveValue('0')
  fireEvent.change(screen.getByLabelText('RPM'), { target: { value: '0' } })
  fireEvent.change(screen.getByLabelText('TPM'), { target: { value: '' } })
  fireEvent.change(screen.getByLabelText('Priority'), {
    target: { value: '12' },
  })
  fireEvent.change(screen.getByLabelText('Weight'), { target: { value: '30' } })
  fireEvent.click(screen.getByRole('button', { name: 'Save' }))
  await waitFor(() =>
    expect(patch).toHaveBeenCalledWith(
      '/api/channel/7/model-routing-overrides',
      {
        overrides: [
          {
            channel_id: 7,
            model: 'model-a',
            rpm_override: 0,
            tpm_override: null,
            priority_override: 12,
            weight_override: 30,
          },
        ],
      },
      expect.anything()
    )
  )
  expect(close).toHaveBeenCalledOnce()
})

test('rejects invalid fields with field-level errors and focuses the first error', async () => {
  const patch = vi.spyOn(api, 'patch')
  renderEditor()
  fireEvent.change(screen.getByLabelText('RPM'), { target: { value: '-1' } })
  fireEvent.change(screen.getByLabelText('TPM'), {
    target: { value: '9007199254740992' },
  })
  fireEvent.change(screen.getByLabelText('Weight'), {
    target: { value: '2.5' },
  })
  fireEvent.click(screen.getByRole('button', { name: 'Save' }))
  await waitFor(() =>
    expect(screen.getByLabelText('RPM')).toHaveAttribute('aria-invalid', 'true')
  )
  expect(screen.getByLabelText('TPM')).toHaveAttribute('aria-invalid', 'true')
  expect(screen.getByLabelText('Weight')).toHaveAttribute(
    'aria-invalid',
    'true'
  )
  await waitFor(() => expect(screen.getByLabelText('RPM')).toHaveFocus())
  expect(screen.getByLabelText('RPM')).toHaveAccessibleDescription(
    expect.stringContaining('Enter an integer')
  )
  expect(patch).not.toHaveBeenCalled()
})

test('reset restores inheritance and a failed save keeps drafts available for retry', async () => {
  const patch = vi
    .spyOn(api, 'patch')
    .mockResolvedValueOnce({ data: { success: false } })
    .mockResolvedValue({ data: { success: true, data: [] } })
  const { close } = renderEditor()
  fireEvent.click(
    screen.getByRole('button', { name: 'Reset to channel defaults' })
  )
  for (const label of ['RPM', 'TPM', 'Priority', 'Weight']) {
    expect(screen.getByLabelText(label)).toHaveValue('')
  }
  fireEvent.click(screen.getByRole('button', { name: 'Save' }))
  expect(await screen.findByRole('alert')).toHaveTextContent(
    'Failed to update model routing overrides.'
  )
  expect(close).not.toHaveBeenCalled()
  expect(screen.getByLabelText('TPM')).toHaveValue('')
  fireEvent.click(screen.getByRole('button', { name: 'Save' }))
  await waitFor(() => expect(close).toHaveBeenCalledOnce())
  expect(patch).toHaveBeenLastCalledWith(
    '/api/channel/7/model-routing-overrides',
    {
      overrides: [
        {
          channel_id: 7,
          model: 'model-a',
          rpm_override: null,
          tpm_override: null,
          priority_override: null,
          weight_override: null,
        },
      ],
    },
    expect.anything()
  )
})

test('read-only access exposes effective configuration without editable controls', () => {
  renderEditor(false)
  const dialog = screen.getByRole('dialog')
  expect(
    within(dialog).getByText('You have read-only access to routing overrides.')
  ).toBeVisible()
  for (const label of ['RPM', 'TPM', 'Priority', 'Weight']) {
    expect(screen.getByLabelText(label)).toBeDisabled()
  }
  expect(screen.queryByRole('button', { name: 'Save' })).not.toBeInTheDocument()
  expect(
    screen.queryByRole('button', { name: 'Reset to channel defaults' })
  ).not.toBeInTheDocument()
})
