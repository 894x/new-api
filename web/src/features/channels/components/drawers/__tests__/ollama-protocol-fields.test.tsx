import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { useForm } from 'react-hook-form'
import { describe, expect, it } from 'vitest'

import { Form } from '@/components/ui/form'

import {
  CHANNEL_FORM_DEFAULT_VALUES,
  buildSettingJSON,
  transformChannelToFormDefaults,
  transformFormDataToCreatePayload,
  type ChannelFormValues,
} from '../../../lib/channel-form'
import type { Channel } from '../../../types'
import { OllamaProtocolFields } from '../ollama-protocol-fields'

function Fixture(props: { type?: number; disabled?: boolean }) {
  const form = useForm<ChannelFormValues>({
    defaultValues: { ...CHANNEL_FORM_DEFAULT_VALUES, type: props.type ?? 4 },
  })
  return (
    <Form {...form}>
      <fieldset disabled={props.disabled}>
        <OllamaProtocolFields control={form.control} />
      </fieldset>
    </Form>
  )
}

describe('Ollama native Claude mode', () => {
  it('defaults to conversion and allows keyboard selection of native mode', async () => {
    const user = userEvent.setup()
    render(<Fixture />)
    const toggle = screen.getByRole('switch', { name: 'Use native Claude API' })
    expect(toggle).not.toBeChecked()
    await user.tab()
    expect(toggle).toHaveFocus()
    await user.keyboard(' ')
    expect(toggle).toBeChecked()
    await user.click(toggle)
    expect(toggle).not.toBeChecked()
  })

  it('does not offer Ollama settings on other channels', () => {
    render(<Fixture type={1} />)
    expect(screen.queryByRole('switch')).not.toBeInTheDocument()
  })

  it('respects the sensitive-settings permission lock', async () => {
    const user = userEvent.setup()
    render(<Fixture disabled />)
    const toggle = screen.getByRole('switch')
    await user.click(toggle)
    expect(toggle).not.toBeChecked()
  })

  it('round-trips the opt-in setting and omits it when disabled or changing channel type', () => {
    const values = {
      ...CHANNEL_FORM_DEFAULT_VALUES,
      type: 4,
      name: 'Local Ollama',
      key: 'test-key',
      models: 'llama3',
    }
    const payload = transformFormDataToCreatePayload(values)
    const channel: Channel = {
      type: 4,
      key: '',
      status: 1,
      name: 'Local Ollama',
      created_time: 0,
      test_time: 0,
      response_time: 0,
      other: '',
      balance: 0,
      balance_updated_time: 0,
      models: 'llama3',
      group: 'default',
      used_quota: 0,
      auto_ban: 1,
      other_info: '',
      remark: '',
      max_input_tokens: 0,
      settings: '{}',
      ...payload.channel,
      id: 1,
      channel_info: {
        is_multi_key: false,
        multi_key_size: 0,
        multi_key_polling_index: 0,
        multi_key_mode: 'random',
      },
    }
    expect(transformChannelToFormDefaults(channel).ollama_native_claude).toBe(
      false
    )
    expect(JSON.parse(buildSettingJSON(values))).not.toHaveProperty(
      'ollama_native_claude'
    )
    const setting = buildSettingJSON({ ...values, ollama_native_claude: true })
    expect(JSON.parse(setting).ollama_native_claude).toBe(true)
    const reopened = transformChannelToFormDefaults({ ...channel, setting })
    expect(reopened.ollama_native_claude).toBe(true)
    expect(
      JSON.parse(buildSettingJSON({ ...reopened, ollama_native_claude: false }))
    ).not.toHaveProperty('ollama_native_claude')
    expect(
      JSON.parse(buildSettingJSON({ ...reopened, type: 1 }))
    ).not.toHaveProperty('ollama_native_claude')
  })
})
