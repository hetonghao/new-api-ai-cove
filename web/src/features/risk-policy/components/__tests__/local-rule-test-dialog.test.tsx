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
import { after, afterEach, describe, mock, test } from 'node:test'

import { Window } from 'happy-dom'

import { api } from '@/lib/api'

import type { LocalRiskRule } from '../../types'

const domWindow = new Window()
const domGlobals = [
  'window',
  'document',
  'navigator',
  'HTMLElement',
  'HTMLButtonElement',
  'HTMLTextAreaElement',
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

const { cleanup, fireEvent, render, screen } =
  await import('@testing-library/react')
const { createInstance } = await import('i18next')
const { I18nextProvider, initReactI18next } = await import('react-i18next')
const { LocalRuleTestDialog } = await import('../local-rule-test-dialog')
const reactTestGlobals = globalThis as typeof globalThis & {
  IS_REACT_ACT_ENVIRONMENT?: boolean
}
reactTestGlobals.IS_REACT_ACT_ENVIRONMENT = true

const i18n = createInstance()
await i18n.use(initReactI18next).init({
  lng: 'en',
  resources: {
    en: {
      translation: {
        'Test local rule': 'Test local rule',
        'Test text': 'Test text',
        'Keywords and phrases use normalized text; Go regular expressions use the text as entered.':
          'Keywords and phrases use normalized text; Go regular expressions use the text as entered.',
        'Test result': 'Test result',
        Matched: 'Matched',
        'Send to cloud review': 'Send to cloud review',
        Close: 'Close',
        'Run test': 'Run test',
        'Rule pattern is required': 'Rule pattern is required',
        'Test text is required': 'Test text is required',
      },
    },
  },
})

const REGEX_RULE: LocalRiskRule = {
  id: 3,
  rule_type: 'regex',
  pattern: '^Calculate and respond with ONLY the number, nothing else\\.$',
  action: 'review',
  enabled: true,
  created_at: '2026-07-31T00:00:00Z',
  updated_at: '2026-07-31T00:00:00Z',
}

function renderDialog() {
  render(
    <I18nextProvider i18n={i18n}>
      <LocalRuleTestDialog
        open
        rule={REGEX_RULE}
        onOpenChange={() => undefined}
      />
    </I18nextProvider>
  )
}

describe('local risk rule test dialog', () => {
  afterEach(() => {
    cleanup()
    mock.restoreAll()
  })

  after(() => {
    domWindow.close()
  })

  test('shows raw regex input and keeps the result tied to the submitted text', async () => {
    // Given a saved Go regex rule and a successful test endpoint
    mock.method(api, 'post', async () => ({
      data: {
        success: true,
        message: '',
        data: {
          normalized_text:
            'calculate and respond with only the number, nothing else.',
          matched: true,
          action: 'review',
        },
      },
    }))
    renderDialog()

    const testText = 'Calculate and respond with ONLY the number, nothing else.'
    const textarea = screen.getByRole('textbox', { name: 'Test text' })

    // When the operator tests the exact original input
    fireEvent.change(textarea, { target: { value: testText } })
    fireEvent.click(screen.getByRole('button', { name: 'Run test' }))

    // Then the UI explains the raw-input semantics and shows the submitted text
    assert.ok(await screen.findByText('Matched'))
    assert.ok(
      screen.getByText(
        'Keywords and phrases use normalized text; Go regular expressions use the text as entered.'
      )
    )
    assert.equal(
      screen.getByText(testText, { selector: 'code' }).textContent,
      testText
    )

    // When the operator edits the textarea after the result arrives
    fireEvent.change(textarea, { target: { value: 'edited after testing' } })

    // Then the result remains visibly tied to the input that was actually tested
    assert.equal(
      screen.getByText(testText, { selector: 'code' }).textContent,
      testText
    )
    assert.equal(
      screen.queryByText('edited after testing', { selector: 'code' }),
      null
    )
  })
})
