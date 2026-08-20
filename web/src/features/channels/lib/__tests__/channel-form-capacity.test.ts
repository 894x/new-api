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
// @ts-ignore -- test-runner types are intentionally outside the application tsconfig.
import { describe, expect, it } from 'vitest'

import { channelSchema } from '../../types'
import {
  CHANNEL_FORM_DEFAULT_VALUES,
  channelFormSchema,
  transformChannelToFormDefaults,
  transformFormDataToCreatePayload,
  transformFormDataToUpdatePayload,
} from '../channel-form'

describe('channel RPM and TPM form contract', () => {
  it('uses zero as the unlimited default and rejects unsafe limits', () => {
    expect(CHANNEL_FORM_DEFAULT_VALUES.rpm).toBe(0)
    expect(CHANNEL_FORM_DEFAULT_VALUES.tpm).toBe(0)

    expect(
      channelFormSchema.safeParse({
        ...CHANNEL_FORM_DEFAULT_VALUES,
        name: 'capacity channel',
        models: 'gpt-test',
        rpm: 60,
        tpm: 60_000,
      }).success
    ).toBe(true)
    expect(
      channelFormSchema.safeParse({
        ...CHANNEL_FORM_DEFAULT_VALUES,
        name: 'capacity channel',
        models: 'gpt-test',
        rpm: -1,
      }).success
    ).toBe(false)
    expect(
      channelFormSchema.safeParse({
        ...CHANNEL_FORM_DEFAULT_VALUES,
        name: 'capacity channel',
        models: 'gpt-test',
        tpm: 1.5,
      }).success
    ).toBe(false)
    expect(
      channelFormSchema.safeParse({
        ...CHANNEL_FORM_DEFAULT_VALUES,
        name: 'capacity channel',
        models: 'gpt-test',
        tpm: Number.MAX_SAFE_INTEGER + 1,
      }).success
    ).toBe(false)
  })

  it('round-trips channel defaults through edit, create, and update payloads', () => {
    const channel = channelSchema.parse({
      id: 1,
      type: 1,
      key: '',
      status: 1,
      name: 'capacity channel',
      rpm: 60,
      tpm: 60_000,
      created_time: 0,
      test_time: 0,
      response_time: 0,
      balance_updated_time: 0,
    })

    const form = transformChannelToFormDefaults(channel)
    expect(form.rpm).toBe(60)
    expect(form.tpm).toBe(60_000)
    expect(transformFormDataToCreatePayload(form).channel).toMatchObject({
      rpm: 60,
      tpm: 60_000,
    })
    expect(transformFormDataToUpdatePayload(form, 1)).toMatchObject({
      rpm: 60,
      tpm: 60_000,
    })
  })
})
