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

import { t } from 'i18next'

import type { RiskProviderFormValues } from '../types'
import {
  formValuesToPayload,
  getChannelModelOptions,
  getRiskProviderFormSchema,
  getRiskProviderServerFormError,
} from './risk-provider-form.ts'

const formValues: RiskProviderFormValues = {
  name: 'Cloudflare primary',
  provider_type: 'cloudflare',
  model: '@cf/meta/llama-guard-3-8b',
  account_id: '0123456789abcdef0123456789abcdef',
  channel_id: null,
  credential: '',
  timeout_ms: 800,
  failure_threshold: 5,
  cooldown_seconds: 30,
}

describe('risk provider form behavior', () => {
  test('lists unique models from the selected platform channel', () => {
    // Given two channels whose model lists include whitespace and duplicates
    const channels = [
      { id: 24, models: 'guard-b, guard-a, guard-b, ,' },
      { id: 25, models: 'other-model' },
    ]

    // When the internal provider selects channel 24
    const models = getChannelModelOptions(channels, 24)

    // Then only that channel's normalized models are offered in config order
    assert.deepEqual(models, ['guard-b', 'guard-a'])
  })

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

  test('maps an API failure to the form-level server error', () => {
    // Given an unstructured API failure message
    // When the failure is mapped to React Hook Form
    const formError = getRiskProviderServerFormError('Provider already exists')

    // Then the dialog can render it as a form-level error
    assert.deepEqual(formError, {
      name: 'root.server',
      error: { type: 'server', message: 'Provider already exists' },
    })
  })

  test('requires a credential when creating a provider', () => {
    // Given a new provider form with a blank credential
    // When the form schema validates the values
    const result = getRiskProviderFormSchema(t, true).safeParse(formValues)

    // Then the credential field receives the validation issue
    assert.equal(result.success, false)
    if (!result.success) {
      assert.deepEqual(result.error.issues[0]?.path, ['credential'])
    }
  })

  test('allows a blank credential when editing a configured provider', () => {
    // Given an edit form whose existing credential stays masked
    // When the form schema validates the untouched credential field
    const result = getRiskProviderFormSchema(t, false).safeParse(formValues)

    // Then the values remain valid and the stored credential can be retained
    assert.equal(result.success, true)
  })

  test('requires a valid Cloudflare account ID', () => {
    // Given a provider form with an invalid account ID
    const values = { ...formValues, account_id: 'not-an-account-id' }

    // When the form schema validates the values
    const result = getRiskProviderFormSchema(t, false).safeParse(values)

    // Then the account field receives the validation issue
    assert.equal(result.success, false)
    if (!result.success) {
      assert.deepEqual(result.error.issues[0]?.path, ['account_id'])
    }
  })

  test('platform internal provider requires a channel but no credential or account ID', () => {
    const internalValues: RiskProviderFormValues = {
      ...formValues,
      provider_type: 'platform_internal',
      account_id: '',
      channel_id: 24,
      credential: '',
      model: 'guard-model',
    }

    const result = getRiskProviderFormSchema(t, true).safeParse(internalValues)
    assert.equal(result.success, true)
    assert.deepEqual(formValuesToPayload(internalValues), {
      name: 'Cloudflare primary',
      provider_type: 'platform_internal',
      channel_id: 24,
      model: 'guard-model',
      timeout_ms: 800,
      failure_threshold: 5,
      cooldown_seconds: 30,
    })
  })

  test('platform internal provider rejects a missing channel', () => {
    const result = getRiskProviderFormSchema(t, false).safeParse({
      ...formValues,
      provider_type: 'platform_internal',
      account_id: '',
      channel_id: null,
      model: 'guard-model',
    })

    assert.equal(result.success, false)
    if (!result.success) {
      assert.deepEqual(result.error.issues[0]?.path, ['channel_id'])
    }
  })
})
