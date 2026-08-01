import assert from 'node:assert/strict'
import { test } from 'node:test'

import type { TFunction } from 'i18next'
import React from 'react'
import { renderToStaticMarkup } from 'react-dom/server'
import type { TooltipPayloadEntry } from 'recharts'

import { UserResultTooltip } from '../risk-statistics-shared'

const translate = ((key: string) => key) as TFunction

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
  const html = renderToStaticMarkup(
    React.createElement(UserResultTooltip, {
      active: true,
      affectedUsers: 37,
      payload,
      translate,
    })
  )

  assert.match(html, /Affected users/)
  assert.match(html, />37<\/span>/)
})
