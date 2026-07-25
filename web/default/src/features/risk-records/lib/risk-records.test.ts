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

import { riskRecordPageSchema } from '../types.ts'
import {
  buildRiskRecordQueryParams,
  commitRiskRecordFilters,
  getRiskRecordResultFilterLabel,
  getRiskRecordResultVariant,
  getRiskRecordSourceFilterLabel,
  getRiskRecordSourceVariant,
  getRiskRecordTotalPages,
} from './risk-records.ts'

const VALID_RECORD = {
  id: 1,
  request_id: 'req-123',
  channel_id: 12,
  user_id: 34,
  rule_ids: [5],
  provider_id: 7,
  provider_name: 'Cloud review',
  result: 'unsafe',
  source: 'provider',
  provider_called: true,
  categories: ['violence'],
  latency_ms: 93,
  prompt_tokens: 11,
  completion_tokens: 2,
  total_tokens: 13,
  neurons: 7,
  error_code: '',
  observed_at: '2026-07-25T12:00:00Z',
} as const

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

  it('maps unsafe records to the danger status when rendered', () => {
    // Given
    const result = 'unsafe'

    // When
    const variant = getRiskRecordResultVariant(result)

    // Then
    assert.equal(variant, 'danger')
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
      'Provider source',
      'Cache source',
      'In-flight source',
      'Local source',
    ])
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
      start_time: '2026-07-25T12:34:00Z',
      end_time: '2026-07-26T01:02:00Z',
      channel_id: '12',
      user_id: '34',
      provider_id: '7',
      result: 'unsafe',
      source: 'provider',
    }

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
      user_id: 34,
      provider_id: 7,
      result: 'unsafe',
      source: 'provider',
    })
  })

  it('preserves provider ID zero in the risk record API params', () => {
    // Given
    const draft = {
      start_time: '',
      end_time: '',
      channel_id: '',
      user_id: '',
      provider_id: '0',
      result: '',
      source: '',
    }

    // When
    const filters = commitRiskRecordFilters(draft)
    const params = buildRiskRecordQueryParams(1, 20, filters)

    // Then
    assert.deepEqual(params, { p: 1, page_size: 20, provider_id: 0 })
  })

  it('omits cleared filters from the risk record API params', () => {
    // Given
    const draft = {
      start_time: '',
      end_time: '',
      channel_id: '',
      user_id: '',
      provider_id: '',
      result: '',
      source: '',
    }

    // When
    const filters = commitRiskRecordFilters(draft)
    const params = buildRiskRecordQueryParams(1, 20, filters)

    // Then
    assert.deepEqual(params, { p: 1, page_size: 20 })
  })
})
