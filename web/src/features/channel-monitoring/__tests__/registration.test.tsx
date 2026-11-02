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
import { cleanup, render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { useState } from 'react'
import { afterEach, expect, test, vi } from 'vitest'

import {
  CHANNEL_FORM_DEFAULT_VALUES,
  transformFormDataToCreatePayload,
  transformFormDataToUpdatePayload,
} from '@/features/channels/lib/channel-form'

import { ChannelMonitorConfigEditor } from '../channel-monitor-config'

afterEach(cleanup)

test('new channel monitoring is enabled and disabling it preserves registered targets', async () => {
  const changed = vi.fn()
  function Editor() {
    const [value, setValue] = useState(
      '{"targets":[{"model":"model-a","endpoint_type":"anthropic"}]}'
    )
    return (
      <ChannelMonitorConfigEditor
        value={value}
        models='model-a,model-b'
        onChange={(next) => {
          setValue(next)
          changed(next)
        }}
      />
    )
  }
  render(<Editor />)
  const toggle = screen.getByRole('switch', {
    name: 'Enable channel monitoring',
  })
  expect(toggle).toHaveAttribute('aria-checked', 'true')
  await userEvent.click(toggle)
  expect(JSON.parse(changed.mock.lastCall?.[0] ?? '{}')).toEqual({
    enabled: false,
    targets: [{ model: 'model-a', endpoint_type: 'anthropic' }],
  })
  expect(
    screen.queryByRole('button', { name: 'Add monitoring target' })
  ).not.toBeInTheDocument()
  await userEvent.click(toggle)
  expect(
    screen.getByRole('button', { name: 'Add monitoring target' })
  ).toBeEnabled()
})

test('adding and removing monitoring targets emits the model and protocol configuration', async () => {
  const changed = vi.fn()
  function Editor() {
    const [value, setValue] = useState('')
    return (
      <ChannelMonitorConfigEditor
        value={value}
        models='model-a,model-b'
        onChange={(next) => {
          setValue(next)
          changed(next)
        }}
      />
    )
  }
  render(<Editor />)
  await userEvent.click(
    screen.getByRole('button', { name: 'Add monitoring target' })
  )
  expect(JSON.parse(changed.mock.lastCall?.[0] ?? '{}').targets).toEqual([
    { model: 'model-a', endpoint_type: '' },
  ])
  await userEvent.click(
    screen.getByRole('button', { name: 'Add monitoring target' })
  )
  expect(JSON.parse(changed.mock.lastCall?.[0] ?? '{}').targets).toEqual([
    { model: 'model-a', endpoint_type: '' },
    { model: 'model-b', endpoint_type: '' },
  ])
  await userEvent.click(
    screen.getByRole('button', { name: 'Remove monitoring target 1' })
  )
  expect(JSON.parse(changed.mock.lastCall?.[0] ?? '{}').targets).toEqual([
    { model: 'model-b', endpoint_type: '' },
  ])
})

test('channel create and update payloads preserve disabled monitoring registrations', () => {
  const configuration =
    '{"enabled":false,"targets":[{"model":"model-a","endpoint_type":"anthropic"}]}'
  const form = {
    ...CHANNEL_FORM_DEFAULT_VALUES,
    name: 'Example',
    models: 'model-a',
    monitor_config: configuration,
  }
  expect(transformFormDataToCreatePayload(form).channel.monitor_config).toBe(
    configuration
  )
  expect(transformFormDataToUpdatePayload(form, 24).monitor_config).toBe(
    configuration
  )
})

test('a channel without configured models cannot add an empty monitoring target', () => {
  render(<ChannelMonitorConfigEditor value='' models='' onChange={vi.fn()} />)
  expect(
    screen.getByRole('button', { name: 'Add monitoring target' })
  ).toBeDisabled()
})
