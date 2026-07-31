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
import { describe, it } from 'node:test'

import {
  riskRecordGovernanceSchema,
  riskRecordPageSchema,
  type RiskRecordFilterDraft,
} from '../types.ts'
import {
  buildRiskRecordQueryParams,
  commitRiskRecordFilters,
  getRiskRecordResultFilterLabel,
  getRiskRecordResultLabel,
  getRiskRecordResultVariant,
  getRiskRecordCategoryLabel,
  getRiskRecordLatencyTone,
  getRiskRecordSourceFilterLabel,
  getRiskRecordSourceVariant,
  getRiskRecordTotalPages,
} from './risk-records.ts'

const VALID_RECORD = {
  id: 1,
  request_id: 'req-123',
  channel_id: 12,
  channel_name: 'CPA Pro',
  user_id: 34,
  username: 'alice',
  token_id: 56,
  token_name: 'Codex',
  model: 'gpt-5.6',
  path: '/v1/responses',
  preview: 'redacted moderation content',
  content_hash: 'a'.repeat(64),
  rule_ids: [5],
  provider_id: 7,
  provider_name: 'Cloud review',
  provider_type: 'cloudflare',
  result: 'unsafe',
  source: 'provider',
  provider_called: true,
  cache_hit: false,
  blocked: true,
  categories: ['violence'],
  latency_ms: 93,
  prompt_tokens: 11,
  completion_tokens: 2,
  total_tokens: 13,
  neurons: 7,
  error_code: '',
  observed_at: '2026-07-25T12:00:00Z',
} as const

const VALID_CHUNKS = [
  {
    index: 0,
    result: 'safe',
    categories: ['clean'],
    latency_ms: 41,
    prompt_tokens: 11,
    completion_tokens: 2,
    total_tokens: 13,
    neurons: 7,
  },
  {
    index: 1,
    result: 'unsafe',
    categories: ['violence', 'threat'],
    latency_ms: 52,
    prompt_tokens: 17,
    completion_tokens: 3,
    total_tokens: 20,
    neurons: 9,
  },
] as const

