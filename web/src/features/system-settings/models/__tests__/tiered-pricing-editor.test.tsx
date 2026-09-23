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
import { act, fireEvent, render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { useState } from 'react'
import { afterEach, describe, expect, test, vi } from 'vitest'

import { TieredPricingEditor } from '../tiered-pricing-editor'

afterEach(() => vi.unstubAllGlobals())

function EditorFixture(props: { rules: string; billing?: string }) {
  const [billing, setBilling] = useState(
    props.billing ?? 'tier("base", p * 1 + c * 2)'
  )
  const [rules, setRules] = useState(props.rules)
  return (
    <>
      <TieredPricingEditor
        billingExpr={billing}
        requestRuleExpr={rules}
        onBillingExprChange={setBilling}
        onRequestRuleExprChange={setRules}
      />
      <output aria-label='Saved request rules'>{rules}</output>
    </>
  )
}

const morning = 'hour("Asia/Shanghai") >= 9 && hour("Asia/Shanghai") < 12'
const afternoon = 'hour("Asia/Shanghai") >= 14 && hour("Asia/Shanghai") < 18'

describe('request-rule time ranges', () => {
  test('shows the range explanation and switches to overnight logic when start exceeds end', () => {
    render(<EditorFixture rules={`(${morning} ? 2 : 1)`} />)
    expect(screen.getByText('Time range')).toBeInTheDocument()
    expect(
      screen.getByText(
        'Start ≤ end: within the day; start > end: across midnight'
      )
    ).toBeInTheDocument()
    fireEvent.change(screen.getByRole('spinbutton', { name: 'Start' }), {
      target: { value: '21' },
    })
    expect(screen.getByLabelText('Saved request rules')).toHaveTextContent(
      '(hour("Asia/Shanghai") >= 21 || hour("Asia/Shanghai") < 12 ? 2 : 1)'
    )
  })

  test.each([
    '(hour("Asia/Shanghai") >= 9 ? 2 : 1)',
    '(month("Asia/Shanghai") >= 3 && month("Asia/Shanghai") < 6 ? 2 : 1)',
  ])(
    'does not show a midnight explanation for non-hour-range rule %s',
    (rules) => {
      render(<EditorFixture rules={rules} />)
      expect(
        screen.queryByText(
          'Start ≤ end: within the day; start > end: across midnight'
        )
      ).not.toBeInTheDocument()
    }
  )

  test.each([
    [
      'rule group',
      `(${morning} ? 2 : 1) * (${afternoon} ? 3 : 1)`,
      'Remove rule group',
    ],
    ['condition', `(${morning} && ${afternoon} ? 2 : 1)`, 'Remove condition'],
  ])(
    'removing an earlier %s preserves the remaining range input draft',
    (_name, rules, removeLabel) => {
      render(<EditorFixture rules={rules} />)
      const remainingStart = screen.getAllByRole('spinbutton', {
        name: 'Start',
      })[1]
      act(() => remainingStart.focus())
      fireEvent.change(remainingStart, { target: { value: '014' } })
      expect(remainingStart).toHaveDisplayValue('014')
      fireEvent.click(screen.getAllByRole('button', { name: removeLabel })[0])
      expect(screen.getByRole('spinbutton', { name: 'Start' })).toHaveFocus()
      expect(
        screen.getByRole('spinbutton', { name: 'Start' })
      ).toHaveDisplayValue('014')
      expect(screen.getByLabelText('Saved request rules')).toHaveTextContent(
        afternoon
      )
      expect(
        screen.getByLabelText('Saved request rules')
      ).not.toHaveTextContent(morning)
    }
  )
})

describe('visual tier row identity', () => {
  test('works on HTTP deployments without the secure-context randomUUID API', () => {
    vi.stubGlobal('crypto', {
      getRandomValues: crypto.getRandomValues.bind(crypto),
    })
    render(<EditorFixture rules={`(${morning} ? 2 : 1)`} />)
    expect(
      screen.getByRole('spinbutton', { name: 'Start' })
    ).toHaveDisplayValue('9')
    fireEvent.change(screen.getByRole('spinbutton', { name: 'Start' }), {
      target: { value: '10' },
    })
    expect(screen.getByLabelText('Saved request rules')).toHaveTextContent(
      '>= 10'
    )
  })

  test('deleting an earlier tier condition preserves the remaining draft', async () => {
    render(
      <EditorFixture
        rules=''
        billing='len < 100 && c < 20 ? tier("small", p * 1 + c * 2) : tier("base", p * 3 + c * 4)'
      />
    )
    const user = userEvent.setup()
    const remaining = screen.getAllByRole('textbox', {
      name: 'Condition value',
    })[1]
    act(() => remaining.focus())
    fireEvent.change(remaining, { target: { value: '020' } })
    await user.click(
      screen.getByRole('button', { name: 'Condition actions 1.1' })
    )
    await user.click(screen.getByRole('menuitem', { name: 'Remove condition' }))
    expect(
      screen.getByRole('textbox', { name: 'Condition value' })
    ).toHaveValue('020')
  })

  test('deleting an earlier tier keeps the remaining pricing editor open', async () => {
    render(
      <EditorFixture
        rules=''
        billing='len < 100 ? tier("small", p * 1 + c * 2) : tier("base", p * 3 + c * 4)'
      />
    )
    const user = userEvent.setup()
    await user.click(
      screen.getByRole('button', { name: 'Edit pricing rule base' })
    )
    await user.click(screen.getByRole('button', { name: 'Branch actions 1' }))
    await user.click(screen.getByRole('menuitem', { name: 'Remove branch' }))
    expect(
      screen.getByRole('button', { name: 'Edit pricing rule base' })
    ).toHaveAttribute('aria-expanded', 'true')
  })

  test('switching the model replaces the prior model rules', () => {
    const onBillingExprChange = vi.fn()
    const onRequestRuleExprChange = vi.fn()
    const { rerender } = render(
      <TieredPricingEditor
        modelName='first'
        billingExpr='tier("base", p * 1 + c * 2)'
        requestRuleExpr={`(${morning} ? 2 : 1)`}
        onBillingExprChange={onBillingExprChange}
        onRequestRuleExprChange={onRequestRuleExprChange}
      />
    )
    rerender(
      <TieredPricingEditor
        modelName='second'
        billingExpr='tier("next", p * 3 + c * 4)'
        requestRuleExpr={`(${afternoon} ? 3 : 1)`}
        onBillingExprChange={onBillingExprChange}
        onRequestRuleExprChange={onRequestRuleExprChange}
      />
    )
    expect(
      screen.getByRole('spinbutton', { name: 'Start' })
    ).toHaveDisplayValue('14')
    expect(
      screen.getByRole('textbox', { name: 'Tier name' })
    ).toHaveDisplayValue('next')
    expect(onBillingExprChange).not.toHaveBeenCalled()
    expect(onRequestRuleExprChange).not.toHaveBeenCalled()
  })

  test('opening a raw-only expression does not emit default visual pricing', () => {
    const onBillingExprChange = vi.fn()
    render(
      <TieredPricingEditor
        billingExpr='custom_cost(p, c)'
        requestRuleExpr=''
        onBillingExprChange={onBillingExprChange}
        onRequestRuleExprChange={vi.fn()}
      />
    )
    expect(screen.getByDisplayValue('custom_cost(p, c)')).toBeInTheDocument()
    expect(onBillingExprChange).not.toHaveBeenCalled()
  })
})
