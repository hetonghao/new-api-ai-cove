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

import { shouldRefetchRiskRecords } from './risk-records.ts'

describe('risk record query behavior', () => {
  it('detects whether submitted filters keep the same query', () => {
    const filters = {
      start_timestamp: 1_784_982_840,
      channel_id: 12,
      username: 'alice',
      result: 'unsafe',
    }

    assert.equal(shouldRefetchRiskRecords(0, filters, { ...filters }), true)
    assert.equal(shouldRefetchRiskRecords(1, filters, { ...filters }), false)
    assert.equal(
      shouldRefetchRiskRecords(0, filters, { ...filters, result: 'safe' }),
      false
    )
    assert.equal(
      shouldRefetchRiskRecords(0, filters, {
        start_timestamp: filters.start_timestamp,
        channel_id: filters.channel_id,
        username: filters.username,
      }),
      false
    )
  })
})
