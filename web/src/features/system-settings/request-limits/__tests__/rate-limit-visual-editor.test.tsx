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

import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
  within,
} from '@testing-library/react'
import { createInstance } from 'i18next'
import { renderToStaticMarkup } from 'react-dom/server'
import { I18nextProvider, initReactI18next } from 'react-i18next'
import { afterEach, describe, expect, test, vi } from 'vitest'

import { RateLimitVisualEditor } from '../rate-limit-visual-editor'

afterEach(cleanup)

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
  test('keeps invalid JSON visible instead of allowing an edit to discard rules', () => {
    const onChange = vi.fn()
    render(
      <I18nextProvider i18n={i18n}>
        <RateLimitVisualEditor
          value='{"vip":{"limits":[200,100,60000],"models":{"gpt-5":{"rpm":-1}}}}'
          onChange={onChange}
        />
      </I18nextProvider>
    )
    expect(screen.getByRole('alert')).toHaveTextContent(
      'Invalid JSON format or values out of allowed range'
    )
    expect(
      screen.queryByRole('button', { name: 'Add group' })
    ).not.toBeInTheDocument()
    expect(onChange).not.toHaveBeenCalled()
  })

  test('edits model rules without dropping inherited fields or other groups', async () => {
    const onChange = vi.fn()
    render(
      <I18nextProvider i18n={i18n}>
        <RateLimitVisualEditor
          value='{"vip":{"limits":[200,100,60000],"models":{" gpt-5 ":{"rpm":30},"claude-sonnet":{"tpm":0}}},"legacy":[200,100]}'
          onChange={onChange}
        />
      </I18nextProvider>
    )
    const row = screen.getByRole('row', { name: /vip/ })
    fireEvent.click(within(row).getByRole('button', { name: 'Edit' }))
    const dialog = await screen.findByRole('dialog')
    expect(
      within(dialog).getByRole('textbox', { name: 'Model 1' })
    ).toHaveValue(' gpt-5 ')
    const rpm = within(dialog).getByRole('spinbutton', { name: 'RPM 1' })
    fireEvent.change(rpm, { target: { value: '' } })
    fireEvent.change(
      within(dialog).getByRole('spinbutton', { name: 'TPM 1' }),
      { target: { value: '50000' } }
    )
    expect(rpm).toHaveValue(null)
    fireEvent.click(within(dialog).getByRole('button', { name: 'Update' }))
    await waitFor(() => expect(onChange).toHaveBeenCalled())
    expect(JSON.parse(onChange.mock.lastCall?.[0] ?? 'null')).toEqual({
      vip: {
        limits: [200, 100, 60000],
        models: { ' gpt-5 ': { tpm: 50000 }, 'claude-sonnet': { tpm: 0 } },
      },
      legacy: [200, 100],
    })
  })

  test('rejects duplicate model names with a field error before saving', async () => {
    const onChange = vi.fn()
    render(
      <I18nextProvider i18n={i18n}>
        <RateLimitVisualEditor
          value='{"vip":[200,100,60000]}'
          onChange={onChange}
        />
      </I18nextProvider>
    )
    fireEvent.click(screen.getByRole('button', { name: 'Edit' }))
    const dialog = await screen.findByRole('dialog')
    fireEvent.click(
      within(dialog).getByRole('button', { name: 'Add model rule' })
    )
    fireEvent.click(
      within(dialog).getByRole('button', { name: 'Add model rule' })
    )
    fireEvent.change(within(dialog).getByRole('textbox', { name: 'Model 1' }), {
      target: { value: 'gpt-5' },
    })
    fireEvent.change(within(dialog).getByRole('textbox', { name: 'Model 2' }), {
      target: { value: 'gpt-5' },
    })
    fireEvent.click(within(dialog).getByRole('button', { name: 'Update' }))
    expect(
      await within(dialog).findByText('Model names must be unique')
    ).toBeInTheDocument()
    expect(
      within(dialog).getByRole('textbox', { name: 'Model 2' })
    ).toHaveAttribute('aria-invalid', 'true')
    expect(onChange).not.toHaveBeenCalled()
  })

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
