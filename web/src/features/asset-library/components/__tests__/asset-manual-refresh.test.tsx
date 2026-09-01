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
import type { Row } from '@tanstack/react-table'
import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import type { ButtonHTMLAttributes, ReactNode } from 'react'
import { afterEach, beforeEach, describe, expect, test, vi } from 'vitest'

import type { Asset } from '../../types'

const { toastError, toastSuccess, toastWarning } = vi.hoisted(() => ({
  toastError: vi.fn(),
  toastSuccess: vi.fn(),
  toastWarning: vi.fn(),
}))

vi.mock('sonner', () => ({
  toast: {
    error: toastError,
    success: toastSuccess,
    warning: toastWarning,
  },
}))

vi.mock('@/components/ui/dropdown-menu', () => ({
  DropdownMenu: (props: { children: ReactNode }) => <div>{props.children}</div>,
  DropdownMenuContent: (props: { children: ReactNode }) => (
    <div>{props.children}</div>
  ),
  DropdownMenuItem: (props: ButtonHTMLAttributes<HTMLButtonElement>) => (
    <button type='button' {...props} />
  ),
  DropdownMenuShortcut: (props: { children: ReactNode }) => (
    <span>{props.children}</span>
  ),
  DropdownMenuTrigger: (props: { children: ReactNode }) => (
    <div>{props.children}</div>
  ),
}))

const { createInstance } = await import('i18next')
const { I18nextProvider, initReactI18next } = await import('react-i18next')
const { QueryClient, QueryClientProvider } =
  await import('@tanstack/react-query')
const { api } = await import('@/lib/api')
const { getAsset, listAllAssets } = await import('../../api')
const { assetLibraryQueryKeys } = await import('../../lib')
const { AssetLibraryProvider } = await import('../asset-library-provider')
const { AssetLibraryPrimaryButtons } =
  await import('../asset-library-primary-buttons')
const { AssetRowActions } = await import('../asset-row-actions')

const i18n = createInstance()
await i18n.use(initReactI18next).init({
  lng: 'en',
  resources: { en: { translation: {} } },
})

type ApiPost = (
  url: string,
  data?: unknown,
  config?: { params?: { Action?: string } }
) => Promise<{ data: unknown }>

const apiClient = api as unknown as { post: ApiPost }
const originalPost = apiClient.post
let queryClient: InstanceType<typeof QueryClient> | null = null

const assets = Array.from({ length: 5 }, (_, index) => ({
  Id: `asset-${index + 1}`,
  Name: `Asset ${index + 1}`,
  URL: `https://example.com/${index + 1}.png`,
  GroupId: 'group-one',
  AssetType: 'Image' as const,
  Status: 'Processing',
  ProjectName: 'default',
  CreateTime: '2026-09-02T00:00:00Z',
  UpdateTime: '2026-09-02T00:00:00Z',
}))

function renderWithProviders(ui: ReactNode) {
  queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  })
  return render(
    <QueryClientProvider client={queryClient}>
      <I18nextProvider i18n={i18n}>
        <AssetLibraryProvider>{ui}</AssetLibraryProvider>
      </I18nextProvider>
    </QueryClientProvider>
  )
}

beforeEach(() => {
  toastError.mockReset()
  toastSuccess.mockReset()
  toastWarning.mockReset()
})

afterEach(() => {
  apiClient.post = originalPost
  queryClient?.clear()
  queryClient = null
})

