import assert from 'node:assert/strict'
import test from 'node:test'

import {
  getDisplayPromptTokens,
  getLogCacheWriteTokens,
} from './display-tokens.ts'

test('getLogCacheWriteTokens prefers split cache-write totals when present', () => {
  assert.equal(
    getLogCacheWriteTokens({
      cache_creation_tokens: 999,
      cache_creation_tokens_5m: 200,
      cache_creation_tokens_1h: 50,
    }),
    250
  )
})

test('getDisplayPromptTokens subtracts cache read and write for non-claude logs', () => {
  assert.equal(
    getDisplayPromptTokens(392604, {
      cache_tokens: 391552,
    }),
    1052
  )
})

test('getDisplayPromptTokens keeps claude prompt tokens unchanged', () => {
  assert.equal(
    getDisplayPromptTokens(89, {
      claude: true,
      cache_tokens: 27105,
      cache_creation_tokens: 287,
    }),
    89
  )
})
