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

import type { RiskProvider, RiskProviderFormValues } from '../types'
import {
  canActivateProvider,
  formValuesToPayload,
} from './risk-provider-form.ts'

const formValues: RiskProviderFormValues = {
  name: 'Cloudflare primary',
  provider_type: 'cloudflare',
  model: '@cf/meta/llama-guard-3-8b',
  base_url: 'https://api.cloudflare.com/client/v4/accounts/demo/ai/run',
  credential: '',
  timeout_ms: 800,
  failure_threshold: 5,
  cooldown_seconds: 30,
}

function provider(overrides: Partial<RiskProvider> = {}): RiskProvider {
  return {
    id: 1,
    name: formValues.name,
    provider_type: 'cloudflare',
    model: formValues.model,
    base_url: formValues.base_url,
    has_credential: true,
    timeout_ms: 800,
    failure_threshold: 5,
    cooldown_seconds: 30,
    validated_at: null,
    active: false,
    created_at: '2026-07-25T00:00:00Z',
    updated_at: '2026-07-25T00:00:00Z',
    ...overrides,
  }
}

describe('risk provider form behavior', () => {
  test('omits a blank credential when editing a configured provider', () => {
    // Given a provider edit form whose masked credential input was untouched
    // When the form is converted to the API payload
    const payload = formValuesToPayload(formValues)

    // Then the stored server-side credential is retained
    assert.equal('credential' in payload, false)
  })

  test('sends a replacement credential when the operator enters one', () => {
    // Given a provider edit form with a replacement credential
    // When the form is converted to the API payload
    const payload = formValuesToPayload({
      ...formValues,
      credential: 'replacement-token',
    })

    // Then only the entered replacement is sent
    assert.equal(payload.credential, 'replacement-token')
  })

  test('allows activation only after successful validation', () => {
    // Given saved providers in unvalidated, validated, and active states
    const unvalidated = provider()
    const validated = provider({ validated_at: '2026-07-25T00:10:00Z' })
    const active = provider({
      validated_at: '2026-07-25T00:10:00Z',
      active: true,
    })

    // When the activation affordance is evaluated
    // Then only the validated inactive provider can be selected
    assert.equal(canActivateProvider(unvalidated), false)
    assert.equal(canActivateProvider(validated), true)
    assert.equal(canActivateProvider(active), false)
  })
})
