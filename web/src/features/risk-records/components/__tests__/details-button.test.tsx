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

const { cleanup, render, screen } = await import('@testing-library/react')
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

  test('shows the error code in destructive text when the record failed', () => {
    renderDetailsButton({
      ...BASE_RECORD,
      result: 'error',
      error_code: 'timeout',
      total_tokens: 0,
    })

    const label = getLabel('timeout')

    assert.equal(label.classList.contains('text-destructive'), true)
    assert.equal(label.getAttribute('title'), 'timeout')
  })

  test('shows the translated error fallback in destructive text when the code is empty', () => {
    renderDetailsButton({
      ...BASE_RECORD,
      result: 'error',
      total_tokens: 0,
    })

    const label = getLabel('Error')

    assert.equal(label.classList.contains('text-destructive'), true)
    assert.equal(label.getAttribute('title'), 'Error')
  })

  test('keeps the token count in the normal link color when the record succeeded', () => {
    renderDetailsButton(BASE_RECORD)

    const label = getLabel('12 tokens')

    assert.equal(label.classList.contains('text-destructive'), false)
    assert.equal(label.getAttribute('title'), '12 tokens')
  })
})
