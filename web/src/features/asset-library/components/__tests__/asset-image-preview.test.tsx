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
import {
  act,
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from '@testing-library/react'
import { afterEach, expect, it, vi } from 'vitest'

import { api } from '@/lib/api'

import type { Asset } from '../../types'
import { AssetLibraryProvider } from '../asset-library-provider'
import { AssetThumbnail } from '../asset-thumbnail'

vi.mock('react-i18next', () => ({
  useTranslation: () => ({ t: (key: string) => key }),
}))
afterEach(() => {
  cleanup()
  vi.unstubAllGlobals()
})
const asset: Asset = {
  Id: 'asset-na-http',
  Name: 'Portrait',
  URL: 'http://images.example/portrait.jpg',
  GroupId: 'group-na-1',
  AssetType: 'Image',
  Status: 'Failed',
  ProjectName: 'default',
  CreateTime: '',
  UpdateTime: '',
}
function renderThumbnail(targetUserId?: number, url = asset.URL) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  const view = render(
    <QueryClientProvider client={client}>
      <AssetLibraryProvider targetUserId={targetUserId}>
        <AssetThumbnail asset={{ ...asset, URL: url }} />
      </AssetLibraryProvider>
    </QueryClientProvider>
  )
  return view
}
it('loads HTTP images through an authenticated asset endpoint and releases the preview URL', async () => {
  const body = new Blob(['image'], { type: 'image/jpeg' })
  const get = vi.spyOn(api, 'get').mockResolvedValue({ data: body })
  const create = vi
    .spyOn(URL, 'createObjectURL')
    .mockReturnValue('blob:preview-image')
  const revoke = vi.spyOn(URL, 'revokeObjectURL').mockImplementation(() => {})
  const view = renderThumbnail()
  expect(screen.queryByRole('img')).toBeNull()
  await waitFor(() =>
    expect(screen.getByRole('img')).toHaveAttribute('src', 'blob:preview-image')
  )
  expect(get).toHaveBeenCalledWith(
    '/api/asset-library/assets/asset-na-http/preview',
    expect.objectContaining({ responseType: 'blob' })
  )
  expect(create).toHaveBeenCalledWith(body)
  view.unmount()
  expect(revoke).toHaveBeenCalledWith('blob:preview-image')
})
it('keeps administrator previews scoped to the selected owner', async () => {
  const get = vi
    .spyOn(api, 'get')
    .mockResolvedValue({ data: new Blob(['image'], { type: 'image/jpeg' }) })
  vi.spyOn(URL, 'createObjectURL').mockReturnValue('blob:owner-preview')
  vi.spyOn(URL, 'revokeObjectURL').mockImplementation(() => {})
  renderThumbnail(7)
  await waitFor(() =>
    expect(get).toHaveBeenCalledWith(
      '/api/asset-library/admin/users/7/assets/asset-na-http/preview',
      expect.objectContaining({ responseType: 'blob' })
    )
  )
})
it('shows a readable fallback when an image cannot be loaded', async () => {
  renderThumbnail(undefined, 'https://images.example/portrait.jpg')
  fireEvent.error(screen.getByRole('img'))
  expect(await screen.findByText('Failed to load asset preview.')).toBeVisible()
  expect(screen.queryByRole('img')).toBeNull()
})

it('does not fetch an HTTP thumbnail until it enters the viewport', async () => {
  let callback: IntersectionObserverCallback | undefined
  const disconnect = vi.fn()
  class Observer {
    constructor(cb: IntersectionObserverCallback) {
      callback = cb
    }
    observe = vi.fn()
    disconnect = disconnect
  }
  vi.stubGlobal('IntersectionObserver', Observer)
  const get = vi
    .spyOn(api, 'get')
    .mockResolvedValue({ data: new Blob(['image'], { type: 'image/jpeg' }) })
  vi.spyOn(URL, 'createObjectURL').mockReturnValue('blob:lazy-preview')
  vi.spyOn(URL, 'revokeObjectURL').mockImplementation(() => {})
  renderThumbnail()
  expect(get).not.toHaveBeenCalled()
  act(() =>
    callback?.(
      [{ isIntersecting: true } as IntersectionObserverEntry],
      {} as IntersectionObserver
    )
  )
  await waitFor(() =>
    expect(screen.getByRole('img')).toHaveAttribute('src', 'blob:lazy-preview')
  )
  expect(disconnect).toHaveBeenCalled()
})
