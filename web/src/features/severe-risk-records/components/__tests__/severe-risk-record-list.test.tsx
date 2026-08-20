/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as published by
the Free Software Foundation, either version 3 of the License, or
(at your option) any later version.
*/
import assert from 'node:assert/strict'
import { after, afterEach, test } from 'node:test'

import { Window } from 'happy-dom'

const domWindow = new Window()
for (const key of [
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
] as const) {
  Object.defineProperty(globalThis, key, {
    configurable: true,
    value: domWindow[key],
  })
}

const { QueryClient, QueryClientProvider } =
  await import('@tanstack/react-query')
const { cleanup, fireEvent, render, screen } =
  await import('@testing-library/react')
const { createInstance } = await import('i18next')
const { I18nextProvider, initReactI18next } = await import('react-i18next')
const { api } = await import('@/lib/api')
const { SevereRiskRecordList } = await import('../severe-risk-record-list')

const originalAdapter = api.defaults.adapter
const i18n = createInstance()
await i18n.use(initReactI18next).init({ lng: 'en' })

const RECORD = {
  id: 1,
  request_id: 'severe-request-1',
  channel_id: 37,
  channel_name: 'Codex',
  user_id: 56,
  username: 'user@example.com',
  token_id: 8,
  token_name: 'Codex token',
  model: 'gpt-5.6-sol',
  path: '/v1/responses',
  error_code: 'invalid_prompt',
  error_detail: 'Invalid prompt',
  context_hash: 'hash',
  channel_scope: 'all',
  user_action_status: 'success',
  channel_action_status: 'success',
  triggered_at: '2026-08-21T08:32:00Z',
} as const

afterEach(() => {
  cleanup()
  api.defaults.adapter = originalAdapter
})

after(() => {
  domWindow.close()
})

test('loads a severe record and opens its full context detail', async () => {
  api.defaults.adapter = async (config) => ({
    config,
    data:
      config.url === '/api/risk/severe-records/1'
        ? {
            success: true,
            data: {
              record: RECORD,
              context: '{"messages":[{"role":"user","content":"safe context"}]}',
            },
          }
        : {
            success: true,
            data: { items: [RECORD], total: 1, page: 1, page_size: 20 },
          },
    headers: {},
    status: 200,
    statusText: 'OK',
  })

  const queryClient = new QueryClient({
    defaultOptions: { queries: { gcTime: Infinity, retry: false } },
  })
  render(
    <QueryClientProvider client={queryClient}>
      <I18nextProvider i18n={i18n}>
        <SevereRiskRecordList />
      </I18nextProvider>
    </QueryClientProvider>
  )

  assert.ok(await screen.findByText('user@example.com'))
  fireEvent.click(screen.getByRole('button', { name: 'View' }))

  assert.ok(await screen.findByText('severe-request-1'))
  assert.ok(await screen.findByText(/safe context/))
})
