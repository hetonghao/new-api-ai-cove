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
import { BILLING_PRICING_VARS } from '@/features/pricing/lib/billing-expr'

import type { LogOtherData } from '../types'
import { getLogCacheWriteTokens } from './display-tokens'

/**
 * Resolve the token counts that actually entered settlement for each priced
 * variable of a dynamic (expression) billing log.
 *
 * The settlement itself only records these counts for expressions that price
 * `img_cr` (`billing_tokens`). For every other expression the same counts have
 * to be reconstructed from the log's own token fields, mirroring
 * `BuildTieredTokenParams` in `service/tiered_settle.go`. The normalization is
 * load-bearing: OpenAI-style upstreams report `prompt_tokens` as a total that
 * already contains cache hits, so pricing `cr` separately without subtracting
 * it would overstate the cost by orders of magnitude.
 *
 * Returns null when a priced variable cannot be resolved from the log, so the
 * caller can say the formula is unavailable instead of showing a wrong amount.
 */
export function resolveBilledTokenCounts(props: {
  promptTokens: number
  completionTokens: number
  other: LogOtherData
  pricedKeys: string[]
}): Record<string, number> | null {
  const { other } = props
  const recorded = other.billing_tokens
  if (recorded && props.pricedKeys.every((key) => isCount(recorded[key]))) {
    return Object.fromEntries(
      props.pricedKeys.map((key) => [key, recorded[key] as number])
    )
  }

  const priced = new Set(props.pricedKeys)
  const isClaude = other.claude === true
  const cacheRead = count(other.cache_tokens)
  // Non-Claude cache writes are billed as one total; only Anthropic reports
  // the 5-minute and 1-hour TTLs separately.
  const cacheWriteTotal =
    count(other.cache_creation_tokens) || getLogCacheWriteTokens(other)
  const cacheWrite5m = count(other.cache_creation_tokens_5m)
  const cacheWrite1h = count(other.cache_creation_tokens_1h)
  const imageInput = count(other.image_output)
  const imageCacheRead = count(other.image_cache_tokens)
  const audioInput = count(other.audio_input ?? other.audio_input_token_count)
  const audioOutput = count(other.audio_output)

  if (priced.has('img_o')) return null

  let prompt = props.promptTokens
  let completion = props.completionTokens
  if (isClaude) {
    if (!priced.has('cr')) prompt += cacheRead
  } else {
    if (priced.has('cr')) prompt -= cacheRead
    if (priced.has('cc')) prompt -= cacheWriteTotal
    if (priced.has('img')) prompt -= imageInput
    if (priced.has('img_cr')) prompt -= imageCacheRead
    if (priced.has('ai')) prompt -= audioInput
    if (priced.has('ao')) completion -= audioOutput
  }

  const resolved: Record<string, number> = {}
  for (const key of props.pricedKeys) {
    switch (key) {
      case 'p':
        resolved[key] = Math.max(0, prompt)
        break
      case 'c':
        resolved[key] = Math.max(0, completion)
        break
      case 'cr':
        resolved[key] = cacheRead
        break
      case 'cc':
        resolved[key] = isClaude ? cacheWrite5m : cacheWriteTotal
        break
      case 'cc1h':
        resolved[key] = isClaude ? cacheWrite1h : 0
        break
      case 'img':
        resolved[key] = imageInput
        break
      case 'img_cr':
        resolved[key] = imageCacheRead
        break
      case 'ai':
        resolved[key] = audioInput
        break
      case 'ao':
        resolved[key] = audioOutput
        break
      default:
        return null
    }
  }
  return resolved
}

/** Expression variable keys carrying a price in the matched tier. */
export function billedKeysForFields(fields: string[]): string[] {
  return fields.flatMap((field) => {
    const variable = BILLING_PRICING_VARS.find(
      (candidate) => candidate.field === field
    )
    return variable ? [variable.key] : []
  })
}

function count(value: number | undefined): number {
  return typeof value === 'number' && Number.isFinite(value) ? value : 0
}

function isCount(value: number | undefined): boolean {
  return typeof value === 'number' && Number.isFinite(value)
}
