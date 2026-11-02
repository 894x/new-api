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
import { zodResolver } from '@hookform/resolvers/zod'
import { fireEvent, render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { useForm } from 'react-hook-form'
import { describe, expect, it, vi } from 'vitest'

import { Form } from '@/components/ui/form'

import {
  buildSettingJSON,
  CHANNEL_FORM_DEFAULT_VALUES,
  channelFormSchema,
  transformChannelToFormDefaults,
  type ChannelFormValues,
} from '../../lib/channel-form'
import { channelSchema } from '../../types'
import { StreamResponseHeaderSetting } from '../stream-response-header-setting'

const defaults: ChannelFormValues = {
  ...CHANNEL_FORM_DEFAULT_VALUES,
  name: 'Streaming channel',
  key: 'test-key',
  models: 'test-model',
}

function SettingsForm(props: {
  values?: ChannelFormValues
  disabled?: boolean
  onSave: (value: string) => void
}) {
  const form = useForm<ChannelFormValues>({
    resolver: zodResolver(channelFormSchema),
    defaultValues: props.values ?? defaults,
  })
  return (
    <Form {...form}>
      <form
        onSubmit={form.handleSubmit((values) =>
          props.onSave(buildSettingJSON(values))
        )}
      >
        <StreamResponseHeaderSetting disabled={props.disabled} />
        <button type='submit'>Save</button>
      </form>
    </Form>
  )
}

describe('channel stream response headers', () => {
  it('saves exact header names and removes blank lines and case-insensitive duplicates', async () => {
    const user = userEvent.setup()
    const saved = vi.fn()
    render(<SettingsForm onSave={saved} />)
    const input = screen.getByRole('textbox', {
      name: 'Stream response headers',
    })
    expect(input).toHaveValue('')
    fireEvent.change(input, {
      target: {
        value:
          ' Msh-Usage-Cache-Write-Tokens-5m \n\nmsh-usage-cache-write-tokens-5m\nMsh-Usage-Cache-Write-Tokens-1h',
      },
    })
    await user.click(screen.getByRole('button', { name: 'Save' }))
    expect(saved).toHaveBeenCalledOnce()
    expect(JSON.parse(saved.mock.calls[0][0]).stream_response_headers).toEqual([
      'Msh-Usage-Cache-Write-Tokens-5m',
      'Msh-Usage-Cache-Write-Tokens-1h',
    ])
  })

  it('restores saved headers for editing and removes the setting when cleared', async () => {
    const user = userEvent.setup()
    const saved = vi.fn()
    const channel = channelSchema.parse({
      ...defaults,
      group: 'default',
      id: 1,
      created_time: 0,
      test_time: 0,
      response_time: 0,
      balance_updated_time: 0,
      setting: JSON.stringify({
        stream_response_headers: ['X-First', 'X-Second'],
      }),
    })
    const values = transformChannelToFormDefaults(channel)
    render(<SettingsForm values={values} onSave={saved} />)
    const input = screen.getByRole('textbox', {
      name: 'Stream response headers',
    })
    expect(input).toHaveValue('X-First\nX-Second')
    await user.clear(input)
    await user.click(screen.getByRole('button', { name: 'Save' }))
    expect(saved).toHaveBeenCalledOnce()
    expect(JSON.parse(saved.mock.calls[0][0])).not.toHaveProperty(
      'stream_response_headers'
    )
    expect(JSON.parse(buildSettingJSON(defaults))).not.toHaveProperty(
      'stream_response_headers'
    )
  })

  it('disables editing when the channel settings are locked', () => {
    render(<SettingsForm disabled onSave={vi.fn()} />)
    expect(
      screen.getByRole('textbox', { name: 'Stream response headers' })
    ).toBeDisabled()
  })

  it('shows a field error and prevents saving a gateway-managed header', async () => {
    const user = userEvent.setup()
    const saved = vi.fn()
    render(<SettingsForm onSave={saved} />)
    const input = screen.getByRole('textbox', {
      name: 'Stream response headers',
    })
    await user.type(input, 'Content-Encoding')
    await user.click(screen.getByRole('button', { name: 'Save' }))
    expect(input).toHaveAttribute('aria-invalid', 'true')
    expect(saved).not.toHaveBeenCalled()
  })

  it.each([
    'X Invalid',
    'X-Trace\rInjected',
    'Set-Cookie',
    'connection',
    'X'.repeat(129),
    Array.from({ length: 65 }, (_, index) => `X-Header-${index}`).join('\n'),
  ])(
    'rejects invalid, excessive or gateway-managed header names: %s',
    (value) => {
      const result = channelFormSchema.safeParse({
        ...defaults,
        stream_response_headers: value,
      })
      expect(result.success).toBe(false)
      if (!result.success) {
        expect(
          result.error.issues.some(
            (issue) => issue.path[0] === 'stream_response_headers'
          )
        ).toBe(true)
      }
    }
  )
})
