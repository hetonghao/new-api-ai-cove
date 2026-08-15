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

import type { UsageLog } from '../../data/schema'

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
        'Turbo warm-up request': 'Turbo-预热请求',
        Version: '版本',
      },
    },
  },
})

const { StreamTpsCell } = await import('../timing-metrics-cell')
const { isTurboWarmupLog, parseLogOther } = await import('../../lib/format')
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

const baseUsageLog: UsageLog = {
  id: 1,
  user_id: 1,
  created_at: 1_700_000_000,
  type: 2,
  content: '',
  username: '',
  token_name: '',
  model_name: 'gpt-5.2',
  quota: 0,
  prompt_tokens: 128,
  completion_tokens: 0,
  use_time: 0,
  is_stream: true,
  channel: 1,
  channel_name: '',
  token_id: 1,
  group: '',
  ip: '',
  other: JSON.stringify({
    transport: 'websocket',
    client_source: 'turbo',
    stream_status: { status: 'ok' },
  }),
  request_id: '',
  upstream_request_id: '',
}

function createUsageLog(
  overrides: Partial<UsageLog> = {},
  otherOverrides: Record<string, unknown> = {}
): UsageLog {
  const baseOther = JSON.parse(baseUsageLog.other) as Record<string, unknown>
  return {
    ...baseUsageLog,
    ...overrides,
    other: JSON.stringify({ ...baseOther, ...otherOverrides }),
  }
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

for (const scenario of [
  {
    name: '命中流式 WebSocket Turbo 且有输入无输出',
    log: createUsageLog(),
    expected: true,
  },
  {
    name: '非 WebSocket',
    log: createUsageLog({}, { transport: 'http' }),
    expected: false,
  },
  {
    name: '输入 token 为 0',
    log: createUsageLog({ prompt_tokens: 0 }),
    expected: false,
  },
  {
    name: '已有输出 token',
    log: createUsageLog({ completion_tokens: 4, use_time: 1 }),
    expected: false,
  },
  {
    name: '流状态为 error',
    log: createUsageLog({}, { stream_status: { status: 'error' } }),
    expected: false,
  },
  {
    name: '非流式',
    log: createUsageLog({ is_stream: false }),
    expected: false,
  },
] as const) {
  test(`真实 UsageLog 条件${scenario.name}时${scenario.expected ? '' : '不'}显示预热标签`, async () => {
    const other = parseLogOther(scenario.log.other)
    assert.equal(isTurboWarmupLog(scenario.log, other), scenario.expected)

    const rendered = await renderCell({
      isStream: scenario.log.is_stream,
      isWebSocket: other?.transport === 'websocket' || other?.ws === true,
      isTurbo: other?.client_source === 'turbo',
      isTurboWarmup: isTurboWarmupLog(scenario.log, other),
      tokensPerSecond:
        scenario.log.use_time > 0 && scenario.log.completion_tokens > 0
          ? scenario.log.completion_tokens / scenario.log.use_time
          : null,
      streamStatus: other?.stream_status,
    })

    const badge = rendered.container.querySelector<HTMLElement>(
      '[data-slot="status-badge"]'
    )
    assert.equal(Boolean(badge), scenario.expected)
    assert.equal(
      rendered.container.textContent?.includes('Turbo-预热请求') ?? false,
      scenario.expected
    )
    if (scenario.expected) {
      assert.equal(badge?.className.includes('text-success'), true)
      assert.equal(badge?.className.includes('bg-success/10'), true)
      assert.equal(badge?.className.includes('border-success/30'), true)
      assert.equal(badge?.parentElement?.className.includes('px-0.5'), false)
    }

    await act(async () => rendered.root.unmount())
    rendered.container.remove()
  })
}
