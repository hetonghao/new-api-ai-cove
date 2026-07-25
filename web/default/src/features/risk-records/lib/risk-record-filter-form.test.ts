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
  commitRiskRecordFilters,
  createRiskRecordFilterFormSchema,
} from './risk-records.ts'

const translate = (key: string) => `translated:${key}`

describe('risk record filter form behavior', () => {
  it('rejects invalid field values at the form boundary', () => {
    // Given
    const schema = createRiskRecordFilterFormSchema(translate)

    // When
    const invalid = schema.safeParse({
      start_time: 'not-a-time',
      end_time: '2026-07-25T12:33',
      channel_id: '1.5',
      user_id: '0',
      provider_id: '-1',
      result: 'future-result',
      source: 'future-source',
    })

    // Then
    assert.equal(invalid.success, false)
    if (!invalid.success) {
      assert.deepEqual(invalid.error.flatten().fieldErrors, {
        start_time: ['translated:Invalid configuration'],
        channel_id: ['translated:Please enter a valid number'],
        user_id: ['translated:Please enter a valid number'],
        provider_id: ['translated:Please enter a valid number'],
        result: ['translated:Invalid configuration'],
        source: ['translated:Invalid configuration'],
      })
    }
  })

  it('rejects an end time earlier than the start time', () => {
    // Given
    const schema = createRiskRecordFilterFormSchema(translate)

    // When
    const invalid = schema.safeParse({
      start_time: '2026-07-25T12:34',
      end_time: '2026-07-25T12:33',
      channel_id: '',
      user_id: '',
      provider_id: '',
      result: '',
      source: '',
    })

    // Then
    assert.equal(invalid.success, false)
    if (!invalid.success) {
      assert.deepEqual(invalid.error.flatten().fieldErrors.end_time, [
        'translated:Invalid configuration',
      ])
    }
  })

  it('accepts closed values and preserves local datetime semantics', () => {
    // Given
    const schema = createRiskRecordFilterFormSchema(translate)
    const startTime = '2026-07-25T12:34'

    // When
    const parsed = schema.safeParse({
      start_time: startTime,
      end_time: '',
      channel_id: '12',
      user_id: '34',
      provider_id: '0',
      result: 'not_reviewed',
      source: 'local',
    })

    // Then
    assert.equal(parsed.success, true)
    if (parsed.success) {
      assert.deepEqual(commitRiskRecordFilters(parsed.data), {
        start_timestamp: Math.floor(new Date(startTime).getTime() / 1000),
        channel_id: 12,
        user_id: 34,
        provider_id: 0,
        result: 'not_reviewed',
        source: 'local',
      })
    }
  })
})
