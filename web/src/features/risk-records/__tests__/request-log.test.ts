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
import { afterEach, describe, mock, test } from 'node:test'

import { api } from '@/lib/api'

import { getRiskRequestLog, RiskRecordResponseError } from '../api'

describe('risk request log API', () => {
  afterEach(() => mock.restoreAll())

  test('uses the dedicated exact endpoint and preserves an explicit missing result', async () => {
    // Given a request ID that must be encoded as one route segment
    let requestedPath = ''
    mock.method(api, 'get', async (path: string) => {
      requestedPath = path
      return { data: { success: true, message: '', data: null } }
    })

    // When the risk details flow loads its linked usage record
    const response = await getRiskRequestLog('request/with space')

    // Then it avoids the paginated log endpoint and exposes the nullable result
    assert.equal(requestedPath, '/api/log/request/request%2Fwith%20space')
    assert.equal(response.success, true)
    assert.equal(response.data, null)
  })

  test('rejects a malformed response that omits the required data field', async () => {
    // Given a successful-looking response that does not contain a result field
    mock.method(api, 'get', async () => ({
      data: { success: true, message: '' },
    }))

    // When the request details flow parses the response
    const request = getRiskRequestLog('missing-data')

    // Then it enters the anomaly path instead of treating the response as no match
    await assert.rejects(request, RiskRecordResponseError)
  })
})
