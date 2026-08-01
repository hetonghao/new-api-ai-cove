import assert from 'node:assert/strict'
import { after, afterEach, test } from 'node:test'

import { Window } from 'happy-dom'
import React from 'react'
import type { TooltipPayloadEntry } from 'recharts'

import type { RiskRecordFilters } from '@/features/risk-records/types'

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

const { cleanup, fireEvent, render, screen } =
  await import('@testing-library/react')
const { createInstance } = await import('i18next')
const { I18nextProvider, initReactI18next } = await import('react-i18next')
const { UserResultChart } = await import('../risk-statistics-charts')
const { UserResultTooltip } = await import('../risk-statistics-shared')

const i18n = createInstance()
await i18n.use(initReactI18next).init({ lng: 'en' })

afterEach(cleanup)

after(() => {
  domWindow.close()
})

test('shows the filtered unique affected-user count in the user tooltip', () => {
  const payload: TooltipPayloadEntry[] = [
    {
      graphicalItemId: 'user',
      payload: {
        label: 'alice',
        username: 'alice',
        user_id: 7,
        safe: 2,
        unsafe: 1,
        errors: 0,
        not_reviewed: 0,
        total: 3,
      },
    },
  ]
  render(
    React.createElement(
      I18nextProvider,
      { i18n },
      React.createElement(UserResultTooltip, {
        active: true,
        affectedUsers: 37,
        payload,
      })
    )
  )

  assert.ok(screen.getByText('Affected users'))
  assert.ok(screen.getByText('37'))
})

test('opens the selected user from the keyboard chart action', () => {
  const navigation: RiskRecordFilters[] = []
  render(
    React.createElement(
      I18nextProvider,
      { i18n },
      React.createElement(UserResultChart, {
        users: [
          {
            user_id: 7,
            username: 'alice',
            safe: 1,
            unsafe: 0,
            errors: 0,
            not_reviewed: 0,
            total: 1,
          },
          {
            user_id: 8,
            username: 'bob',
            safe: 2,
            unsafe: 0,
            errors: 0,
            not_reviewed: 0,
            total: 2,
          },
        ],
        affectedUsers: 0,
        loading: false,
        emptyText: 'No risk statistics',
        recordFilters: { start_timestamp: 100, end_timestamp: 200 },
        onNavigateToRecords: (filters) => navigation.push(filters),
      })
    )
  )

  fireEvent.keyDown(
    screen.getByRole('button', { name: /User result distribution: alice/ }),
    { key: 'ArrowRight' }
  )
  fireEvent.keyDown(
    screen.getByRole('button', { name: /User result distribution: bob/ }),
    { key: 'Enter' }
  )

  const selectionHint = screen.getByText('bob', {
    selector: 'span',
  }).parentElement
  assert.equal(selectionHint?.getAttribute('aria-live'), 'polite')
  assert.deepEqual(navigation, [
    { start_timestamp: 100, end_timestamp: 200, user_id: 8 },
  ])
})
