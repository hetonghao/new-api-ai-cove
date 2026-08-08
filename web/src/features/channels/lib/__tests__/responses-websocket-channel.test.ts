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
import { describe, test } from 'node:test'

import { RESPONSES_WEBSOCKET_CHANNEL_TYPES } from '../../constants'
import { channelSchema } from '../../types'
import {
  CHANNEL_FORM_DEFAULT_VALUES,
  transformChannelToFormDefaults,
  transformFormDataToCreatePayload,
} from '../channel-form'

function channelWithSettings(type: number, settings: string) {
  return channelSchema.parse({
    id: 1,
    type,
    key: '',
    status: 1,
    name: 'OpenAI',
    created_time: 0,
    test_time: 0,
    response_time: 0,
    balance_updated_time: 0,
    settings,
  })
}

describe('Responses WebSocket channel settings', () => {
  test('defaults to disabled and restores enabled OpenAI settings', () => {
    assert.equal(CHANNEL_FORM_DEFAULT_VALUES.supports_websockets, false)
    assert.equal(
      transformChannelToFormDefaults(
        channelWithSettings(1, '{"supports_websockets":true}')
      ).supports_websockets,
      true
    )
  })

  test('serializes the capability for all Responses WebSocket channel types', () => {
    for (const type of RESPONSES_WEBSOCKET_CHANNEL_TYPES) {
      const channel = transformFormDataToCreatePayload({
        ...CHANNEL_FORM_DEFAULT_VALUES,
        name: `channel-${type}`,
        type,
        key: 'test-key',
        models: 'gpt-5.4',
        supports_websockets: true,
      })
      assert.equal(
        JSON.parse(String(channel.channel.settings)).supports_websockets,
        true
      )
    }

    const nonOpenAI = transformFormDataToCreatePayload({
      ...CHANNEL_FORM_DEFAULT_VALUES,
      name: 'Anthropic',
      type: 14,
      key: 'test-key',
      models: 'claude-sonnet-4-5',
      settings: '{"supports_websockets":true}',
      supports_websockets: true,
    })
    assert.equal(
      'supports_websockets' in JSON.parse(String(nonOpenAI.channel.settings)),
      false
    )
  })
})
