import assert from 'node:assert/strict'
import test from 'node:test'

import {
  CHANNEL_TYPES,
  CHANNEL_TYPE_OPTIONS,
  CHANNEL_TYPE_SEEDANCE_SLS,
} from '../../constants'
import { getChannelTypeConfig } from '../channel-type-config'
import { getChannelExternalAPIs, getChannelTypeIcon } from '../channel-utils'

test('Seedance SLS is exposed as a dedicated video channel', () => {
  assert.equal(CHANNEL_TYPE_SEEDANCE_SLS, 104)
  assert.equal(CHANNEL_TYPES[CHANNEL_TYPE_SEEDANCE_SLS], 'Seedance SLS')
  assert.ok(
    CHANNEL_TYPE_OPTIONS.some(
      (option) =>
        option.value === CHANNEL_TYPE_SEEDANCE_SLS &&
        option.label === 'Seedance SLS'
    )
  )

  const config = getChannelTypeConfig(CHANNEL_TYPE_SEEDANCE_SLS)
  assert.equal(config.defaultBaseUrl, 'https://lm.sls.cn')
  assert.deepEqual(config.supportedModels, [
    'doubao-seedance-2-0',
    'doubao-seedance-2-0-fast',
    'doubao-seedance-2-0-mini',
  ])
  assert.equal(getChannelTypeIcon(CHANNEL_TYPE_SEEDANCE_SLS), 'Doubao')
  assert.deepEqual(
    getChannelExternalAPIs({
      type: CHANNEL_TYPE_SEEDANCE_SLS,
      settings: '{}',
    }),
    []
  )
})
