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
import { fireEvent, render, screen } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'

import { ParameterCapabilityTemplatePanel } from '../parameter-capability-template-panel'

describe('parameter capability templates', () => {
  it('previews a selected template and copies it for an editable upstream alias', () => {
    const onApply = vi.fn()
    render(<ParameterCapabilityTemplatePanel config={{}} onApply={onApply} />)
    fireEvent.click(screen.getByRole('button', { name: 'Seedance 2.0 Fast' }))
    fireEvent.change(screen.getByLabelText('Upstream model name'), {
      target: { value: 'sls-fast-alias' },
    })
    expect(
      screen.getByLabelText('Template configuration preview').textContent
    ).toContain('sls-fast-alias')
    expect(onApply).not.toHaveBeenCalled()
    fireEvent.click(screen.getByRole('button', { name: 'Apply template' }))
    const config = onApply.mock.calls[0][0]
    expect(config.rules[0].selector).toEqual({
      type: 'exact',
      value: 'sls-fast-alias',
    })
    expect(config.rules[0].parameters.resolution.allowed_values).toEqual([
      '480p',
      '720p',
    ])
    expect(config.rules[0].parameters.duration.allowed_values).toContain('-1')
    expect(config.rules[0].parameters.duration.participate_in_selection).toBe(
      false
    )
  })

  it('preserves existing constraints by default and previews explicit replacement', () => {
    render(
      <ParameterCapabilityTemplatePanel
        config={{ defaults: { duration: { max: 8 } } }}
        onApply={vi.fn()}
      />
    )
    const preview = () =>
      JSON.parse(
        screen.getByLabelText('Template configuration preview').textContent ||
          '{}'
      )
    expect(preview().rules[0].parameters.duration.max).toBe(8)
    fireEvent.click(
      screen.getByLabelText(
        'Replace existing constraints for template parameters'
      )
    )
    expect(preview().rules[0].parameters.duration.max).toBe(15)
    fireEvent.change(screen.getByLabelText('Upstream model name'), {
      target: { value: '  ' },
    })
    expect(
      screen.getByRole('button', { name: 'Apply template' })
    ).toBeDisabled()
  })
})
