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
import { act, render, screen } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, test, vi } from 'vitest'

import type { Asset } from '../../types'

const { navigate } = vi.hoisted(() => ({ navigate: vi.fn() }))

vi.mock('@tanstack/react-router', async () => {
  const actual = await vi.importActual<typeof import('@tanstack/react-router')>(
    '@tanstack/react-router'
  )
  return {
    ...actual,
    getRouteApi: () => ({
      useSearch: () => ({}),
      useNavigate: () => navigate,
    }),
  }
})

vi.mock('@/hooks/use-table-url-state', () => ({
  useTableUrlState: () => ({
    globalFilter: '',
    onGlobalFilterChange: () => undefined,
    columnFilters: [],
    onColumnFiltersChange: () => undefined,
    pagination: { pageIndex: 0, pageSize: 20 },
    onPaginationChange: () => undefined,
    ensurePageInRange: () => undefined,
  }),
}))

vi.mock('@/components/data-table', () => ({
  useDataTable: (props: { data: Array<{ Id: string; Status?: string }> }) => ({
    table: { data: props.data },
  }),
  DataTablePage: (props: {
    table: { data: Array<{ Id: string; Status?: string }> }
  }) => (
    <div>
      {props.table.data.map((asset) => (
        <span key={asset.Id}>{asset.Status}</span>
      ))}
    </div>
  ),
}))

const { createInstance } = await import('i18next')
const { I18nextProvider, initReactI18next } = await import('react-i18next')
const { focusManager, QueryClient, QueryClientProvider } =
  await import('@tanstack/react-query')
const { api } = await import('@/lib/api')
const { AssetLibraryProvider } = await import('../asset-library-provider')
const { AssetsTable } = await import('../assets-table')

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
let assetRefreshCount = 0
let assetStaysProcessing = false
let assetRefreshFails = false

const processingAsset: Asset = {
  Id: 'asset-na-processing',
  Name: 'Processing image',
  URL: 'https://example.com/processing.png',
  GroupId: 'group-na-one',
  AssetType: 'Image',
  Status: 'Processing',
  ProjectName: 'default',
  CreateTime: '2026-09-02T00:00:00Z',
  UpdateTime: '2026-09-02T00:00:00Z',
}
let listedAsset: Asset = processingAsset
let refreshedAssetOverride: Asset | undefined

beforeEach(() => {
  navigate.mockReset()
  assetRefreshCount = 0
  assetStaysProcessing = false
  assetRefreshFails = false
  listedAsset = processingAsset
  refreshedAssetOverride = undefined
  apiClient.post = async (_url, _data, config) => {
    switch (config?.params?.Action) {
      case 'ListAssetGroups':
        return {
          data: {
            Result: { TotalCount: 0, Items: [], PageNumber: 1, PageSize: 100 },
          },
        }
      case 'ListAssets':
        return {
          data: {
            Result: {
              TotalCount: 1,
              Items: [listedAsset],
              PageNumber: 1,
              PageSize: 20,
            },
          },
        }
      case 'GetAsset':
        assetRefreshCount += 1
        if (assetRefreshFails) {
          throw new Error('Upstream status is temporarily unavailable')
        }
        return {
          data: {
            Result:
              refreshedAssetOverride ??
              ({
                ...processingAsset,
                Status:
                  assetStaysProcessing || assetRefreshCount === 1
                    ? 'Processing'
                    : 'Active',
              } as Asset),
          },
        }
      default:
        throw new Error(
          `Unexpected asset-library action ${config?.params?.Action}`
        )
    }
  }
})

afterEach(() => {
  apiClient.post = originalPost
  queryClient?.clear()
  queryClient = null
  localStorage.clear()
  focusManager.setFocused(undefined)
  vi.useRealTimers()
})

