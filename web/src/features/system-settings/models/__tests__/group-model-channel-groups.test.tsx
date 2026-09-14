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
import { render, screen, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { useState } from 'react'
import { describe, expect, test } from 'vitest'

import { GroupModelChannelGroupsEditor } from '../group-model-channel-groups-editor'
import { parseGroupModelChannelGroups } from '../lib/group-model-channel-groups'

function PolicyEditor(props: { initial?: string; disabled?: boolean }) {
  const [value, setValue] = useState(props.initial ?? '{}')
  return (
    <>
      <GroupModelChannelGroupsEditor
        value={value}
        onChange={setValue}
        groupOptions={['enterprise', 'official', 'preferred', 'auto']}
        disabled={props.disabled}
      />
      <output aria-label='Policy JSON'>{value}</output>
    </>
  )
}

describe('model channel pool editing', () => {
  test('adds an explicit deny rule, selects pools and removes the restriction', async () => {
    const user = userEvent.setup()
    render(<PolicyEditor />)
    expect(
      screen.getByText('No model channel restrictions')
    ).toBeInTheDocument()
    await user.selectOptions(screen.getByLabelText('User group'), 'enterprise')
    await user.type(screen.getByLabelText('Model'), 'model-a')
    await user.click(screen.getByRole('button', { name: 'Add rule' }))
    expect(
      JSON.parse(screen.getByLabelText('Policy JSON').textContent ?? '')
    ).toEqual({ enterprise: { 'model-a': [] } })
    expect(
      screen.getByText('No channel groups selected: this model is blocked.')
    ).toBeInTheDocument()
    const row = screen.getByRole('group', { name: 'enterprise / model-a' })
    await user.click(within(row).getByLabelText('Channel groups'))
    await user.click(await screen.findByRole('option', { name: /^official$/ }))
    await user.keyboard('{Escape}')
    expect(
      JSON.parse(screen.getByLabelText('Policy JSON').textContent ?? '')
    ).toEqual({ enterprise: { 'model-a': ['official'] } })
    await user.click(within(row).getByRole('button', { name: 'Remove rule' }))
    expect(
      JSON.parse(screen.getByLabelText('Policy JSON').textContent ?? '')
    ).toEqual({})
  })

  test('rejects a duplicate rule without replacing the selected pools', async () => {
    const user = userEvent.setup()
    render(<PolicyEditor initial='{"enterprise":{"model-a":["official"]}}' />)
    await user.selectOptions(screen.getByLabelText('User group'), 'enterprise')
    await user.type(screen.getByLabelText('Model'), 'model-a')
    await user.click(screen.getByRole('button', { name: 'Add rule' }))
    expect(await screen.findByRole('alert')).toHaveTextContent(
      'A channel pool rule already exists for this group and model'
    )
    expect(screen.getByLabelText('Model')).toHaveAttribute(
      'aria-invalid',
      'true'
    )
    expect(
      JSON.parse(screen.getByLabelText('Policy JSON').textContent ?? '')
    ).toEqual({ enterprise: { 'model-a': ['official'] } })
  })

  test('preserves malformed input for JSON repair instead of overwriting it', () => {
    render(<PolicyEditor initial='{"enterprise":{"model-a":null}}' />)
    expect(screen.getByRole('alert')).toHaveTextContent(
      'Invalid channel pool policy. Switch to JSON mode to repair it.'
    )
    expect(
      screen.queryByRole('button', { name: 'Add rule' })
    ).not.toBeInTheDocument()
    expect(screen.getByLabelText('Policy JSON')).toHaveTextContent(
      '{"enterprise":{"model-a":null}}'
    )
  })

  test('disables editing while settings are saving', () => {
    render(<PolicyEditor initial='{"enterprise":{"model-a":[]}}' disabled />)
    expect(screen.getByRole('button', { name: 'Add rule' })).toBeDisabled()
    expect(screen.getByRole('button', { name: 'Remove rule' })).toBeDisabled()
    expect(screen.getByLabelText('Model')).toBeDisabled()
    expect(screen.getByLabelText('User group')).toBeDisabled()
  })
})

describe('model channel pool contract', () => {
  test('distinguishes a missing model from an explicit empty pool selection', () => {
    expect(
      parseGroupModelChannelGroups('{"enterprise":{"model-a":[]}}')
    ).toEqual({ enterprise: { 'model-a': [] } })
    expect(parseGroupModelChannelGroups('{}')).toEqual({})
  })

  test.each([
    'null',
    '[]',
    '{"enterprise":null}',
    '{"enterprise":{"m":null}}',
    '{"enterprise":{"m":["auto"]}}',
    '{"enterprise":{"m":["official","official"]}}',
    '{"enterprise":{" m":[]}}',
  ])('rejects invalid policy %s', (value) => {
    expect(parseGroupModelChannelGroups(value)).toBeNull()
  })
})
