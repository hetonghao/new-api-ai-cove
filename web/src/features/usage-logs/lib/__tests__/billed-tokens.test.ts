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
import { expect, test } from 'vitest'

import { billedKeysForFields, resolveBilledTokenCounts } from '../billed-tokens'

test('billedKeysForFields maps priced tier fields to expression variables', () => {
  expect(
    billedKeysForFields(['inputPrice', 'outputPrice', 'cacheReadPrice'])
  ).toEqual(['p', 'c', 'cr'])
  expect(billedKeysForFields(['fixedPrice'])).toEqual([])
})

test('subsequent cached tokens are removed from the prompt when cr is priced', () => {
  // Real production log: prompt_tokens 373220 includes 372736 cache hits.
  const counts = resolveBilledTokenCounts({
    promptTokens: 373220,
    completionTokens: 1713,
    other: { cache_tokens: 372736 },
    pricedKeys: ['p', 'c', 'cr'],
  })
  expect(counts).toEqual({ p: 484, c: 1713, cr: 372736 })
})

test('claude keeps text-only prompt tokens and folds cache reads when unpriced', () => {
  expect(
    resolveBilledTokenCounts({
      promptTokens: 1000,
      completionTokens: 10,
      other: { claude: true, cache_tokens: 400 },
      pricedKeys: ['p', 'cr'],
    })
  ).toEqual({ p: 1000, cr: 400 })

  expect(
    resolveBilledTokenCounts({
      promptTokens: 1000,
      completionTokens: 10,
      other: { claude: true, cache_tokens: 400 },
      pricedKeys: ['p'],
    })
  ).toEqual({ p: 1400 })
})

test('cache writes are removed from the prompt only when priced', () => {
  expect(
    resolveBilledTokenCounts({
      promptTokens: 1000,
      completionTokens: 0,
      other: { cache_creation_tokens: 250 },
      pricedKeys: ['p', 'cc'],
    })
  ).toEqual({ p: 750, cc: 250 })

  // Claude reports text-only input and separate cache-write TTLs, so p is not
  // reduced by cache writes and cc1h only exists on this path.
  expect(
    resolveBilledTokenCounts({
      promptTokens: 1000,
      completionTokens: 0,
      other: {
        claude: true,
        cache_creation_tokens: 250,
        cache_creation_tokens_5m: 200,
        cache_creation_tokens_1h: 50,
      },
      pricedKeys: ['p', 'cc', 'cc1h'],
    })
  ).toEqual({ p: 1000, cc: 200, cc1h: 50 })

  expect(
    resolveBilledTokenCounts({
      promptTokens: 1000,
      completionTokens: 0,
      other: {},
      pricedKeys: ['p'],
    })
  ).toEqual({ p: 1000 })
})

test('negative derived counts clamp at zero', () => {
  expect(
    resolveBilledTokenCounts({
      promptTokens: 100,
      completionTokens: 0,
      other: { cache_tokens: 900 },
      pricedKeys: ['p', 'cr'],
    })
  ).toEqual({ p: 0, cr: 900 })
})

test('settlement-recorded counts win over derived ones', () => {
  expect(
    resolveBilledTokenCounts({
      promptTokens: 1000,
      completionTokens: 10,
      other: { cache_tokens: 400, billing_tokens: { p: 300, cr: 100, c: 10 } },
      pricedKeys: ['p', 'c', 'cr'],
    })
  ).toEqual({ p: 300, c: 10, cr: 100 })
})

test('an unresolvable priced variable yields no counts', () => {
  expect(
    resolveBilledTokenCounts({
      promptTokens: 1000,
      completionTokens: 10,
      other: {},
      pricedKeys: ['p', 'img_o'],
    })
  ).toBeNull()
})
