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
import { render, screen } from '@testing-library/react'
import type { ReactNode } from 'react'
import { describe, expect, test, vi } from 'vitest'

import type { Asset, AssetGroup } from '../../types'
import { AssetLibraryProvider } from '../asset-library-provider'
import { AssetsTable } from '../assets-table'
import { GroupsTable } from '../groups-table'

const groups: AssetGroup[] = [
  {
    Id: 'group-na-brand',
    Name: 'Brand assets',
    Description: 'Campaign and product launch assets',
    GroupType: 'AIGC',
    ProjectName: 'default',
    CreateTime: '2026-09-02T00:00:00Z',
    UpdateTime: '2026-09-02T00:00:00Z',
  },
]

const assets: Asset[] = [
  {
    Id: 'asset-na-hero',
    Name: 'Launch hero',
    URL: 'https://example.com/launch-hero.png',
    GroupId: groups[0].Id,
    AssetType: 'Image',
    Status: 'Active',
    ProjectName: 'default',
    CreateTime: '2026-09-02T00:00:00Z',
    UpdateTime: '2026-09-02T00:00:00Z',
  },
]

vi.mock('@tanstack/react-router', () => ({
  getRouteApi: () => ({
    useSearch: () => ({}),
    useNavigate: () => vi.fn(),
  }),
}))

vi.mock('../../api', () => ({
  listAllAssetGroups: async () => groups,
  listAssets: async () => ({
    TotalCount: assets.length,
    Items: assets,
    PageNumber: 1,
    PageSize: 20,
  }),
  listAssetGroups: async () => ({
    TotalCount: groups.length,
    Items: groups,
    PageNumber: 1,
    PageSize: 20,
  }),
}))

function TestProviders(props: { children: ReactNode }) {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })

  return (
    <QueryClientProvider client={queryClient}>
      <AssetLibraryProvider>{props.children}</AssetLibraryProvider>
    </QueryClientProvider>
  )
}

function expectContainerResponsiveGrid(container: HTMLElement) {
  const card = container.querySelector('[data-slot="data-table-card"]')
  expect(card).toBeInTheDocument()
  expect(card?.parentElement).toHaveClass(
    '[grid-template-columns:repeat(auto-fill,minmax(min(100%,15rem),1fr))]'
  )
}

describe('asset library card layout', () => {
  test('sizes asset columns from the available container width', async () => {
    const { container } = render(
      <TestProviders>
        <AssetsTable />
      </TestProviders>
    )

    await screen.findByText('Launch hero')
    expectContainerResponsiveGrid(container)
  })

  test('sizes group columns from the available container width', async () => {
    const { container } = render(
      <TestProviders>
        <GroupsTable />
      </TestProviders>
    )

    await screen.findByText('Brand assets')
    expectContainerResponsiveGrid(container)
  })
})
