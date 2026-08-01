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

import type { LocalRiskRule, RiskPolicy } from '../types'
import {
  createLocalRuleFormSchema,
  createLocalRuleTestFormSchema,
  createRiskPolicyFormSchema,
  localRuleFormValuesToPayload,
  localRuleTestFormValuesToPayload,
  localRuleTestToFormValues,
  mergeRiskPolicyModelOptions,
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
      enabled_channels: [],
      excluded_user_ids: [],
      excluded_models: [],
      review_mode: 'selective',
      action_mode: 'observe',
    }

    // When the policy is loaded into the public form seam
    const values = riskPolicyToFormValues(policy)

    // Then risk control stays disabled with the documented safe modes
    assert.deepEqual(values, {
      enabled: false,
      enabled_channels: [],
      excluded_user_ids: [],
      excluded_models: [],
      non_blocking_categories: [],
      review_mode: 'selective',
      action_mode: 'observe',
    })
  })

  test('defaults excluded models for policies saved before model exclusions existed', () => {
    // Given a policy returned by an older runtime without the new field
    const policy: RiskPolicy = {
      configured: true,
      enabled: false,
      enabled_channels: [],
      excluded_user_ids: [],
      review_mode: 'selective',
      action_mode: 'observe',
    }

    // When the legacy response is loaded into the current form
    const values = riskPolicyToFormValues(policy)

    // Then the configuration page remains usable with an empty exclusion list
    assert.deepEqual(values.excluded_models, [])
  })

  test('does not require an available provider before enabling risk control', () => {
    const schema = createRiskPolicyFormSchema(translate)

    // When an operator enables risk control with another provider
    const invalid = schema.safeParse({
      enabled: true,
      enabled_channels: [24],
      excluded_user_ids: [42],
      excluded_models: ['codex-auto-review'],
      review_mode: 'selective',
      action_mode: 'observe',
    })

    assert.equal(invalid.success, true)
  })

  test('keeps provider and channels when the policy is disabled', () => {
    // Given a disabled form that still contains a previous selection
    const values = {
      enabled: false,
      enabled_channels: [24],
      excluded_user_ids: [42],
      excluded_models: ['codex-auto-review'],
      review_mode: 'full' as const,
      action_mode: 'block' as const,
    }

    // When the form is converted to the API payload
    const payload = riskPolicyFormValuesToPayload(values)

    // Then only the enabled state changes and the saved selections remain
    assert.deepEqual(payload, {
      enabled: false,
      enabled_channels: [24],
      excluded_user_ids: [42],
      excluded_models: ['codex-auto-review'],
      review_mode: 'full',
      action_mode: 'block',
    })
  })

  test('preserves selected channel ids in the policy payload', () => {
    // Given an enabled form with actual channel selections
    const values = {
      enabled: true,
      enabled_channels: [24, 31],
      excluded_user_ids: [42, 84],
      excluded_models: ['codex-auto-review', 'gpt-5.6'],
      review_mode: 'selective' as const,
      action_mode: 'observe' as const,
    }

    // When the form is converted to the API payload
    const payload = riskPolicyFormValuesToPayload(values)

    // Then the contract preserves the selected channel IDs
    assert.deepEqual(payload, {
      enabled: true,
      enabled_channels: [24, 31],
      excluded_user_ids: [42, 84],
      excluded_models: ['codex-auto-review', 'gpt-5.6'],
      review_mode: 'selective',
      action_mode: 'observe',
    })
  })

  test('requires at least one actual channel when risk control is enabled', () => {
    // Given a validated provider but no selected channel
    const schema = createRiskPolicyFormSchema(translate)

    // When the enabled form crosses the public boundary
    const parsed = schema.safeParse({
      enabled: true,
      enabled_channels: [],
      excluded_user_ids: [],
      excluded_models: [],
      review_mode: 'selective',
      action_mode: 'observe',
    })

    // Then the channel selector owns the validation error
    assert.equal(parsed.success, false)
    if (parsed.success) return
    assert.deepEqual(parsed.error.issues[0]?.path, ['enabled_channels'])
  })

  test('keeps saved excluded models visible when the current catalog no longer lists them', () => {
    // Given one saved model is missing from the current enabled-model catalog
    const catalog = ['gpt-5.6', 'claude-sonnet-4.5']
    const saved = ['codex-auto-review', 'gpt-5.6']

    // When candidates are prepared for the multi-select
    const options = mergeRiskPolicyModelOptions(catalog, saved)

    // Then every saved value remains visible and removable without duplicates
    assert.deepEqual(options, [
      'gpt-5.6',
      'claude-sonnet-4.5',
      'codex-auto-review',
    ])
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
      action: 'skip',
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
      action: 'skip' as const,
      enabled: true,
    }

    // When the form is converted to the API payload
    const payload = localRuleFormValuesToPayload(values)

    // Then the stable API seam receives the intended phrase
    assert.deepEqual(payload, {
      rule_type: 'phrase',
      pattern: 'ignore previous',
      action: 'skip',
      enabled: true,
    })
  })

  test('assigns a missing pattern to the pattern field', () => {
    // Given a new rule without a usable pattern
    const schema = createLocalRuleFormSchema(translate)

    // When React Hook Form resolves the public form schema
    const parsed = schema.safeParse({
      rule_type: 'keyword',
      pattern: '   ',
      action: 'review',
      enabled: true,
    })

    // Then the error can render beside the pattern control
    assert.equal(parsed.success, false)
    if (parsed.success) return
    assert.deepEqual(parsed.error.issues[0]?.path, ['pattern'])
    assert.equal(parsed.error.issues[0]?.message, 'Rule pattern is required')
  })

  test('maps the selected rule to fresh test-dialog values', () => {
    // Given a saved regex rule and text left from a previous test
    const rule: LocalRiskRule = {
      id: 3,
      rule_type: 'regex',
      pattern: '(?P<verb>ignore)\\s+previous',
      action: 'skip',
      enabled: true,
      created_at: '2026-07-25T00:00:00Z',
      updated_at: '2026-07-25T00:00:00Z',
    }

    // When the test dialog opens for that rule
    const values = localRuleTestToFormValues(rule)

    // Then it uses the selected rule and clears the editable test text
    assert.deepEqual(values, {
      rule_type: 'regex',
      pattern: '(?P<verb>ignore)\\s+previous',
      action: 'skip',
      text: '',
    })
  })

  test('assigns missing test text to the text field', () => {
    // Given a valid saved rule with blank test input
    const schema = createLocalRuleTestFormSchema(translate)

    // When React Hook Form resolves the test form schema
    const parsed = schema.safeParse({
      rule_type: 'keyword',
      pattern: 'ignore',
      action: 'review',
      text: '   ',
    })

    // Then the error can render beside the test text control
    assert.equal(parsed.success, false)
    if (parsed.success) return
    assert.deepEqual(parsed.error.issues[0]?.path, ['text'])
    assert.equal(parsed.error.issues[0]?.message, 'Test text is required')
  })

  test('preserves test text while trimming the stored rule pattern', () => {
    // Given a test input whose whitespace is part of the server normalization case
    const values = {
      rule_type: 'phrase' as const,
      pattern: '  ignore previous  ',
      action: 'skip' as const,
      text: '  Ignore   previous  ',
    }

    // When the form is converted to the API payload
    const payload = localRuleTestFormValuesToPayload(values)

    // Then only the persisted rule pattern is trimmed
    assert.deepEqual(payload, {
      rule_type: 'phrase',
      pattern: 'ignore previous',
      action: 'skip',
      text: '  Ignore   previous  ',
    })
  })
})
