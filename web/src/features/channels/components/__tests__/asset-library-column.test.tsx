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
import { render, screen } from '@testing-library/react'
import { createInstance } from 'i18next'
import type { ReactNode } from 'react'
import { I18nextProvider, initReactI18next } from 'react-i18next'
import { describe, expect, test } from 'vitest'

import type { Channel } from '../../types'
import {
  AssetLibraryStatusCell,
  createAssetLibraryStatusColumn,
} from '../asset-library-status-column'

const i18n = createInstance()
await i18n.use(initReactI18next).init({
  lng: 'en',
  resources: {
    en: {
      translation: {
        'Asset Library': 'Asset Library',
        Enabled: 'Enabled',
        Disabled: 'Disabled',
      },
    },
  },
})

function TestProviders(props: { children: ReactNode }) {
  return <I18nextProvider i18n={i18n}>{props.children}</I18nextProvider>
}

describe('channel asset library column', () => {
  test('includes an Asset Library column in the channel table', () => {
    const column = createAssetLibraryStatusColumn(i18n.t)

    expect('accessorKey' in column && column.accessorKey).toBe(
      'asset_library_enabled'
    )
    expect(column.header).toBe('Asset Library')
  })

  test('shows whether the channel asset library configuration is enabled', () => {
    const { rerender } = render(
      <AssetLibraryStatusCell
        channel={{ asset_library_enabled: true } as Channel}
      />,
      { wrapper: TestProviders }
    )

    expect(screen.getByText('Enabled')).toBeInTheDocument()

    rerender(
      <AssetLibraryStatusCell
        channel={{ asset_library_enabled: false } as Channel}
      />
    )

    expect(screen.getByText('Disabled')).toBeInTheDocument()
  })
})
