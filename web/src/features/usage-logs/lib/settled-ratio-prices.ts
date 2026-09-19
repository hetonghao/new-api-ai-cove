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
import type { LogOtherData } from '../types'
import { getLogCacheWriteTokens } from './display-tokens'
import { isPerCallBilling } from './utils'

/** Ratio 1 is priced at $2 per million tokens, so unit prices scale with it. */
const USD_PER_MILLION_PER_RATIO = 2

export interface SettledPriceRow {
  id: string
  label: string
  base: number
  final: number
}

export interface SettledUsageRow {
  id: string
  label: string
  count: number
  price: number
}

export interface RatioSettledPlan {
  rows: SettledPriceRow[]
  /**
   * Usage behind the charge, or undefined when the charge does not multiply a
   * unit price by usage (per-call billing) and no usage block belongs to it.
   */
  usage: SettledUsageRow[] | undefined
  groupLabel: string
  groupRatio: number
  unitLabel: string
}

interface RatioSettledLine {
  id: string
  label: string
  price: number
  count: number
}

/**
 * Rebuild the final unit prices of a ratio-billed request from the ratios its
 * log entry recorded.
 *
 * `service/text_quota.go` prices cache reads, cache writes, image input and
 * separately priced audio input as their own lines and removes them from the
 * input line, so the logged prompt tokens are normalized the same way here.
 * Anthropic input already excludes cache reads, and its cache writes split into
 * a remaining aggregate line plus the 5-minute and 1-hour lines.
 *
 * Realtime audio sessions price text and audio with dedicated audio ratios but
 * only log aggregate counts, so their unit prices stay unavailable instead of
 * being guessed. The caller falls back to the legacy billing breakdown then.
 */
export function resolveRatioSettledPlan(props: {
  other: LogOtherData
  promptTokens: number
  completionTokens: number
  withUsageCost: boolean
}): RatioSettledPlan | null {
  const { other } = props
  if (other.is_task) return null
  if (other.ws === true || other.audio === true) return null

  const hasUserRatio =
    other.user_group_ratio != null && other.user_group_ratio !== -1
  const groupRatio = hasUserRatio ? other.user_group_ratio : other.group_ratio
  if (!isRatio(groupRatio)) return null
  const groupLabel = hasUserRatio ? 'User Exclusive Ratio' : 'Group Ratio'

  if (isPerCallBilling(other.model_price)) {
    const modelPrice = other.model_price as number
    return {
      rows: [
        {
          id: 'fixedPrice',
          label: 'Per-call',
          base: modelPrice,
          final: modelPrice * groupRatio,
        },
      ],
      usage: undefined,
      groupLabel,
      groupRatio,
      unitLabel: 'request',
    }
  }

  const modelRatio = other.model_ratio
  if (!isRatio(modelRatio)) return null
  const inputPrice = modelRatio * USD_PER_MILLION_PER_RATIO
  const completionRatio = other.completion_ratio
  if (!isRatio(completionRatio)) return null

  const isClaude = other.claude === true
  const cacheReadTokens = count(other.cache_tokens)
  // Settlement bills the aggregate cache-write total, of which Anthropic's
  // 5-minute and 1-hour counts are a breakdown.
  const cacheWriteTokens =
    count(other.cache_creation_tokens) || getLogCacheWriteTokens(other)
  const cacheWrite5mTokens = count(other.cache_creation_tokens_5m)
  const cacheWrite1hTokens = count(other.cache_creation_tokens_1h)
  const imageTokens = count(other.image_output)
  const audioInputPrice = count(other.audio_input_price)
  const audioInputTokens = count(other.audio_input_token_count)

  const lines: RatioSettledLine[] = []
  let inputTokens = props.promptTokens

  if (cacheReadTokens > 0) {
    if (!isRatio(other.cache_ratio)) return null
    if (!isClaude) inputTokens -= cacheReadTokens
    lines.push({
      id: 'cr',
      label: 'Cache Read',
      price: inputPrice * other.cache_ratio,
      count: cacheReadTokens,
    })
  }

  if (cacheWriteTokens > 0) {
    if (!isRatio(other.cache_creation_ratio)) return null
    if (!isClaude) {
      inputTokens -= cacheWriteTokens
      lines.push({
        id: 'cc',
        label: 'Cache Write',
        price: inputPrice * other.cache_creation_ratio,
        count: cacheWriteTokens,
      })
    } else {
      const remainder = Math.max(
        0,
        cacheWriteTokens - cacheWrite5mTokens - cacheWrite1hTokens
      )
      if (remainder > 0) {
        lines.push({
          id: 'cc',
          label: 'Cache Write',
          price: inputPrice * other.cache_creation_ratio,
          count: remainder,
        })
      }
      if (cacheWrite5mTokens > 0) {
        const ratio5m =
          other.cache_creation_ratio_5m ?? other.cache_creation_ratio
        if (!isRatio(ratio5m)) return null
        lines.push({
          id: 'cc5m',
          label: 'Cache Write (5m)',
          price: inputPrice * ratio5m,
          count: cacheWrite5mTokens,
        })
      }
      if (cacheWrite1hTokens > 0) {
        if (!isRatio(other.cache_creation_ratio_1h)) return null
        lines.push({
          id: 'cc1h',
          label: 'Cache Write (1h)',
          price: inputPrice * other.cache_creation_ratio_1h,
          count: cacheWrite1hTokens,
        })
      }
    }
  }

  if (imageTokens > 0) {
    if (!isRatio(other.image_ratio)) return null
    inputTokens -= imageTokens
    lines.push({
      id: 'img',
      label: 'Image In',
      price: inputPrice * other.image_ratio,
      count: imageTokens,
    })
  }

  // Audio input priced per million tokens instead of by a model ratio.
  if (audioInputPrice > 0 && audioInputTokens > 0) {
    inputTokens -= audioInputTokens
    lines.push({
      id: 'ai',
      label: 'Audio In',
      price: audioInputPrice,
      count: audioInputTokens,
    })
  }

  const entries: RatioSettledLine[] = [
    {
      id: 'p',
      label: 'Input',
      price: inputPrice,
      count: Math.max(0, inputTokens),
    },
    {
      id: 'c',
      label: 'Output',
      price: inputPrice * completionRatio,
      count: props.completionTokens,
    },
    ...lines,
  ]

  return {
    rows: entries.map((entry) => ({
      id: entry.id,
      label: entry.label,
      base: entry.price,
      final: entry.price * groupRatio,
    })),
    usage: props.withUsageCost
      ? entries.map((entry) => ({
          id: entry.id,
          label: entry.label,
          count: entry.count,
          price: entry.price * groupRatio,
        }))
      : undefined,
    groupLabel,
    groupRatio,
    unitLabel: '1M token',
  }
}

function isRatio(value: number | undefined): value is number {
  return typeof value === 'number' && Number.isFinite(value) && value >= 0
}

function count(value: number | undefined): number {
  return typeof value === 'number' && Number.isFinite(value) ? value : 0
}
