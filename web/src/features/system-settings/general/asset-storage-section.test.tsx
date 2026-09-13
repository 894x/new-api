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
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from '@testing-library/react'
import { afterEach, expect, it, vi } from 'vitest'

import { SettingsPageProvider } from '../components/settings-page-context'
import { AssetStorageSection } from './asset-storage-section'

const mutateAsync = vi.hoisted(() => vi.fn())
vi.mock('../hooks/use-update-option', () => ({
  useUpdateOption: () => ({ mutateAsync }),
}))
vi.mock('react-i18next', () => ({
  useTranslation: () => ({ t: (key: string) => key }),
}))
afterEach(() => {
  cleanup()
  vi.clearAllMocks()
})

it('saves an integer MB quota and preserves unsaved values when the server rejects it', async () => {
  const actions = document.createElement('div')
  document.body.append(actions)
  const view = render(
    <SettingsPageProvider actionsContainer={actions}>
      <AssetStorageSection quotaMB={1024} />
    </SettingsPageProvider>
  )
  const input = screen.getByRole('spinbutton', {
    name: 'Default free asset quota (MB)',
  })
  expect(input).toHaveValue(1024)
  fireEvent.change(input, { target: { value: '512' } })
  mutateAsync.mockResolvedValueOnce({ success: false })
  fireEvent.click(screen.getByRole('button', { name: 'Save Changes' }))
  await waitFor(() =>
    expect(mutateAsync).toHaveBeenCalledWith({
      key: 'asset_storage_setting.default_quota_mb',
      value: '512',
    })
  )
  await waitFor(() =>
    expect(screen.getByRole('button', { name: 'Save Changes' })).toBeEnabled()
  )
  mutateAsync.mockResolvedValueOnce({ success: true })
  fireEvent.click(screen.getByRole('button', { name: 'Save Changes' }))
  await waitFor(() =>
    expect(screen.getByRole('button', { name: 'Save Changes' })).toBeDisabled()
  )
  await waitFor(() => expect(input).toBeEnabled())
  fireEvent.change(input, { target: { value: '0' } })
  await waitFor(() =>
    expect(screen.getByRole('button', { name: 'Save Changes' })).toBeEnabled()
  )
  mutateAsync.mockResolvedValueOnce({ success: true })
  fireEvent.click(screen.getByRole('button', { name: 'Save Changes' }))
  await waitFor(() =>
    expect(mutateAsync).toHaveBeenLastCalledWith({
      key: 'asset_storage_setting.default_quota_mb',
      value: '0',
    })
  )
  view.unmount()
  actions.remove()
})
