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
import { describe, expect, it } from 'vitest'

import {
  CHANNEL_FORM_DEFAULT_VALUES,
  channelFormSchema,
  transformFormDataToCreatePayload,
} from '../channel-form'

const values = {
  ...CHANNEL_FORM_DEFAULT_VALUES,
  name: 'Channel groups',
  key: 'test-key',
  models: 'test-model',
}

describe('channel group count validation', () => {
  it('accepts sixteen long group names and preserves the full submitted value', () => {
    const group = Array.from(
      { length: 16 },
      (_, index) => `${String(index).padStart(2, '0')}-${'长'.repeat(61)}`
    )
    const result = channelFormSchema.safeParse({ ...values, group })

    expect(result.success).toBe(true)
    const payload = transformFormDataToCreatePayload({ ...values, group })
    expect(payload.channel.group).toBe(group.join(','))
  })

  it('rejects seventeen groups even when their combined names fit in sixty four characters', () => {
    const group = Array.from({ length: 17 }, (_, index) => String(index))
    const result = channelFormSchema.safeParse({ ...values, group })

    expect(result.success).toBe(false)
    if (!result.success) {
      expect(result.error.issues).toContainEqual(
        expect.objectContaining({
          path: ['group'],
          message: 'A channel can belong to at most 16 groups.',
        })
      )
    }
  })

  it('continues to require at least one group', () => {
    expect(channelFormSchema.safeParse({ ...values, group: [] }).success).toBe(
      false
    )
  })
})
