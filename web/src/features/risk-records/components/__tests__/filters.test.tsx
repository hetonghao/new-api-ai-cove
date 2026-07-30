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
  'HTMLButtonElement',
  'HTMLInputElement',
  'HTMLSelectElement',
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

const { cleanup, fireEvent, render, screen, waitFor } =
  await import('@testing-library/react')
const { createInstance } = await import('i18next')
const { I18nextProvider, initReactI18next } = await import('react-i18next')
const { RiskRecordFiltersForm } = await import('../risk-record-filters')

const i18n = createInstance()
await i18n.use(initReactI18next).init({ lng: 'en' })

function renderFilters(
  disabled: boolean,
  onApply: (filters: { readonly provider_type?: string }) => void = () => {}
) {
  render(
    <I18nextProvider i18n={i18n}>
      <RiskRecordFiltersForm
        disabled={disabled}
        initialValues={{
          start_time: new Date('2026-07-29T00:00:00Z'),
          end_time: new Date('2026-07-29T23:59:59Z'),
          channel_id: '',
          username: '',
          provider_id: '',
          provider_type: '',
          result: '',
          source: '',
        }}
        onApply={onApply}
        providers={[]}
      />
    </I18nextProvider>
  )
}

describe('risk record filters presentation', () => {
  afterEach(cleanup)

  after(() => {
    domWindow.close()
  })

  test('shows a disabled spinner in the Search button while records load', () => {
    renderFilters(true)

    const searchButton = screen.getByRole('button', { name: 'Search' })

    assert.equal(searchButton.hasAttribute('disabled'), true)
    assert.ok(searchButton.querySelector('.animate-spin'))
  })

  test('submits the selected cloud review provider type', async () => {
    let submitted: { readonly provider_type?: string } | undefined
    renderFilters(false, (filters) => {
      submitted = filters
    })

    fireEvent.click(screen.getByRole('button', { name: 'Expand' }))
    fireEvent.change(screen.getByRole('combobox', { name: 'Provider type' }), {
      target: { value: 'platform_internal' },
    })
    fireEvent.click(screen.getByRole('button', { name: 'Search' }))

    await waitFor(() => {
      assert.equal(submitted?.provider_type, 'platform_internal')
    })
  })
})
