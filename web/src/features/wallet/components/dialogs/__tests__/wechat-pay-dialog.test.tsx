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
import { render, screen, waitFor } from '@testing-library/react'
import i18n from 'i18next'
import { I18nextProvider, initReactI18next } from 'react-i18next'
import { beforeAll, describe, expect, test, vi } from 'vitest'

import { WechatPayDialog } from '../wechat-pay-dialog'

beforeAll(async () => {
  await i18n.use(initReactI18next).init({
    lng: 'en',
    resources: { en: { translation: {} } },
    interpolation: { escapeValue: false },
  })
})

describe('WechatPayDialog', () => {
  test('renders the native QR code and closes after active status compensation succeeds', async () => {
    const checkStatus = vi.fn().mockResolvedValue(true)
    const onPaid = vi.fn()

    render(
      <I18nextProvider i18n={i18n}>
        <WechatPayDialog
          open
          onOpenChange={vi.fn()}
          order={{
            code_url: 'weixin://wxpay/example',
            trade_no: 'WX123',
            money_cents: 1001,
            expires_at: Math.floor(Date.now() / 1000) + 900,
          }}
          checkStatus={checkStatus}
          onPaid={onPaid}
        />
      </I18nextProvider>
    )

    expect(screen.getByTitle('WeChat Pay QR code')).toBeInTheDocument()
    expect(screen.getByText('WX123')).toBeInTheDocument()
    await waitFor(() => expect(checkStatus).toHaveBeenCalledWith('WX123'))
    await waitFor(() => expect(onPaid).toHaveBeenCalledTimes(1))
  })
})
