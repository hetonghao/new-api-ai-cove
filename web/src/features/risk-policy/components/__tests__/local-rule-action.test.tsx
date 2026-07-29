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

import type { LocalRiskRule } from '../../types'

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
const { LocalRuleCard } = await import('../local-rule-card')

const i18n = createInstance()
await i18n.use(initReactI18next).init({
  lng: 'en',
  resources: {
    en: {
      translation: {
        'Skip cloud review': 'Skip cloud review',
      },
    },
  },
})

const SKIP_RULE: LocalRiskRule = {
  id: 7,
  rule_type: 'regex',
  pattern: '^heartbeat:',
  action: 'skip',
  enabled: true,
  created_at: '2026-07-29T00:00:00Z',
  updated_at: '2026-07-29T00:00:00Z',
}

describe('local risk rule action presentation', () => {
  afterEach(cleanup)

  after(() => {
    domWindow.close()
  })

  test('shows the configured skip action on the rule card', () => {
    // Given a saved rule that skips AI cloud review
    render(
      <I18nextProvider i18n={i18n}>
        <LocalRuleCard
          rule={SKIP_RULE}
          isPending={false}
          onToggle={() => {}}
          onEdit={() => {}}
          onTest={() => {}}
          onDelete={() => {}}
        />
      </I18nextProvider>
    )

    // When the rule card is presented to the operator
    const action = screen.getByText('Skip cloud review')

    // Then the action is visible without opening the edit dialog
    assert.ok(action)
  })
})
