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
import { fireEvent, render, screen, within } from '@testing-library/react'
import i18n from 'i18next'
import { describe, expect, test, vi } from 'vitest'

import { MatrixTable } from '../components/matrix-table'
import { createMatrixFixture } from './fixtures'

describe('model-channel matrix table', () => {
  test.each(['zhCN', 'zhTW'])(
    'renders numeric values in the %s interface language',
    async (language) => {
      await i18n.changeLanguage(language)
      try {
        const fixture = createMatrixFixture()
        fixture.cells[0].effective_rpm = 1200
        render(<MatrixTable data={fixture} busy={false} onSelect={vi.fn()} />)
        expect(screen.getByText('1,200')).toBeVisible()
      } finally {
        await i18n.changeLanguage('en')
      }
    }
  )

  test('keeps models on rows and channels on columns with inherited, unlimited and unconfigured cells', () => {
    const select = vi.fn()
    render(
      <MatrixTable
        data={createMatrixFixture()}
        busy={false}
        onSelect={select}
      />
    )
    expect(
      screen.getAllByRole('rowheader').map((header) => header.textContent)
    ).toEqual(['model-a', 'model-b'])
    expect(
      screen.getAllByRole('columnheader').map((header) => header.textContent)
    ).toEqual(['Model', 'Alpha#7Enableddefault', 'Beta#8Enabledpremium'])
    const cell = screen.getByRole('button', {
      name: 'Configure model-a on Alpha (#7)',
    })
    expect(within(cell).getByText('500')).toBeVisible()
    expect(within(cell).getByText('Unlimited')).toBeVisible()
    expect(within(cell).getAllByText('Inherit')).toHaveLength(3)
    expect(within(cell).getByText('Override')).toBeVisible()
    expect(cell).toHaveAccessibleDescription(
      expect.stringMatching(/RPM.*500.*Inherit.*TPM.*Unlimited.*Override/)
    )
    expect(screen.getAllByText('Not configured')).toHaveLength(2)
    fireEvent.click(cell)
    expect(select).toHaveBeenCalledWith(
      expect.objectContaining({ channel_id: 7, model: 'model-a' }),
      cell
    )
  })

  test('keeps both headers fixed in one scroll region and wraps long names', () => {
    const fixture = createMatrixFixture()
    fixture.models = ['a-very-long-public-model-name-with-no-spaces']
    fixture.channels[0].name = 'a-very-long-channel-name-with-no-spaces'
    render(<MatrixTable data={fixture} busy={false} onSelect={vi.fn()} />)
    const region = screen.getByRole('region', { name: 'Model-channel matrix' })
    expect(region).toHaveClass('overflow-auto')
    expect(region).toHaveAttribute('tabindex', '0')
    expect(screen.getByRole('rowheader')).toHaveClass(
      'sticky',
      'left-0',
      'break-all'
    )
    expect(screen.getByRole('columnheader', { name: 'Model' })).toHaveClass(
      'sticky',
      'top-0',
      'left-0'
    )
    expect(screen.getByText(fixture.channels[0].name)).toHaveClass('break-all')
  })

  test('marks disabled channels and prevents editing a stale page during loading', () => {
    const fixture = createMatrixFixture()
    fixture.channels[0].status = 2
    const select = vi.fn()
    render(<MatrixTable data={fixture} busy onSelect={select} />)
    expect(screen.getByText('Disabled')).toBeVisible()
    const cell = screen.getByRole('button', {
      name: 'Configure model-a on Alpha (#7)',
    })
    expect(cell).toBeDisabled()
    fireEvent.click(cell)
    expect(select).not.toHaveBeenCalled()
    expect(screen.getByRole('region')).toHaveAttribute('aria-busy', 'true')
  })
})
