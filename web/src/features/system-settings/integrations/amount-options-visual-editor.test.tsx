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
import i18n from 'i18next'
import { I18nextProvider, initReactI18next } from 'react-i18next'
import { beforeAll, describe, expect, test, vi } from 'vitest'

import { AmountOptionsVisualEditor } from './amount-options-visual-editor'

beforeAll(async () => {
  await i18n.use(initReactI18next).init({
    lng: 'en',
    resources: { en: { translation: {} } },
    interpolation: { escapeValue: false },
  })
})

describe('AmountOptionsVisualEditor', () => {
  test('renders configured CNY amounts with the CNY symbol', () => {
    render(
      <I18nextProvider i18n={i18n}>
        <AmountOptionsVisualEditor
          value='[100]'
          onChange={vi.fn()}
          currencySymbol='¥'
        />
      </I18nextProvider>
    )

    expect(screen.getByText('¥100')).toBeInTheDocument()
    expect(screen.queryByText('$100')).not.toBeInTheDocument()
  })
})
