import assert from 'node:assert/strict'
import test from 'node:test'

import { CHANNEL_TYPES, CHANNEL_TYPE_OPTIONS } from '../../constants'
import { getChannelTypeConfig } from '../channel-type-config'
import {
  getChannelExternalAPIs,
  getChannelTypeIcon,
} from '../channel-utils'

const XUNFEI_MAAS_CHANNEL_TYPE = 101

test('Xunfei MaaS uses the custom channel range and native MaaS defaults', () => {
  assert.equal(CHANNEL_TYPES[XUNFEI_MAAS_CHANNEL_TYPE], 'Xunfei MaaS')
  assert.equal(getChannelTypeIcon(XUNFEI_MAAS_CHANNEL_TYPE), 'Spark')

  const config = getChannelTypeConfig(XUNFEI_MAAS_CHANNEL_TYPE)
  assert.equal(
    config.defaultBaseUrl,
    'https://maas-api.cn-huabei-1.xf-yun.com'
  )
  assert.deepEqual(config.supportedModels, ['xopdeepseekv4flash'])
  assert.deepEqual(getChannelExternalAPIs(XUNFEI_MAAS_CHANNEL_TYPE), [
    'chat',
    'responses',
    'messages',
  ])
})

test('custom channel types remain ordered after upstream built-ins', () => {
  const newAPIIndex = CHANNEL_TYPE_OPTIONS.findIndex(({ value }) => value === 60)
  const tokenHubIndex = CHANNEL_TYPE_OPTIONS.findIndex(
    ({ value }) => value === 100
  )
  const xunfeiIndex = CHANNEL_TYPE_OPTIONS.findIndex(
    ({ value }) => value === XUNFEI_MAAS_CHANNEL_TYPE
  )

  assert.ok(newAPIIndex >= 0)
  assert.ok(tokenHubIndex > newAPIIndex)
  assert.ok(xunfeiIndex > tokenHubIndex)
})
