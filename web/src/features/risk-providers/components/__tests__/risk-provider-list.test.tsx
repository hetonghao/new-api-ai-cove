/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as published by
the Free Software Foundation, either version 3 of the License, or
(at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.
*/
import assert from 'node:assert/strict'
import { after, afterEach, test } from 'node:test'

import { Window } from 'happy-dom'

import type { RiskProvider } from '../../types'

const domWindow = new Window()
let viewportWidth = 1280
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

Object.defineProperty(domWindow, 'matchMedia', {
  configurable: true,
  value: (query: string) => ({
    matches: query === '(max-width: 640px)' && viewportWidth <= 640,
    media: query,
    addEventListener: () => {},
    removeEventListener: () => {},
    addListener: () => {},
    removeListener: () => {},
    onchange: null,
    dispatchEvent: () => false,
  }),
})

const { cleanup, fireEvent, render, screen } =
  await import('@testing-library/react')
const { createInstance } = await import('i18next')
const { I18nextProvider, initReactI18next } = await import('react-i18next')
const { RiskProviderList } = await import('../risk-provider-list')

const i18n = createInstance()
await i18n.use(initReactI18next).init({ lng: 'en' })

const PROVIDER: RiskProvider = {
  id: 1,
  name: 'Cloudflare primary',
  provider_type: 'cloudflare',
  account_id: '0123456789abcdef0123456789abcdef',
  channel_id: 0,
  model: '@cf/meta/llama-guard-3-8b',
  has_credential: true,
  system_managed: false,
  timeout_ms: 800,
  failure_threshold: 5,
  cooldown_seconds: 30,
  priority: 10,
  daily_neurons_limit: 10000,
  daily_reset_time: '08:00',
  current_status: 'normal',
  daily_neurons_used: 0,
  daily_neurons_reserved: 0,
  daily_neurons_remaining: 10000,
  daily_neurons_reset_at: null,
  validated_at: '2026-08-01T00:00:00Z',
  active: true,
  created_at: '2026-08-01T00:00:00Z',
  updated_at: '2026-08-01T00:00:00Z',
}

afterEach(() => {
  cleanup()
  domWindow.localStorage.clear()
})

after(() => {
  domWindow.close()
})

function renderProviderList(): void {
  render(
    <I18nextProvider i18n={i18n}>
      <RiskProviderList
        providers={[PROVIDER]}
        isLoading={false}
        error={null}
        pendingProviderId={null}
        pendingAction={null}
        onRetry={() => {}}
        onRefresh={() => {}}
        isRefreshing={false}
        onCreate={() => {}}
        onEdit={() => {}}
        onValidate={() => {}}
        onDelete={() => {}}
        onToggleActive={() => {}}
      />
    </I18nextProvider>
  )
}

function setViewport(width: number): void {
  viewportWidth = width
  Object.defineProperty(domWindow, 'innerWidth', {
    configurable: true,
    value: width,
  })
}

test('keeps the model discoverable in the 375px compact provider row', () => {
  setViewport(375)
  renderProviderList()

  const model = screen.getByText('@cf/meta/llama-guard-3-8b')
  assert.equal(model.classList.contains('lg:hidden'), true)
  assert.equal(model.parentElement?.classList.contains('overflow-hidden'), true)
})

test('switches views while keeping enabled and current status distinct', () => {
  setViewport(1280)
  renderProviderList()

  assert.equal(
    screen
      .getByRole('button', { name: 'Table view' })
      .getAttribute('aria-pressed'),
    'true'
  )
  assert.ok(screen.getByRole('columnheader', { name: 'Enabled status' }))
  assert.ok(screen.getByRole('columnheader', { name: 'Current status' }))

  fireEvent.click(screen.getByRole('button', { name: 'Card view' }))

  assert.equal(
    screen
      .getByRole('button', { name: 'Card view' })
      .getAttribute('aria-pressed'),
    'true'
  )
  assert.ok(screen.getByText('@cf/meta/llama-guard-3-8b'))
  assert.ok(screen.getByText('Normal'))
  assert.ok(screen.getByText('Enabled'))
})
