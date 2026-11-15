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

import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen } from '@testing-library/react'
import { describe, expect, it } from 'vitest'

import { SubscriptionRow } from '../components/subscription-row'
import type { UpstreamSubscription } from '../types'

const subscription: UpstreamSubscription = {
  id: 1,
  name: 'Quota account',
  platform: 'opencode',
  refresh_interval_minutes: 10,
  last_success_at: 1_800_000_000,
  last_attempt_at: 0,
  next_refresh_at: 0,
  error_code: '',
  stale: false,
  quota: {
    windows: [
      {
        key: 'rolling',
        duration_seconds: 18000,
        status: 'ok',
        used_percent: 1.25,
        resets_at: '2027-01-01T01:00:00Z',
      },
      {
        key: 'weekly',
        duration_seconds: 604800,
        status: 'ok',
        used_percent: 88,
        resets_at: '2027-01-02T01:00:00Z',
      },
      {
        key: 'monthly',
        duration_seconds: 0,
        status: 'ok',
        used_percent: 100,
        resets_at: '2027-01-03T01:00:00Z',
      },
    ],
  },
}

function renderSubscription(value: UpstreamSubscription) {
  const client = new QueryClient({
    defaultOptions: { mutations: { retry: false } },
  })
  render(
    <QueryClientProvider client={client}>
      <ul>
        <SubscriptionRow
          subscription={value}
          onEdit={() => undefined}
          onRemove={() => undefined}
        />
      </ul>
    </QueryClientProvider>
  )
}

describe('subscription allowance windows', () => {
  it('shows all three OpenCode windows with independent labeled progress values', () => {
    renderSubscription(subscription)

    expect(screen.getAllByRole('progressbar')).toHaveLength(3)
    expect(
      screen.getByRole('progressbar', { name: 'Quota account · 5-hour window' })
    ).toHaveAttribute('aria-valuenow', '1.25')
    expect(
      screen.getByRole('progressbar', { name: 'Quota account · Weekly window' })
    ).toHaveAttribute('aria-valuenow', '88')
    expect(
      screen.getByRole('progressbar', {
        name: 'Quota account · Monthly window',
      })
    ).toHaveAttribute('aria-valuenow', '100')
    expect(screen.getByText('Limit reached')).toBeVisible()
    expect(screen.getAllByText('Resets at')).toHaveLength(3)
  })

  it('keeps exact large credits and the provider remaining value when usage exceeds its limit', () => {
    renderSubscription({
      ...subscription,
      platform: 'opendesign',
      quota: {
        windows: [
          {
            key: 'weekly',
            duration_seconds: 604800,
            status: 'ok',
            used_percent: 100.0013333,
            used_credits: '9007199254740993',
            limit_credits: '9007199254740992',
            remaining_credits: '7',
            resets_at: '2027-01-03T01:00:00Z',
          },
        ],
      },
    })

    expect(screen.getByRole('progressbar')).toHaveAttribute(
      'aria-valuenow',
      '100'
    )
    expect(screen.getByText('Over limit')).toBeVisible()
    expect(
      screen.getByText(/9,007,199,254,740,993 \/ 9,007,199,254,740,992/)
    ).toBeVisible()
    expect(screen.getByText('Remaining credits: 7')).toBeVisible()
  })

  it('labels an unavailable percentage without showing a fabricated zero progress value', () => {
    renderSubscription({
      ...subscription,
      quota: {
        windows: [
          {
            key: 'weekly',
            duration_seconds: 604800,
            status: 'limited',
            used_percent: null,
            resets_at: null,
          },
        ],
      },
    })

    expect(screen.queryByRole('progressbar')).not.toBeInTheDocument()
    expect(screen.getByText('Unknown used')).toBeVisible()
    expect(screen.getByText('Restricted by platform')).toBeVisible()
  })

  it('retains the last progress values and explains an expired credential', () => {
    renderSubscription({
      ...subscription,
      stale: true,
      error_code: 'unauthorized',
    })

    expect(screen.getByText('Stale data')).toBeVisible()
    expect(screen.getByRole('status')).toHaveTextContent('Credential expired.')
    expect(
      screen.getByRole('progressbar', { name: 'Quota account · Weekly window' })
    ).toHaveAttribute('aria-valuenow', '88')
    expect(
      screen.getByText('Showing the last successful quota snapshot.')
    ).toBeVisible()
  })
})
