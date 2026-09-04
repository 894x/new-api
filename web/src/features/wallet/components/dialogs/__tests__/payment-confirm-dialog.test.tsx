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
import { afterEach, beforeAll, describe, expect, test, vi } from 'vitest'

import {
  DEFAULT_CURRENCY_CONFIG,
  useSystemConfigStore,
} from '@/stores/system-config-store'

import { PaymentConfirmDialog } from '../payment-confirm-dialog'

beforeAll(async () => {
  await i18n.use(initReactI18next).init({
    lng: 'en',
    resources: { en: { translation: {} } },
    interpolation: { escapeValue: false },
  })
})

afterEach(() => {
  useSystemConfigStore.getState().setConfig({
    currency: { ...DEFAULT_CURRENCY_CONFIG },
  })
})

describe('PaymentConfirmDialog', () => {
  test('shows a direct CNY top-up amount without applying the exchange rate twice', () => {
    useSystemConfigStore.getState().setConfig({
      currency: {
        ...DEFAULT_CURRENCY_CONFIG,
        quotaDisplayType: 'CNY',
        usdExchangeRate: 7.3,
      },
    })

    render(
      <I18nextProvider i18n={i18n}>
        <PaymentConfirmDialog
          open
          onOpenChange={vi.fn()}
          onConfirm={vi.fn()}
          topupAmount={100}
          paymentAmount={100}
          paymentMethod={{ name: 'WeChat Pay', type: 'wechat_native' }}
          calculating={false}
          processing={false}
        />
      </I18nextProvider>
    )

    expect(screen.getByText('¥100')).toBeInTheDocument()
    expect(screen.queryByText('¥730')).not.toBeInTheDocument()
  })
})
