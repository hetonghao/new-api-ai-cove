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
      start_time: new Date('invalid'),
      end_time: new Date('2026-07-25T12:33:00Z'),
      channel_id: '1.5',
      username: 'a'.repeat(21),
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
        username: ['translated:Invalid configuration'],
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
      start_time: new Date('2026-07-25T12:34:00Z'),
      end_time: new Date('2026-07-25T12:33:00Z'),
      channel_id: '',
      username: '',
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
    const startTime = new Date('2026-07-25T12:34:00Z')

    // When
    const parsed = schema.safeParse({
      start_time: startTime,
      end_time: undefined,
      channel_id: '12',
      username: ' alice ',
      provider_id: '0',
      result: 'not_reviewed',
      source: 'local',
    })

    // Then
    assert.equal(parsed.success, true)
    if (parsed.success) {
      assert.deepEqual(commitRiskRecordFilters(parsed.data), {
        start_timestamp: Math.floor(startTime.getTime() / 1000),
        channel_id: 12,
        username: 'alice',
        provider_id: 0,
        result: 'not_reviewed',
        source: 'local',
      })
    }
  })
})
