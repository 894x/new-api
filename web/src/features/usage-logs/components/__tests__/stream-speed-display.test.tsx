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

import { createInstance } from 'i18next'
import { renderToStaticMarkup } from 'react-dom/server'
import { I18nextProvider, initReactI18next } from 'react-i18next'

import { StreamTpsCell } from '../timing-metrics-cell'

const i18n = createInstance()
await i18n.use(initReactI18next).init({
  lng: 'en',
  resources: {
    en: {
      translation: {
        Generation: 'Generation',
        Stream: 'Stream',
        'Non-stream': 'Non-stream',
      },
    },
  },
})

describe('stream speed display', () => {
  test('adds generation speed without replacing the existing stream rate', () => {
    const markup = renderToStaticMarkup(
      <I18nextProvider i18n={i18n}>
        <StreamTpsCell
          isStream
          tokensPerSecond={8}
          completionTokens={101}
          useTimeSec={12}
          frtMs={2000}
        />
      </I18nextProvider>
    )

    assert.equal(markup.includes('8 t/s'), true)
    assert.equal(markup.includes('Generation 10 t/s'), true)
  })
})