describe('risk record behavior', () => {
  it('parses a valid paginated API payload when records are returned', () => {
    // Given
    const payload = {
      items: [VALID_RECORD],
      total: 1,
      page: 1,
      page_size: 20,
    }

    // When
    const result = riskRecordPageSchema.safeParse(payload)

    // Then
    assert.equal(result.success, true)
  })

  it('keeps an unknown result and source renderable when the API evolves', () => {
    // Given
    const payload = {
      items: [{ ...VALID_RECORD, result: 'pending', source: 'future-source' }],
      total: 1,
      page: 1,
      page_size: 20,
    }

    // When
    const result = riskRecordPageSchema.safeParse(payload)

    // Then
    assert.equal(result.success, true)
    if (result.success) {
      assert.equal(result.data.items[0]?.result, 'pending')
      assert.equal(result.data.items[0]?.source, 'future-source')
      assert.equal(result.data.items[0]?.provider_called, true)
    }
  })

  it('keeps per-chunk audit details from the risk record API', () => {
    // Given
    const payload = {
      items: [{ ...VALID_RECORD, chunks: VALID_CHUNKS }],
      total: 1,
      page: 1,
      page_size: 20,
    }

    // When
    const result = riskRecordPageSchema.safeParse(payload)

    // Then
    assert.equal(result.success, true)
    if (result.success) {
      assert.deepEqual(result.data.items[0]?.chunks, VALID_CHUNKS)
    }
  })

  it('keeps saved content and request metadata for the details dialog', () => {
    // Given
    const payload = {
      items: [VALID_RECORD],
      total: 1,
      page: 1,
      page_size: 20,
    }

    // When
    const result = riskRecordPageSchema.safeParse(payload)

    // Then
    assert.equal(result.success, true)
    if (result.success) {
      const record = result.data.items[0]
      assert.equal(record?.preview, 'redacted moderation content')
      assert.equal(record?.content_hash, 'a'.repeat(64))
      assert.equal(record?.model, 'gpt-5.6')
      assert.equal(record?.path, '/v1/responses')
      assert.equal(record?.blocked, true)
      assert.equal(record?.channel_name, 'CPA Pro')
      assert.equal(record?.username, 'alice')
      assert.equal(record?.token_name, 'Codex')
    }
  })

  it('accepts an empty saved-content preview when storage is disabled', () => {
    // Given
    const payload = {
      items: [{ ...VALID_RECORD, preview: '', content_hash: '' }],
      total: 1,
      page: 1,
      page_size: 20,
    }

    // When
    const result = riskRecordPageSchema.safeParse(payload)

    // Then
    assert.equal(result.success, true)
  })

  it('parses the global risk content storage setting', () => {
    // Given
    const payload = {
      save_scope: 'all',
      content_save_scope: 'unsafe',
      retention_days: 30,
      safe_preview_chars: 200,
      non_safe_preview_chars: 800,
    }

    // When
    const result = riskRecordGovernanceSchema.safeParse(payload)

    // Then
    assert.equal(result.success, true)
    if (result.success) {
      assert.equal(result.data.content_save_scope, 'unsafe')
      assert.equal(result.data.safe_preview_chars, 200)
      assert.equal(result.data.non_safe_preview_chars, 800)
    }
  })

  it('maps the legacy preview length to both result-specific settings', () => {
    const result = riskRecordGovernanceSchema.safeParse({
      save_scope: 'all',
      content_save_scope: 'all',
      retention_days: 30,
      preview_chars: 1200,
    })

    assert.equal(result.success, true)
    if (result.success) {
      assert.equal(result.data.safe_preview_chars, 1200)
      assert.equal(result.data.non_safe_preview_chars, 1200)
    }
  })

  it('normalizes historical null chunks to an empty array', () => {
    // Given
    const payload = {
      items: [{ ...VALID_RECORD, chunks: null }],
      total: 1,
      page: 1,
      page_size: 20,
    }

    // When
    const result = riskRecordPageSchema.safeParse(payload)

    // Then
    assert.equal(result.success, true)
    if (result.success) {
      assert.deepEqual(result.data.items[0]?.chunks, [])
    }
  })

  it('maps parsed chunk results to visible labels and status variants', () => {
    // Given
    const payload = {
      items: [{ ...VALID_RECORD, chunks: VALID_CHUNKS }],
      total: 1,
      page: 1,
      page_size: 20,
    }

    // When
    const result = riskRecordPageSchema.safeParse(payload)
    const chunks = result.success ? (result.data.items[0]?.chunks ?? []) : []

    // Then
    assert.deepEqual(
      chunks.map((chunk) => getRiskRecordResultLabel(chunk.result)),
      ['Safe', 'Unsafe']
    )
    assert.deepEqual(
      chunks.map((chunk) => getRiskRecordResultVariant(chunk.result)),
      ['success', 'danger']
    )
  })

  it('maps unsafe records to the danger status when rendered', () => {
    // Given
    const result = 'unsafe'

    // When
    const variant = getRiskRecordResultVariant(result)

    // Then
    assert.equal(variant, 'danger')
  })

  it('maps error records to the warning status when rendered', () => {
    // Given
    const result = 'error'

    // When
    const variant = getRiskRecordResultVariant(result)

    // Then
    assert.equal(variant, 'warning')
  })

  it('maps an unknown result to a neutral status when rendered', () => {
    // Given
    const result = 'future-result'

    // When
    const variant = getRiskRecordResultVariant(result)

    // Then
    assert.equal(variant, 'neutral')
  })

  it('maps an unknown source to a neutral status when rendered', () => {
    // Given
    const source = 'future-source'

    // When
    const variant = getRiskRecordSourceVariant(source)

    // Then
    assert.equal(variant, 'neutral')
  })

  it('maps latency quartiles to green blue yellow and red', () => {
    assert.equal(getRiskRecordLatencyTone(0), 'green')
    assert.equal(getRiskRecordLatencyTone(375), 'green')
    assert.equal(getRiskRecordLatencyTone(376), 'blue')
    assert.equal(getRiskRecordLatencyTone(750), 'blue')
    assert.equal(getRiskRecordLatencyTone(751), 'yellow')
    assert.equal(getRiskRecordLatencyTone(1125), 'yellow')
    assert.equal(getRiskRecordLatencyTone(1126), 'red')
    assert.equal(getRiskRecordLatencyTone(1500), 'red')
    assert.equal(getRiskRecordLatencyTone(5000), 'red')
  })

  it('maps every risk record filter value to a visible label', () => {
    // Given
    const resultValues = [
      'all-results',
      'safe',
      'unsafe',
      'error',
      'not_reviewed',
    ]
    const sourceValues = [
      'all-sources',
      'provider',
      'cache',
      'inflight',
      'local',
    ]

    // When
    const resultLabels = resultValues.map(getRiskRecordResultFilterLabel)
    const sourceLabels = sourceValues.map(getRiskRecordSourceFilterLabel)

    // Then
    assert.deepEqual(resultLabels, [
      'All results',
      'Safe',
      'Unsafe',
      'Error',
      'Not reviewed',
    ])
    assert.deepEqual(sourceLabels, [
      'All sources',
      'Cloud review source',
      'Cache source',
      'In-flight source',
      'Local source',
    ])
  })

  it('maps Llama Guard category codes to readable labels', () => {
    assert.equal(getRiskRecordCategoryLabel('S1'), 'Violent crimes')
    assert.equal(getRiskRecordCategoryLabel('s14'), 'Code interpreter abuse')
    assert.equal(getRiskRecordCategoryLabel('future-category'), undefined)
  })

  it('calculates the final partial page when total is not divisible', () => {
    // Given
    const total = 21
    const pageSize = 20

    // When
    const pages = getRiskRecordTotalPages(total, pageSize)

    // Then
    assert.equal(pages, 2)
  })

  it('serializes committed filters into the risk record API params', () => {
    // Given
    const draft = {
      start_time: new Date('2026-07-25T12:34:00Z'),
      end_time: new Date('2026-07-26T01:02:00Z'),
      channel_id: '12',
      username: 'alice',
      provider_id: '7',
      provider_type: 'platform_internal',
      result: 'unsafe',
      source: 'provider',
    } satisfies RiskRecordFilterDraft

    // When
    const filters = commitRiskRecordFilters(draft)
    const params = buildRiskRecordQueryParams(2, 20, filters)

    // Then
    assert.deepEqual(params, {
      p: 2,
      page_size: 20,
      start_timestamp: 1_784_982_840,
      end_timestamp: 1_785_027_720,
      channel_id: 12,
      username: 'alice',
      provider_id: 7,
      provider_type: 'platform_internal',
      result: 'unsafe',
      source: 'provider',
    })
  })

  it('preserves provider ID zero in the risk record API params', () => {
    // Given
    const draft = {
      start_time: undefined,
      end_time: undefined,
      channel_id: '',
      username: '',
      provider_id: '0',
      provider_type: '',
      result: '',
      source: '',
    } satisfies RiskRecordFilterDraft

    // When
    const filters = commitRiskRecordFilters(draft)
    const params = buildRiskRecordQueryParams(1, 20, filters)

    // Then
    assert.deepEqual(params, { p: 1, page_size: 20, provider_id: 0 })
  })

  it('omits cleared filters from the risk record API params', () => {
    // Given
    const draft = {
      start_time: undefined,
      end_time: undefined,
      channel_id: '',
      username: '',
      provider_id: '',
      provider_type: '',
      result: '',
      source: '',
    } satisfies RiskRecordFilterDraft

    // When
    const filters = commitRiskRecordFilters(draft)
    const params = buildRiskRecordQueryParams(1, 20, filters)

    // Then
    assert.deepEqual(params, { p: 1, page_size: 20 })
  })
})
