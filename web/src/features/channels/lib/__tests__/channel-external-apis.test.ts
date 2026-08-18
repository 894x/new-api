import assert from 'node:assert/strict'
import test from 'node:test'

import { CHANNEL_TYPE_OPTIONS } from '../../constants'
import { getChannelExternalAPIs } from '../channel-utils'

type ExternalAPI = 'chat' | 'responses' | 'messages'

const BUILT_IN_EXPECTATIONS: ReadonlyArray<{
  types: readonly number[]
  apis: readonly ExternalAPI[]
}> = [
  {
    types: [1, 3, 7, 8, 17, 19, 20, 22, 24, 27, 31, 43, 45, 47, 59, 60, 101],
    apis: ['chat', 'responses', 'messages'],
  },
  {
    types: [4, 14, 25, 26, 33, 35, 40, 41, 46],
    apis: ['chat', 'messages'],
  },
  {
    types: [39, 48],
    apis: ['chat', 'responses'],
  },
  {
    types: [15, 16, 18, 23, 34, 37, 42, 49, 53, 103],
    apis: ['chat'],
  },
  {
    types: [57],
    apis: ['responses'],
  },
  {
    types: [2, 5, 36, 38, 44, 50, 51, 52, 54, 55, 56, 100, 102, 104],
    apis: [],
  },
]

test('every displayed built-in channel reports its verified client-facing APIs', () => {
  const coveredTypes = new Set<number>([58])

  for (const expectation of BUILT_IN_EXPECTATIONS) {
    for (const type of expectation.types) {
      coveredTypes.add(type)
      assert.deepEqual(
        getChannelExternalAPIs({ type, settings: '{}' }),
        expectation.apis,
        `unexpected supported APIs for channel type ${type}`
      )
    }
  }

  assert.deepEqual(
    [...coveredTypes].sort((left, right) => left - right),
    CHANNEL_TYPE_OPTIONS.map((option) => option.value).sort(
      (left, right) => left - right
    )
  )
})

test('advanced custom channel derives and deduplicates APIs from incoming routes', () => {
  const settings = JSON.stringify({
    advanced_custom: {
      advanced_routes: [
        { incoming_path: '/v1/messages' },
        { incoming_path: '/v1/responses/compact' },
        { incoming_path: '/v1/chat/completions' },
        { incoming_path: '/v1/responses' },
        { incoming_path: '/v1/embeddings' },
      ],
    },
  })

  assert.deepEqual(getChannelExternalAPIs({ type: 58, settings }), [
    'chat',
    'responses',
    'messages',
  ])
})

test('advanced custom channel degrades safely for invalid or unrelated settings', () => {
  assert.deepEqual(
    getChannelExternalAPIs({ type: 58, settings: 'not-json' }),
    []
  )
  assert.deepEqual(
    getChannelExternalAPIs({
      type: 58,
      settings: JSON.stringify({
        advanced_custom: {
          advanced_routes: [{ incoming_path: '/v1/embeddings' }],
        },
      }),
    }),
    []
  )
})
