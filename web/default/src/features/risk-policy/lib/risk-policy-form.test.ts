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

import type { RiskPolicy } from '../types'
import {
  createLocalRuleFormSchema,
  createRiskPolicyFormSchema,
  localRuleFormValuesToPayload,
  riskPolicyFormValuesToPayload,
  riskPolicyToFormValues,
} from './risk-policy-form.ts'

const translate = (key: string) => key

describe('risk policy form behavior', () => {
  test('maps an unconfigured policy to the safe disabled defaults', () => {
    // Given the API defaults returned when no policy exists
    const policy: RiskPolicy = {
      configured: false,
      enabled: false,
      provider_id: null,
      enabled_channels: [],
      review_mode: 'selective',
      action_mode: 'observe',
    }

    // When the policy is loaded into the public form seam
    const values = riskPolicyToFormValues(policy)

    // Then risk control stays disabled with the documented safe modes
    assert.deepEqual(values, {
      enabled: false,
      provider_id: '',
      review_mode: 'selective',
      action_mode: 'observe',
    })
  })

  test('requires a validated provider before enabling CPA Pro risk control', () => {
    // Given only provider 7 has passed connection validation
    const schema = createRiskPolicyFormSchema([7], translate)

    // When an operator enables risk control with another provider
    const invalid = schema.safeParse({
      enabled: true,
      provider_id: '9',
      review_mode: 'selective',
      action_mode: 'observe',
    })

    // Then the public form boundary rejects the unvalidated selection
    assert.equal(invalid.success, false)
  })

  test('clears provider and channels when the policy is disabled', () => {
    // Given a disabled form that still contains a previous selection
    const values = {
      enabled: false,
      provider_id: '7',
      review_mode: 'full' as const,
      action_mode: 'block' as const,
    }

    // When the form is converted to the API payload
    const payload = riskPolicyFormValuesToPayload(values)

    // Then the server receives the only valid disabled representation
    assert.deepEqual(payload, {
      provider_id: null,
      enabled_channels: [],
      review_mode: 'full',
      action_mode: 'block',
    })
  })

  test('enables only CPA Pro with the selected validated provider', () => {
    // Given an enabled form with a validated provider
    const values = {
      enabled: true,
      provider_id: '7',
      review_mode: 'selective' as const,
      action_mode: 'observe' as const,
    }

    // When the form is converted to the API payload
    const payload = riskPolicyFormValuesToPayload(values)

    // Then the contract uses the sole supported channel
    assert.deepEqual(payload, {
      provider_id: 7,
      enabled_channels: ['cpa-pro'],
      review_mode: 'selective',
      action_mode: 'observe',
    })
  })
})

describe('local risk rule form behavior', () => {
  test('accepts Go RE2 syntax without applying JavaScript regex semantics', () => {
    // Given a named capture supported by Go RE2 but not JavaScript RegExp
    const schema = createLocalRuleFormSchema(translate)

    // When the rule crosses the public form boundary
    const parsed = schema.safeParse({
      rule_type: 'regex',
      pattern: '(?P<verb>ignore)\\s+previous',
      enabled: true,
    })

    // Then syntax validation is left to the backend contract
    assert.equal(parsed.success, true)
  })

  test('trims rule edges while preserving the selected behavior', () => {
    // Given an enabled phrase rule with accidental outer whitespace
    const values = {
      rule_type: 'phrase' as const,
      pattern: '  ignore previous  ',
      enabled: true,
    }

    // When the form is converted to the API payload
    const payload = localRuleFormValuesToPayload(values)

    // Then the stable API seam receives the intended phrase
    assert.deepEqual(payload, {
      rule_type: 'phrase',
      pattern: 'ignore previous',
      enabled: true,
    })
  })
})
