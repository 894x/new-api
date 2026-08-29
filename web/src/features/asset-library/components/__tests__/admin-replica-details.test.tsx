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

import { createInstance } from 'i18next'
import { renderToStaticMarkup } from 'react-dom/server'
import { I18nextProvider, initReactI18next } from 'react-i18next'
import { describe, test } from 'vitest'

import { AdminReplicaDetails } from '../admin-replica-details'

const i18n = createInstance()
await i18n.use(initReactI18next).init({
  lng: 'en',
  resources: {
    en: {
      translation: {
        Channel: 'Channel',
        Backend: 'Backend',
        Status: 'Status',
        'Upstream ID': 'Upstream ID',
        'Last updated': 'Last updated',
        Actions: 'Actions',
        Enabled: 'Enabled',
        Disabled: 'Disabled',
        'Not synchronized': 'Not synchronized',
        'Sync channel': 'Sync channel',
        Never: 'Never',
      },
    },
  },
})

const replicas = [
  {
    channel_id: 11,
    channel_name: 'Primary upstream',
    backend: 'seedance_sls',
    enabled: true,
    state: 'ready',
    upstream_asset_id: 'lass_primary',
    upstream_status: 'Active',
    updated_time: 1_787_680_000,
  },
  {
    channel_id: 12,
    channel_name: 'Disabled backup',
    backend: 'openapi',
    enabled: false,
    state: 'not_synced',
    upstream_asset_id: '',
    upstream_status: '',
  },
]

describe('AdminReplicaDetails', () => {
  test('shows every configured upstream and its synchronization state', () => {
    const markup = renderToStaticMarkup(
      <I18nextProvider i18n={i18n}>
        <AdminReplicaDetails
          replicas={replicas}
          resource='asset'
          canSync={false}
          onSync={() => undefined}
        />
      </I18nextProvider>
    )

    assert.match(markup, /Primary upstream/)
    assert.match(markup, /lass_primary/)
    assert.match(markup, /Disabled backup/)
    assert.match(markup, /Not synchronized/)
    assert.match(markup, /Disabled/)
    assert.doesNotMatch(markup, /Sync channel/)
  })

  test('offers manual sync only for enabled channels when permitted', () => {
    const markup = renderToStaticMarkup(
      <I18nextProvider i18n={i18n}>
        <AdminReplicaDetails
          replicas={replicas}
          resource='asset'
          canSync
          onSync={() => undefined}
        />
      </I18nextProvider>
    )

    assert.equal(markup.match(/Sync channel/g)?.length, 1)
  })
})
