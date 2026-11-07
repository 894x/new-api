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
import userEvent from '@testing-library/user-event'
import { useForm } from 'react-hook-form'
import { describe, expect, it, vi } from 'vitest'

import { Button } from '@/components/ui/button'
import { Form } from '@/components/ui/form'

import {
  CHANNEL_FORM_DEFAULT_VALUES,
  channelFormSchema,
  transformChannelToFormDefaults,
  transformFormDataToCreatePayload,
  transformFormDataToUpdatePayload,
  type ChannelFormValues,
} from '../../lib/channel-form'
import { channelSchema } from '../../types'
import { CacheWriteSimulationSetting } from '../cache-write-simulation-setting'

function Fixture(props: {
  disabled?: boolean
  onSave?: (values: ChannelFormValues) => void
}) {
  const form = useForm<ChannelFormValues>({
    defaultValues: CHANNEL_FORM_DEFAULT_VALUES,
  })
  return (
    <Form {...form}>
      <form onSubmit={form.handleSubmit((values) => props.onSave?.(values))}>
        <CacheWriteSimulationSetting disabled={props.disabled} />
        <Button type='submit'>Save</Button>
      </form>
    </Form>
  )
}

describe('cache write simulation channel setting', () => {
  it('reveals the default threshold, percentage and TTL when enabled and preserves edits across toggles', async () => {
    const user = userEvent.setup()
    const onSave = vi.fn()
    render(<Fixture onSave={onSave} />)
    expect(screen.queryByRole('spinbutton')).not.toBeInTheDocument()
    await user.click(
      screen.getByRole('switch', { name: 'Simulate cache writes' })
    )
    const threshold = screen.getByRole('spinbutton', {
      name: 'Cache write input threshold (tokens)',
    })
    expect(threshold).toHaveValue(1024)
    expect(
      screen.getByRole('spinbutton', {
        name: 'Uncached input converted to cache writes (%)',
      })
    ).toHaveValue(100)
    expect(
      screen.getByRole('combobox', { name: 'Cache write TTL' })
    ).toHaveTextContent('5 minutes')
    fireEvent.change(threshold, { target: { value: '2048' } })
    await user.click(screen.getByRole('combobox', { name: 'Cache write TTL' }))
    await user.click(screen.getByRole('option', { name: '1 hour' }))
    await user.click(screen.getByRole('switch'))
    expect(screen.queryByRole('spinbutton')).not.toBeInTheDocument()
    await user.click(screen.getByRole('switch'))
    expect(
      screen.getByRole('spinbutton', {
        name: 'Cache write input threshold (tokens)',
      })
    ).toHaveValue(2048)
    await user.click(screen.getByRole('button', { name: 'Save' }))
    expect(onSave).toHaveBeenCalledWith(
      expect.objectContaining({
        cache_write_simulation_enabled: true,
        cache_write_simulation_threshold: 2048,
        cache_write_simulation_ttl: '1h',
      })
    )
  })

  it('prevents enabling the setting without permission', () => {
    render(<Fixture disabled />)
    expect(screen.getByRole('switch')).toHaveAttribute('aria-disabled', 'true')
  })

  it.each(['5m', '1h'] as const)(
    'round trips %s settings through create and update payloads while preserving unrelated settings',
    (ttl) => {
      const channel = channelSchema.parse({
        id: 7,
        name: 'Test',
        type: 1,
        status: 1,
        key: '',
        created_time: 0,
        test_time: 0,
        response_time: 0,
        balance_updated_time: 0,
        settings: JSON.stringify({
          untouched: true,
          cache_write_simulation: {
            enabled: true,
            min_input_tokens: 0,
            percentage: 33.3,
            ttl,
          },
        }),
      })
      const values = transformChannelToFormDefaults(channel)
      expect(values).toMatchObject({
        cache_write_simulation_enabled: true,
        cache_write_simulation_threshold: 0,
        cache_write_simulation_percentage: 33.3,
        cache_write_simulation_ttl: ttl,
      })
      for (const payload of [
        transformFormDataToCreatePayload(values).channel,
        transformFormDataToUpdatePayload(values, channel.id),
      ]) {
        expect(JSON.parse(payload.settings ?? '{}')).toMatchObject({
          untouched: true,
          cache_write_simulation: {
            enabled: true,
            min_input_tokens: 0,
            percentage: 33.3,
            ttl,
          },
        })
      }
      values.cache_write_simulation_enabled = false
      expect(
        JSON.parse(
          transformFormDataToCreatePayload(values).channel.settings ?? '{}'
        )
      ).not.toHaveProperty('cache_write_simulation')
    }
  )

  it.each([
    { cache_write_simulation_threshold: -1 },
    { cache_write_simulation_threshold: 1.5 },
    { cache_write_simulation_threshold: 2147483648 },
    { cache_write_simulation_threshold: undefined },
    { cache_write_simulation_percentage: -1 },
    { cache_write_simulation_percentage: 101 },
    { cache_write_simulation_percentage: undefined },
  ])('rejects invalid enabled settings: %o', (invalid) => {
    expect(
      channelFormSchema.safeParse({
        ...CHANNEL_FORM_DEFAULT_VALUES,
        name: 'Test',
        models: 'model',
        key: 'test',
        cache_write_simulation_enabled: true,
        ...invalid,
      }).success
    ).toBe(false)
  })
})
