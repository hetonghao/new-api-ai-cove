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
import { after, afterEach, describe, test } from 'node:test'

import { Window } from 'happy-dom'

import type { RiskRecord } from '../../types'

const domWindow = new Window()
const domGlobals = [
  'window',
  'document',
  'navigator',
  'HTMLElement',
  'HTMLButtonElement',
  'SVGElement',
  'Node',
  'Element',
  'Event',
  'CustomEvent',
  'MutationObserver',
  'requestAnimationFrame',
  'cancelAnimationFrame',
  'getComputedStyle',
] as const

for (const key of domGlobals) {
  Object.defineProperty(globalThis, key, {
    configurable: true,
    value: domWindow[key],
  })
}

const { cleanup, fireEvent, render, screen, within } =
  await import('@testing-library/react')
const { createInstance } = await import('i18next')
const { I18nextProvider, initReactI18next } = await import('react-i18next')
const { RiskRecordDetailsButton } =
  await import('../risk-record-details-dialog')

const i18n = createInstance()
await i18n.use(initReactI18next).init({
  lng: 'en',
  resources: {
    en: {
      translation: {
        Error: 'Error',
        'Error details': 'Error details',
        'No error details available': 'No error details available',
        'Risk record details': 'Risk record details',
        '{{count}} tokens': '{{count}} tokens',
      },
    },
  },
})

const BASE_RECORD: RiskRecord = {
  id: 1,
  request_id: 'request-1',
  channel_id: 1,
  channel_name: 'AI-Cove',
  user_id: 1,
  username: 'HTH',
  token_id: 1,
  token_name: 'A',
  model: '@cf/meta/llama-guard-3-8b',
  path: '/v1/responses',
  preview: '',
  content_hash: '',
  rule_ids: [],
  provider_id: 1,
  provider_name: 'HTH',
  provider_type: 'cloudflare',
  result: 'safe',
  source: 'provider',
  provider_called: true,
  cache_hit: false,
  blocked: false,
  categories: [],
  latency_ms: 800,
  prompt_tokens: 0,
  completion_tokens: 0,
  total_tokens: 12,
  neurons: 0,
  chunks: [],
  error_code: '',
  error_detail: '',
  observed_at: '2026-07-28T02:15:11Z',
}

function renderDetailsButton(record: RiskRecord) {
  render(
    <I18nextProvider i18n={i18n}>
      <RiskRecordDetailsButton record={record} onUserClick={() => {}} />
    </I18nextProvider>
  )
}

function getLabel(name: string): HTMLElement {
  const button = screen.getByRole('button', { name })
  const label = button.querySelector('span')
  assert.ok(label)
  return label
}

describe('risk record details button presentation', () => {
  afterEach(cleanup)

  after(() => {
    domWindow.close()
  })

  test('shows the error code in warning text when the record failed', () => {
    renderDetailsButton({
      ...BASE_RECORD,
      result: 'error',
      error_code: 'timeout',
      total_tokens: 0,
    })

    const label = getLabel('timeout')

    assert.equal(label.classList.contains('text-warning'), true)
    assert.equal(label.classList.contains('text-destructive'), false)
    assert.equal(label.getAttribute('title'), 'timeout')
  })

  test('shows the translated error fallback in warning text when the code is empty', () => {
    renderDetailsButton({
      ...BASE_RECORD,
      result: 'error',
      total_tokens: 0,
    })

    const label = getLabel('Error')

    assert.equal(label.classList.contains('text-warning'), true)
    assert.equal(label.classList.contains('text-destructive'), false)
    assert.equal(label.getAttribute('title'), 'Error')
  })

  test('keeps the token count in the normal link color when the record succeeded', () => {
    renderDetailsButton(BASE_RECORD)

    const label = getLabel('12 tokens')

    assert.equal(label.classList.contains('text-warning'), false)
    assert.equal(label.classList.contains('text-destructive'), false)
    assert.equal(label.getAttribute('title'), '12 tokens')
  })

  test('opens the risk record dialog when the error detail is clicked', () => {
    renderDetailsButton({
      ...BASE_RECORD,
      result: 'error',
      error_code: 'timeout',
      total_tokens: 0,
    })

    fireEvent.click(screen.getByRole('button', { name: 'timeout' }))

    assert.ok(screen.getByRole('dialog'))
    assert.ok(screen.getByText('Risk record details'))
  })

  test('opens a separate dialog with the saved provider error detail', () => {
    // Given a failed risk record with a sanitized provider diagnostic
    renderDetailsButton({
      ...BASE_RECORD,
      result: 'error',
      error_code: 'provider_error',
      error_detail: 'HTTP 429: rate limit exceeded',
      total_tokens: 0,
    })

    // When the operator opens the record and then its error name
    fireEvent.click(screen.getByRole('button', { name: 'provider_error' }))
    const recordDialog = screen.getByRole('dialog', {
      name: 'Risk record details',
    })
    const errorButton = within(recordDialog).getByRole('button', {
      name: 'provider_error',
    })
    assert.equal(errorButton.classList.contains('text-warning'), true)
    assert.equal(errorButton.classList.contains('text-destructive'), false)
    fireEvent.click(errorButton)

    // Then diagnostics open in their own dialog without expanding the record
    const errorDialog = screen.getByRole('dialog', { name: 'Error details' })
    assert.ok(within(errorDialog).getByText('HTTP 429: rate limit exceeded'))
  })

  test('shows the historical fallback when an error has no saved detail', () => {
    // Given a historical failed record created before error details existed
    renderDetailsButton({
      ...BASE_RECORD,
      result: 'error',
      error_code: 'provider_error',
      error_detail: null,
      total_tokens: 0,
    })

    // When the operator opens the error detail dialog
    fireEvent.click(screen.getByRole('button', { name: 'provider_error' }))
    const recordDialog = screen.getByRole('dialog', {
      name: 'Risk record details',
    })
    fireEvent.click(
      within(recordDialog).getByRole('button', { name: 'provider_error' })
    )

    // Then the absence of recoverable detail is explicit
    const errorDialog = screen.getByRole('dialog', { name: 'Error details' })
    assert.ok(within(errorDialog).getByText('No error details available'))
  })
})
