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
import assert from 'node:assert/strict'
import { describe, test } from 'node:test'

import {
  CHANNEL_TYPE_ASTRAFLOW_GEMINI,
  CHANNEL_TYPE_OPTIONS,
  MODEL_FETCHABLE_TYPES,
} from '../../constants'
import { getChannelTypeConfig } from '../channel-type-config'
import {
  getChannelExternalAPIs,
  getChannelTypeIcon,
  getKeyPromptForType,
} from '../channel-utils'

describe('AstraFlow Gemini channel', () => {
  test('registers a dedicated Gemini image channel with OpenAI chat compatibility', () => {
    const option = CHANNEL_TYPE_OPTIONS.find(
      (item) => item.value === CHANNEL_TYPE_ASTRAFLOW_GEMINI
    )

    assert.deepEqual(option, {
      value: CHANNEL_TYPE_ASTRAFLOW_GEMINI,
      label: 'AstraFlow Gemini',
    })
    assert.equal(CHANNEL_TYPE_ASTRAFLOW_GEMINI, 103)
    assert.equal(MODEL_FETCHABLE_TYPES.has(CHANNEL_TYPE_ASTRAFLOW_GEMINI), false)
    assert.equal(getChannelTypeIcon(CHANNEL_TYPE_ASTRAFLOW_GEMINI), 'Gemini')
    assert.equal(
      getKeyPromptForType(CHANNEL_TYPE_ASTRAFLOW_GEMINI),
      'Enter API key for this channel'
    )

    const config = getChannelTypeConfig(CHANNEL_TYPE_ASTRAFLOW_GEMINI)
    assert.equal(config.icon, 'google')
    assert.equal(config.defaultBaseUrl, 'https://api.modelverse.cn')
    assert.deepEqual(config.supportedModels, ['gemini-2.5-flash-image'])
    assert.deepEqual(
      getChannelExternalAPIs({
        type: CHANNEL_TYPE_ASTRAFLOW_GEMINI,
        settings: '',
      }),
      ['chat']
    )
  })
})