describe('manual asset status refresh', () => {
  test('refreshes one asset and updates every cached asset list', async () => {
    const asset = assets[0]
    apiClient.post = async (_url, data, config) => {
      expect(config?.params?.Action).toBe('GetAsset')
      expect(data).toEqual({ Id: asset.Id })
      return { data: { Result: { ...asset, Status: 'Active' } } }
    }
    renderWithProviders(
      <AssetRowActions row={{ original: asset } as Row<Asset>} />
    )
    queryClient?.setQueryData(
      assetLibraryQueryKeys.assetList({ PageNumber: 1 }),
      { TotalCount: 1, Items: [asset], PageNumber: 1, PageSize: 20 }
    )

    fireEvent.click(screen.getByRole('button', { name: 'Refresh asset' }))

    await waitFor(() => {
      const cached = queryClient?.getQueryData<{
        Items: Array<{ Status?: string }>
      }>(assetLibraryQueryKeys.assetList({ PageNumber: 1 }))
      expect(cached?.Items[0]?.Status).toBe('Active')
    })
    expect(toastSuccess).toHaveBeenCalledWith('Asset status refreshed.')
  })

  test('keeps the asset refresh action usable after a request failure', async () => {
    apiClient.post = async () => {
      throw new Error('refresh unavailable')
    }
    renderWithProviders(
      <AssetRowActions row={{ original: assets[0] } as Row<Asset>} />
    )

    const refreshButton = screen.getByRole('button', { name: 'Refresh asset' })
    fireEvent.click(refreshButton)

    await waitFor(() => {
      expect(toastError).toHaveBeenCalledWith('refresh unavailable')
    })
    expect(refreshButton).toBeEnabled()
  })

  test('shares an in-flight asset request with the automatic status query', async () => {
    let getAssetCalls = 0
    let releaseRequest: (() => void) | undefined
    const requestGate = new Promise<void>((resolve) => {
      releaseRequest = resolve
    })
    apiClient.post = async () => {
      getAssetCalls += 1
      await requestGate
      return { data: { Result: { ...assets[0], Status: 'Active' } } }
    }
    renderWithProviders(
      <AssetRowActions row={{ original: assets[0] } as Row<Asset>} />
    )
    const automaticRefresh = queryClient?.fetchQuery({
      queryKey: assetLibraryQueryKeys.asset(assets[0].Id),
      queryFn: () => getAsset(assets[0].Id),
      staleTime: 0,
    })
    await waitFor(() => expect(getAssetCalls).toBe(1))

    const refreshButton = screen.getByRole('button', { name: 'Refresh asset' })
    fireEvent.click(refreshButton)
    await waitFor(() => expect(refreshButton).toBeDisabled())
    expect(getAssetCalls).toBe(1)

    releaseRequest?.()
    await automaticRefresh
    await waitFor(() => {
      expect(toastSuccess).toHaveBeenCalledWith('Asset status refreshed.')
    })
  })

  test('enumerates asset-list pages sequentially before a library refresh', async () => {
    let activePageRequests = 0
    let maxActivePageRequests = 0
    let releasePages: (() => void) | undefined
    const pageGate = new Promise<void>((resolve) => {
      releasePages = resolve
    })
    apiClient.post = async (_url, data, config) => {
      expect(config?.params?.Action).toBe('ListAssets')
      const pageNumber = (data as { PageNumber: number }).PageNumber
      if (pageNumber === 1) {
        return {
          data: {
            Result: {
              TotalCount: 201,
              Items: [assets[0]],
              PageNumber: 1,
              PageSize: 100,
            },
          },
        }
      }
      activePageRequests += 1
      maxActivePageRequests = Math.max(
        maxActivePageRequests,
        activePageRequests
      )
      await pageGate
      activePageRequests -= 1
      return {
        data: {
          Result: {
            TotalCount: 201,
            Items: [],
            PageNumber: pageNumber,
            PageSize: 100,
          },
        },
      }
    }

    const allAssetsPromise = listAllAssets()
    await waitFor(() => expect(activePageRequests).toBeGreaterThan(0))
    expect(maxActivePageRequests).toBe(1)

    releasePages?.()
    await allAssetsPromise
    expect(maxActivePageRequests).toBe(1)
  })

  test('refreshes the whole library with bounded request concurrency', async () => {
    let activeRequests = 0
    let maxActiveRequests = 0
    let getAssetCalls = 0
    let releaseRequests: (() => void) | undefined
    const requestGate = new Promise<void>((resolve) => {
      releaseRequests = resolve
    })
    apiClient.post = async (_url, data, config) => {
      if (config?.params?.Action === 'ListAssets') {
        return {
          data: {
            Result: {
              TotalCount: assets.length,
              Items: assets,
              PageNumber: 1,
              PageSize: 100,
            },
          },
        }
      }
      if (config?.params?.Action !== 'GetAsset') {
        throw new Error(`Unexpected action ${config?.params?.Action}`)
      }
      getAssetCalls += 1
      activeRequests += 1
      maxActiveRequests = Math.max(maxActiveRequests, activeRequests)
      await requestGate
      activeRequests -= 1
      const id = (data as { Id: string }).Id
      return {
        data: {
          Result: {
            ...assets.find((asset) => asset.Id === id),
            Status: 'Active',
          },
        },
      }
    }
    renderWithProviders(<AssetLibraryPrimaryButtons />)

    const refreshButton = screen.getByRole('button', {
      name: 'Refresh asset library',
    })
    fireEvent.click(refreshButton)

    await waitFor(() => expect(getAssetCalls).toBe(4))
    expect(refreshButton).toBeDisabled()
    expect(maxActiveRequests).toBe(4)

    releaseRequests?.()
    await waitFor(() => expect(getAssetCalls).toBe(assets.length))
    await waitFor(() => expect(refreshButton).toBeEnabled())
    expect(maxActiveRequests).toBe(4)
    expect(toastSuccess).toHaveBeenCalledWith('Asset library refreshed.')
  })
})
