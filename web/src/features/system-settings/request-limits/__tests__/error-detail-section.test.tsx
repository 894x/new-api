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
import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import { ErrorDetailSection } from '../error-detail-section'

const mutateAsync = vi.fn()

vi.mock('../../hooks/use-update-option', () => ({
  useUpdateOption: () => ({ mutateAsync, isPending: false }),
}))

const defaultValues = {
  'error_setting.hide_error_details': true,
  'error_setting.blocked_response_headers': ['X-Request-Id'],
  'error_setting.response_replacement_rules': [],
}

describe('error detail settings', () => {
  beforeEach(() => {
    mutateAsync.mockReset()
    mutateAsync.mockResolvedValue({ success: true })
  })

  it('saves normalized response replacement rules', async () => {
    const { container } = render(
      <ErrorDetailSection defaultValues={defaultValues} />
    )
    const editor = screen.getByRole('textbox', {
      name: 'Error response replacement rules',
    })
    fireEvent.change(editor, {
      target: {
        value:
          '[{"status_code":500,"match":" Moonshot AI ","replacement":" rhzs "}]',
      },
    })

    const form = container.querySelector('form')
    expect(form).not.toBeNull()
    if (!form) return
    fireEvent.submit(form)

    await waitFor(() =>
      expect(mutateAsync).toHaveBeenCalledWith({
        key: 'error_setting.response_replacement_rules',
        value:
          '[{"status_code":500,"match":"Moonshot AI","replacement":"rhzs"}]',
      })
    )
    expect(mutateAsync).toHaveBeenCalledTimes(1)
  })

  it('shows a validation error without saving invalid JSON', async () => {
    const { container } = render(
      <ErrorDetailSection defaultValues={defaultValues} />
    )
    const editor = screen.getByRole('textbox', {
      name: 'Error response replacement rules',
    })
    fireEvent.change(editor, { target: { value: '{broken' } })

    const form = container.querySelector('form')
    expect(form).not.toBeNull()
    if (!form) return
    fireEvent.submit(form)

    expect(
      await screen.findByText(
        'Enter a JSON array with up to 32 rules. Each rule needs status_code (400-599), match, and replacement.'
      )
    ).toBeInTheDocument()
    expect(editor).toHaveAttribute('aria-invalid', 'true')
    expect(mutateAsync).not.toHaveBeenCalled()
  })
})
