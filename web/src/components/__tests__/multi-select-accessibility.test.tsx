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

const domWindow = new Window()
const domGlobals = [
  'window',
  'document',
  'navigator',
  'HTMLElement',
  'HTMLInputElement',
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

const { cleanup, render } = await import('@testing-library/react')
const { createInstance } = await import('i18next')
const { I18nextProvider, initReactI18next } = await import('react-i18next')
const { MultiSelect } = await import('../multi-select')

const i18n = createInstance()
await i18n.use(initReactI18next).init({
  lng: 'en',
  resources: {
    en: {
      translation: {
        'Select items...': 'Select items...',
        'No matching items': 'No matching items',
      },
    },
  },
})

describe('MultiSelect accessibility', () => {
  afterEach(() => cleanup())

  after(() => domWindow.close())

  test('forwards validation attributes to the real input', () => {
    const rendered = render(
      <I18nextProvider i18n={i18n}>
        <MultiSelect
          id='risk-provider-pool'
          options={[]}
          selected={[]}
          onChange={() => undefined}
          aria-invalid
          aria-describedby='risk-provider-pool-error'
        />
      </I18nextProvider>
    )
    const input = rendered.container.querySelector('input#risk-provider-pool')

    assert.ok(input)
    assert.equal(input.getAttribute('aria-invalid'), 'true')
    assert.equal(
      input.getAttribute('aria-describedby'),
      'risk-provider-pool-error'
    )
  })
})
