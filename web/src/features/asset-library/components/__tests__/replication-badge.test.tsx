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
import { describe, test } from 'node:test'

import type { Row } from '@tanstack/react-table'
import { createInstance } from 'i18next'
import { renderToStaticMarkup } from 'react-dom/server'
import { I18nextProvider, initReactI18next } from 'react-i18next'

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
    const markup = renderToStaticMarkup(
      <I18nextProvider i18n={i18n}>
        <AssetLibraryProvider>
          <AssetCard row={{ original: asset } as Row<Asset>} />
        </AssetLibraryProvider>
      </I18nextProvider>
    )

    assert.doesNotMatch(markup, /Channel availability/)
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
