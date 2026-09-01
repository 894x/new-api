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
import assert from 'node:assert/strict'

import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import type { Row } from '@tanstack/react-table'
import { createInstance } from 'i18next'
import { renderToStaticMarkup } from 'react-dom/server'
import { I18nextProvider, initReactI18next } from 'react-i18next'
import { describe, test } from 'vitest'

import type { Asset } from '../../types'
import { AssetCard } from '../asset-card'
import { useAssetColumns } from '../asset-columns'
import { AssetLibraryProvider } from '../asset-library-provider'
import { useAssetGroupColumns } from '../group-columns'
import { ReplicationBadge } from '../replication-badge'

const i18n = createInstance()
await i18n.use(initReactI18next).init({
  lng: 'en',
  resources: {
    en: {
      translation: {
        'Not synchronized': 'Not synchronized',
        '{{ready}} of {{total}} channels ready':
          '{{ready}} of {{total}} channels ready',
        'Ready: {{ready}}, processing: {{processing}}, failed: {{failed}}':
          'Ready: {{ready}}, processing: {{processing}}, failed: {{failed}}',
      },
    },
  },
})

function renderAssetCard(asset: Asset): string {
  const queryClient = new QueryClient()
  return renderToStaticMarkup(
    <QueryClientProvider client={queryClient}>
      <I18nextProvider i18n={i18n}>
        <AssetLibraryProvider>
          <AssetCard row={{ original: asset } as Row<Asset>} />
        </AssetLibraryProvider>
      </I18nextProvider>
    </QueryClientProvider>
  )
}

describe('ReplicationBadge', () => {
  test('renders nothing when replication metadata is not available', () => {
    const markup = renderToStaticMarkup(
      <I18nextProvider i18n={i18n}>
        <ReplicationBadge />
      </I18nextProvider>
    )

    assert.equal(markup, '')
  })

  test('renders replication metadata when the API includes it', () => {
    const markup = renderToStaticMarkup(
      <I18nextProvider i18n={i18n}>
        <ReplicationBadge
          replication={{
            Status: 'ready',
            Ready: 2,
            Processing: 0,
            Failed: 0,
            Total: 2,
          }}
        />
      </I18nextProvider>
    )

    assert.match(markup, /2 of 2 channels ready/)
  })

  test('keeps the alignment offset outside the width-constrained badge', () => {
    const markup = renderToStaticMarkup(
      <I18nextProvider i18n={i18n}>
        <ReplicationBadge
          replication={{
            Status: 'not_synced',
            Ready: 0,
            Processing: 0,
            Failed: 0,
            Total: 0,
          }}
        />
      </I18nextProvider>
    )

    const triggerClasses = markup
      .match(/<span class="([^"]*)"[^>]*role="status"/)?.[1]
      .split(' ')
    const badgeClasses = markup
      .match(/data-slot="status-badge" class="([^"]*)"/)?.[1]
      .split(' ')

    assert.ok(triggerClasses?.includes('-ml-1.5'))
    assert.equal(badgeClasses?.includes('-ml-1.5'), false)
  })

  test('omits the channel section from a customer asset card', () => {
    const asset: Asset = {
      Id: 'asset-na-customer',
      Name: 'Customer asset',
      URL: 'https://example.com/preview.png',
      GroupId: 'group-na-customer',
      AssetType: 'Image',
      Status: 'Active',
      ProjectName: 'default',
      CreateTime: '2026-08-20T00:00:00Z',
      UpdateTime: '2026-08-20T00:00:00Z',
    }
    const markup = renderAssetCard(asset)

    assert.doesNotMatch(markup, /Channel availability/)
  })

  test('renders customer asset cards with a media-first layout', () => {
    const asset: Asset = {
      Id: 'asset-na-media-first',
      Name: 'Customer image',
      URL: 'https://example.com/customer-image.png',
      GroupId: 'group-na-customer',
      AssetType: 'Image',
      Status: 'Active',
      ProjectName: 'default',
      CreateTime: '2026-08-20T00:00:00Z',
      UpdateTime: '2026-08-20T00:00:00Z',
    }
    const markup = renderAssetCard(asset)

    assert.match(markup, /<img[^>]*class="[^"]*aspect-video/)
    assert.doesNotMatch(markup, /asset-na-media-first/)
  })

  test('keeps admin asset cards compact with replication metadata', () => {
    const asset: Asset = {
      Id: 'asset-na-admin',
      Name: 'Admin image',
      URL: 'https://example.com/admin-image.png',
      GroupId: 'group-na-admin',
      AssetType: 'Image',
      Status: 'Active',
      ProjectName: 'default',
      CreateTime: '2026-08-20T00:00:00Z',
      UpdateTime: '2026-08-20T00:00:00Z',
      Replication: {
        Status: 'ready',
        Ready: 1,
        Processing: 0,
        Failed: 0,
        Total: 1,
      },
    }
    const markup = renderAssetCard(asset)

    assert.match(markup, /<img[^>]*class="[^"]*size-16/)
    assert.match(markup, /asset-na-admin/)
    assert.match(markup, /1 of 1 channels ready/)
  })

  test('omits the replication column from the customer asset table', () => {
    function ColumnIds() {
      const columns = useAssetColumns(new Map())
      return columns.map((column, index) => (
        <span key={column.id ?? index}>{column.id}</span>
      ))
    }
    const markup = renderToStaticMarkup(
      <I18nextProvider i18n={i18n}>
        <ColumnIds />
      </I18nextProvider>
    )

    assert.doesNotMatch(markup, /replication/)
  })

  test('omits the replication column from the customer group table', () => {
    function ColumnIds() {
      const columns = useAssetGroupColumns()
      return columns.map((column, index) => (
        <span key={column.id ?? index}>{column.id}</span>
      ))
    }
    const markup = renderToStaticMarkup(
      <I18nextProvider i18n={i18n}>
        <ColumnIds />
      </I18nextProvider>
    )

    assert.doesNotMatch(markup, /replication/)
  })

  test('keeps replication columns in admin asset and group tables', () => {
    function ColumnIds() {
      const assetColumns = useAssetColumns(new Map(), true)
      const groupColumns = useAssetGroupColumns(true)
      return (
        <>
          {assetColumns.map((column) =>
            column.id ? (
              <span key={`asset-${column.id}`}>{column.id}</span>
            ) : null
          )}
          {groupColumns.map((column) =>
            column.id ? (
              <span key={`group-${column.id}`}>{column.id}</span>
            ) : null
          )}
        </>
      )
    }
    const markup = renderToStaticMarkup(
      <I18nextProvider i18n={i18n}>
        <ColumnIds />
      </I18nextProvider>
    )

    assert.equal(markup.match(/replication/g)?.length, 2)
  })
})
