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
import { after, test } from 'node:test'

import { Window } from 'happy-dom'
import type React from 'react'

const domWindow = new Window()
const domGlobals = [
  'window',
  'document',
  'navigator',
  'HTMLElement',
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

const { act } = await import('react')
const { createRoot } = await import('react-dom/client')
const { createInstance } = await import('i18next')
const { I18nextProvider, initReactI18next } = await import('react-i18next')

const i18n = createInstance()
await i18n.use(initReactI18next).init({
  lng: 'zh',
  resources: {
    zh: {
      translation: {
        Stream: '流',
        'WebSocket acceleration channel': 'WebSocket 加速通道',
        'From Turbo': '来自 Turbo',
        Version: '版本',
      },
    },
  },
})

const { StreamTpsCell } = await import('../timing-metrics-cell')
const reactTestGlobals = globalThis as typeof globalThis & {
  IS_REACT_ACT_ENVIRONMENT?: boolean
}
reactTestGlobals.IS_REACT_ACT_ENVIRONMENT = true

type RenderedCell = {
  container: HTMLDivElement
  root: ReturnType<typeof createRoot>
}

async function renderCell(
  props: React.ComponentProps<typeof StreamTpsCell>
): Promise<RenderedCell> {
  const container = document.createElement('div')
  document.body.append(container)
  const root = createRoot(container)

  await act(async () => {
    root.render(
      <I18nextProvider i18n={i18n}>
        <StreamTpsCell {...props} />
      </I18nextProvider>
    )
  })

  return { container, root }
}

after(() => {
  domWindow.close()
})

test('流式日志按流、WS、Turbo 的顺序展示可访问标识', async () => {
  const rendered = await renderCell({
    isStream: true,
    isWebSocket: true,
    isTurbo: true,
    turboVersion: 'mac/0.1.0-beta.4',
    tokensPerSecond: 20,
  })

  const webSocket = rendered.container.querySelector<HTMLButtonElement>(
    'button[aria-label="WebSocket 加速通道"]'
  )
  const turbo = rendered.container.querySelector<HTMLButtonElement>(
    'button[aria-label="来自 Turbo / 版本 mac/0.1.0-beta.4"]'
  )

  assert.ok(webSocket)
  assert.ok(turbo)
  assert.equal(
    webSocket.compareDocumentPosition(turbo) & Node.DOCUMENT_POSITION_FOLLOWING,
    Node.DOCUMENT_POSITION_FOLLOWING
  )
  assert.equal(rendered.container.firstElementChild?.textContent, '流WS20 t/s')
  assert.equal(webSocket.className.includes('cursor-default'), true)
  assert.ok(turbo.querySelector('.lucide-zap'))
  assert.equal(turbo.className.includes('cursor-default'), true)
  assert.equal(turbo.className.includes('text-success'), true)

  await act(async () => rendered.root.unmount())
  rendered.container.remove()
})
