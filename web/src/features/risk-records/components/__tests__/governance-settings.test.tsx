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

const { QueryClient, QueryClientProvider } =
  await import('@tanstack/react-query')
const { cleanup, fireEvent, render, screen, waitFor } =
  await import('@testing-library/react')
const { createInstance } = await import('i18next')
const { I18nextProvider, initReactI18next } = await import('react-i18next')
const { api } = await import('@/lib/api')
const { RiskRecordGovernanceSettings } =
  await import('../risk-record-governance-settings')

const originalAdapter = api.defaults.adapter
const i18n = createInstance()
await i18n.use(initReactI18next).init({
  lng: 'en',
  resources: {
    en: {
      translation: {
        'Risk record settings': 'Risk record settings',
        'Manage risk record storage and automatic cleanup.':
          'Manage risk record storage and automatic cleanup.',
        'Risk content storage': 'Risk content storage',
        'Save all content': 'Save all content',
        'Save unsafe content only': 'Save unsafe content only',
        'Do not save content': 'Do not save content',
        'Controls whether the redacted detection content is retained in risk records.':
          'Controls whether the redacted detection content is retained in risk records.',
        'Retain last N days': 'Retain last N days',
        'Risk records older than this are deleted by the daily cleanup task.':
          'Risk records older than this are deleted by the daily cleanup task.',
        'Retention days must be between {{min}} and {{max}}':
          'Retention days must be between {{min}} and {{max}}',
        'Preview characters': 'Preview characters',
        'Stores at least {{min}} redacted characters with no maximum limit.':
          'Stores at least {{min}} redacted characters with no maximum limit.',
        'Preview characters must be at least {{min}}':
          'Preview characters must be at least {{min}}',
        'Save settings': 'Save settings',
        'Saving...': 'Saving...',
        'Risk record settings saved': 'Risk record settings saved',
        'Failed to load risk record settings':
          'Failed to load risk record settings',
        'Request failed': 'Request failed',
      },
    },
  },
})

function renderSettings() {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { gcTime: Infinity, retry: false } },
  })
  render(
    <QueryClientProvider client={queryClient}>
      <I18nextProvider i18n={i18n}>
        <RiskRecordGovernanceSettings />
      </I18nextProvider>
    </QueryClientProvider>
  )
  return queryClient
}

describe('risk record governance settings', () => {
  afterEach(() => {
    cleanup()
    api.defaults.adapter = originalAdapter
  })

  after(() => {
    domWindow.close()
  })

  test('loads and saves the risk record retention period', async () => {
    let savedPayload: unknown
    api.defaults.adapter = async (config) => {
      if (config.method === 'put') {
        savedPayload = JSON.parse(String(config.data))
        return {
          config,
          data: { success: true, data: savedPayload },
          headers: {},
          status: 200,
          statusText: 'OK',
        }
      }
      return {
        config,
        data: {
          success: true,
          data: {
            save_scope: 'all',
            content_save_scope: 'all',
            retention_days: 30,
            preview_chars: 200,
          },
        },
        headers: {},
        status: 200,
        statusText: 'OK',
      }
    }
    const queryClient = renderSettings()

    const input = await screen.findByRole('spinbutton', {
      name: 'Retain last N days',
    })
    assert.equal(input.getAttribute('value'), '30')

    fireEvent.change(input, { target: { value: '90' } })
    fireEvent.click(screen.getByRole('button', { name: 'Save settings' }))

    await waitFor(() => {
      assert.deepEqual(savedPayload, {
        save_scope: 'all',
        content_save_scope: 'all',
        retention_days: 90,
        preview_chars: 200,
      })
    })

    queryClient.clear()
  })

  test('blocks a retention period outside the supported range', async () => {
    api.defaults.adapter = async (config) => ({
      config,
      data: {
        success: true,
        data: {
          save_scope: 'all',
          content_save_scope: 'all',
          retention_days: 30,
          preview_chars: 200,
        },
      },
      headers: {},
      status: 200,
      statusText: 'OK',
    })
    const queryClient = renderSettings()

    const input = await screen.findByRole('spinbutton', {
      name: 'Retain last N days',
    })
    fireEvent.change(input, { target: { value: '181' } })

    assert.equal(
      screen.getByRole('alert').textContent,
      'Retention days must be between 1 and 180'
    )
    assert.equal(
      screen
        .getByRole('button', { name: 'Save settings' })
        .hasAttribute('disabled'),
      true
    )

    queryClient.clear()
  })

  test('saves a preview length without imposing a maximum', async () => {
    let savedPayload: unknown
    api.defaults.adapter = async (config) => {
      if (config.method === 'put') {
        savedPayload = JSON.parse(String(config.data))
        return {
          config,
          data: { success: true, data: savedPayload },
          headers: {},
          status: 200,
          statusText: 'OK',
        }
      }
      return {
        config,
        data: {
          success: true,
          data: {
            save_scope: 'all',
            content_save_scope: 'all',
            retention_days: 30,
            preview_chars: 200,
          },
        },
        headers: {},
        status: 200,
        statusText: 'OK',
      }
    }
    const queryClient = renderSettings()

    const input = await screen.findByRole('spinbutton', {
      name: 'Preview characters',
    })
    fireEvent.change(input, { target: { value: '12000' } })
    fireEvent.click(screen.getByRole('button', { name: 'Save settings' }))

    await waitFor(() => {
      assert.deepEqual(savedPayload, {
        save_scope: 'all',
        content_save_scope: 'all',
        retention_days: 30,
        preview_chars: 12000,
      })
    })

    queryClient.clear()
  })

  test('blocks a preview length below fifty characters', async () => {
    api.defaults.adapter = async (config) => ({
      config,
      data: {
        success: true,
        data: {
          save_scope: 'all',
          content_save_scope: 'all',
          retention_days: 30,
          preview_chars: 200,
        },
      },
      headers: {},
      status: 200,
      statusText: 'OK',
    })
    const queryClient = renderSettings()

    const input = await screen.findByRole('spinbutton', {
      name: 'Preview characters',
    })
    fireEvent.change(input, { target: { value: '49' } })

    assert.equal(
      screen.getByRole('alert').textContent,
      'Preview characters must be at least 50'
    )
    assert.equal(
      screen
        .getByRole('button', { name: 'Save settings' })
        .hasAttribute('disabled'),
      true
    )

    queryClient.clear()
  })
})
