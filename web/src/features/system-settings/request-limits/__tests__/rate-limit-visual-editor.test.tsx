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

import { RateLimitVisualEditor } from '../rate-limit-visual-editor'

const i18n = createInstance()
await i18n.use(initReactI18next).init({
  lng: 'en',
  resources: {
    en: {
      translation: {
        TPM: 'TPM',
        Unlimited: 'Unlimited',
      },
    },
  },
})

describe('group rate limit visual editor', () => {
  test('shows TPM for both legacy and extended group limits', () => {
    const markup = renderToStaticMarkup(
      <I18nextProvider i18n={i18n}>
        <RateLimitVisualEditor
          value='{"legacy":[200,100],"vip":[0,1000,60000]}'
          onChange={() => {}}
        />
      </I18nextProvider>
    )

    assert.match(markup, />TPM</)
    assert.match(markup, />legacy</)
    assert.match(markup, />Unlimited</)
    assert.match(markup, />60,000</)
  })
})