describe('asset status refresh', () => {
  test('updates a processing asset to its terminal status without reloading the page', async () => {
    vi.useFakeTimers()
    queryClient = new QueryClient({
      defaultOptions: { queries: { retry: false, staleTime: 0 } },
    })

    render(
      <QueryClientProvider client={queryClient}>
        <I18nextProvider i18n={i18n}>
          <AssetLibraryProvider>
            <AssetsTable />
          </AssetLibraryProvider>
        </I18nextProvider>
      </QueryClientProvider>
    )

    await act(async () => {
      await vi.advanceTimersByTimeAsync(0)
    })
    expect(screen.getByText('Processing')).toBeInTheDocument()

    await act(async () => {
      await vi.advanceTimersByTimeAsync(5_000)
    })
    expect(screen.getByText('Active')).toBeInTheDocument()

    await act(async () => {
      await vi.advanceTimersByTimeAsync(20_000)
    })
    expect(assetRefreshCount).toBe(2)
  })

  test('stops polling an asset that remains processing indefinitely', async () => {
    vi.useFakeTimers()
    assetStaysProcessing = true
    queryClient = new QueryClient({
      defaultOptions: { queries: { retry: false, staleTime: 0 } },
    })

    render(
      <QueryClientProvider client={queryClient}>
        <I18nextProvider i18n={i18n}>
          <AssetLibraryProvider>
            <AssetsTable />
          </AssetLibraryProvider>
        </I18nextProvider>
      </QueryClientProvider>
    )

    await act(async () => {
      await vi.advanceTimersByTimeAsync(6 * 60_000)
    })
    const refreshCountAfterSixMinutes = assetRefreshCount
    expect(refreshCountAfterSixMinutes).toBe(30)

    focusManager.setFocused(false)
    focusManager.setFocused(true)
    await act(async () => {
      await vi.advanceTimersByTimeAsync(0)
    })
    expect(assetRefreshCount).toBe(refreshCountAfterSixMinutes)

    await act(async () => {
      await vi.advanceTimersByTimeAsync(6 * 60_000)
    })
    expect(assetRefreshCount).toBe(refreshCountAfterSixMinutes)
  })

  test('stops polling and focus refresh after five consecutive failures', async () => {
    vi.useFakeTimers()
    assetRefreshFails = true
    queryClient = new QueryClient({
      defaultOptions: { queries: { retry: false, staleTime: 0 } },
    })

    render(
      <QueryClientProvider client={queryClient}>
        <I18nextProvider i18n={i18n}>
          <AssetLibraryProvider>
            <AssetsTable />
          </AssetLibraryProvider>
        </I18nextProvider>
      </QueryClientProvider>
    )

    await act(async () => {
      await vi.advanceTimersByTimeAsync(2 * 60_000)
    })
    expect(assetRefreshCount).toBe(5)

    focusManager.setFocused(false)
    focusManager.setFocused(true)
    await act(async () => {
      await vi.advanceTimersByTimeAsync(5 * 60_000)
    })
    expect(assetRefreshCount).toBe(5)
  })

  test('starts a fresh polling budget when a terminal asset begins processing again', async () => {
    vi.useFakeTimers()
    assetStaysProcessing = true
    queryClient = new QueryClient({
      defaultOptions: { queries: { retry: false, staleTime: 0 } },
    })

    render(
      <QueryClientProvider client={queryClient}>
        <I18nextProvider i18n={i18n}>
          <AssetLibraryProvider>
            <AssetsTable />
          </AssetLibraryProvider>
        </I18nextProvider>
      </QueryClientProvider>
    )

    await act(async () => {
      await vi.advanceTimersByTimeAsync(6 * 60_000)
    })
    expect(assetRefreshCount).toBe(30)

    await act(async () => {
      queryClient?.setQueriesData(
        { queryKey: ['asset-library', 'assets', 'list'] },
        (page: unknown) => ({
          ...(page as object),
          Items: [{ ...processingAsset, Status: 'Active' }],
        })
      )
      await vi.advanceTimersByTimeAsync(0)
    })
    expect(screen.getByText('Active')).toBeInTheDocument()

    await act(async () => {
      queryClient?.setQueriesData(
        { queryKey: ['asset-library', 'assets', 'list'] },
        (page: unknown) => ({
          ...(page as object),
          Items: [processingAsset],
        })
      )
      await vi.advanceTimersByTimeAsync(5_000)
    })
    expect(assetRefreshCount).toBeGreaterThan(30)
  })

  test('polls an asset while an administrator-visible replica is processing', async () => {
    vi.useFakeTimers()
    listedAsset = {
      ...processingAsset,
      Status: 'Active',
      Replication: {
        Status: 'Processing',
        Total: 1,
        Ready: 0,
        Processing: 1,
        Failed: 0,
      },
    }
    refreshedAssetOverride = {
      ...processingAsset,
      Status: 'Active',
      Replication: {
        Status: 'Ready',
        Total: 1,
        Ready: 1,
        Processing: 0,
        Failed: 0,
      },
    }
    queryClient = new QueryClient({
      defaultOptions: { queries: { retry: false, staleTime: 0 } },
    })

    render(
      <QueryClientProvider client={queryClient}>
        <I18nextProvider i18n={i18n}>
          <AssetLibraryProvider>
            <AssetsTable />
          </AssetLibraryProvider>
        </I18nextProvider>
      </QueryClientProvider>
    )

    await act(async () => {
      await vi.advanceTimersByTimeAsync(20_000)
    })
    expect(assetRefreshCount).toBe(1)
  })
})
