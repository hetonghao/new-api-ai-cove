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
import { readFileSync } from 'node:fs'
import { join } from 'node:path'
import { after, afterEach, beforeEach, describe, test } from 'node:test'

import { Window } from 'happy-dom'

import zh from '@/i18n/locales/zh.json'

const domWindow = new Window()
const domGlobals = [
  'window',
  'self',
  'document',
  'navigator',
  'HTMLElement',
  'HTMLCanvasElement',
  'HTMLImageElement',
  'SVGElement',
  'Node',
  'Element',
  'Event',
  'CustomEvent',
  'DOMRect',
  'MutationObserver',
  'requestAnimationFrame',
  'cancelAnimationFrame',
  'getComputedStyle',
  'Image',
  'localStorage',
] as const

for (const key of domGlobals) {
  Object.defineProperty(globalThis, key, {
    configurable: true,
    value: domWindow[key],
  })
}

const { cleanup, render } = await import('@testing-library/react')
const {
  createMemoryHistory,
  createRootRoute,
  createRouter,
  RouterProvider,
} = await import('@tanstack/react-router')
const { createInstance } = await import('i18next')
const { I18nextProvider, initReactI18next } = await import('react-i18next')
const { HomeFooter } = await import('../home-footer')
const { useSystemConfigStore } = await import('@/stores/system-config-store')

const i18n = createInstance()
await i18n.use(initReactI18next).init({
  lng: 'zh',
  resources: { zh },
})

const stylesheet = document.createElement('style')
stylesheet.textContent = readFileSync(
  join(process.cwd(), 'src/styles/index.css'),
  'utf8'
)
document.head.append(stylesheet)

describe('home footer', () => {
  beforeEach(() => {
    Object.defineProperty(domWindow, 'innerWidth', {
      configurable: true,
      value: 375,
    })
    Object.defineProperty(document, 'hidden', {
      configurable: true,
      value: false,
    })
    useSystemConfigStore.getState().setConfig({
      systemName: 'AI Cove',
      logo: '/logo.png',
    })
    useSystemConfigStore.getState().setLoadedLogoUrl('/logo.png')
  })

  afterEach(() => cleanup())

  after(() => domWindow.close())

  test('renders both products and readable copy at 375px without WebGL2', async () => {
    Object.defineProperty(domWindow.HTMLCanvasElement.prototype, 'getContext', {
      configurable: true,
      value: () => null,
    })
    const rootRoute = createRootRoute({
      component: () => (
        <I18nextProvider i18n={i18n}>
          <HomeFooter />
        </I18nextProvider>
      ),
    })
    const router = createRouter({
      routeTree: rootRoute,
      history: createMemoryHistory({ initialEntries: ['/'] }),
    })
    await router.load()

    const rendered = render(<RouterProvider router={router} />)
    const cards = rendered.container.querySelectorAll('article')
    const main = rendered.container.querySelector('.home-footer-main')
    const platformCopy = rendered.container.querySelector(
      '.home-footer-platform > p'
    )
    const copy = rendered.container.querySelector('.home-footer-product-copy p')
    const canvas = rendered.container.querySelector('canvas')

    assert.equal(cards.length, 2)
    assert.ok(cards[0]?.querySelector('[aria-label="AI  Cove Design"]'))
    assert.match(cards[1]?.textContent ?? '', /Cove Turbo/)
    assert.match(cards[0]?.textContent ?? '', /的⁠模型/)
    assert.match(cards[1]?.textContent ?? '', /Codex 会话/)
    assert.equal(canvas?.getAttribute('aria-hidden'), 'true')
    assert.equal(main ? getComputedStyle(main).gridTemplateColumns : '', '1fr')
    assert.equal(
      platformCopy ? getComputedStyle(platformCopy).fontSize : '',
      '14px'
    )
    assert.equal(copy ? getComputedStyle(copy).fontSize : '', '14px')
  })
})
