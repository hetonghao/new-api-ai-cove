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
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import type { AxiosAdapter } from 'axios'
import { StrictMode } from 'react'
import { afterEach, describe, expect, it } from 'vitest'

import { api } from '@/lib/api'

import { UpstreamSubscriptions } from '..'
import { QuotaWindow } from '../components/quota-window'
import { SubscriptionRow } from '../components/subscription-row'
import type { UpstreamSubscription } from '../types'

const originalAdapter = api.defaults.adapter

afterEach(() => {
  api.defaults.adapter = originalAdapter
})

const subscription: UpstreamSubscription = {
  id: 1,
  name: 'Quota account',
  platform: 'opencode',
  refresh_interval_minutes: 10,
  auto_refresh_disabled: false,
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
  it('persists automatic quota toggles without replacing other configuration', async () => {
    let current = { ...subscription }
    const changes: unknown[] = []
    api.defaults.adapter = async (config) => {
      if (config.method === 'put') {
        const change: unknown = JSON.parse(config.data)
        changes.push(change)
        current = {
          ...current,
          auto_refresh_disabled: !current.auto_refresh_disabled,
        }
      }
      return {
        config,
        data: {
          success: true,
          data: config.method === 'put' ? current : [current],
        },
        headers: {},
        status: 200,
        statusText: 'OK',
      }
    }
    const client = new QueryClient({
      defaultOptions: {
        queries: { retry: false },
        mutations: { retry: false },
      },
    })
    const view = render(
      <QueryClientProvider client={client}>
        <UpstreamSubscriptions />
      </QueryClientProvider>
    )
    const user = userEvent.setup()
    try {
      await user.click(screen.getByRole('tab', { name: 'Coding Plan' }))
      const toggle = await screen.findByRole('switch', {
        name: 'Automatically update quota for Quota account',
      })
      expect(
        screen.getByRole('group', { name: 'Actions · Quota account' })
      ).toContainElement(toggle)
      await user.click(toggle)
      await waitFor(() => expect(toggle).not.toBeChecked())
      expect(screen.getByText('Automatic quota updates off')).toBeVisible()
      expect(
        screen.getByRole('button', { name: 'Fetch quota for Quota account' })
      ).toBeEnabled()
      await user.click(toggle)
      await waitFor(() => expect(toggle).toBeChecked())
      expect(changes).toEqual([
        { platform: 'opencode', auto_refresh_disabled: true },
        { platform: 'opencode', auto_refresh_disabled: false },
      ])
    } finally {
      view.unmount()
      client.clear()
    }
  })

  it('reloads Coding Plan after switching tabs aborts its first request in StrictMode', async () => {
    let firstSignal: AbortSignal | undefined
    const adapter: AxiosAdapter = async (config) => {
      if (config.url === '/api/devin-monitor') {
        return {
          config,
          headers: {},
          status: 200,
          statusText: 'OK',
          data: {
            success: true,
            data: {
              configured: false,
              collecting: false,
              next_run_at: 0,
              snapshot: null,
            },
          },
        }
      }
      if (!firstSignal && config.signal) {
        const signal = config.signal as AbortSignal
        firstSignal = signal
        await new Promise<void>((_, reject) =>
          signal.addEventListener('abort', () => reject(new Error('aborted')), {
            once: true,
          })
        )
      }
      expect(config.url).toBe('/api/upstream-subscriptions')
      return {
        config,
        data: { success: true, data: [subscription] },
        headers: {},
        status: 200,
        statusText: 'OK',
      }
    }
    api.defaults.adapter = adapter
    const client = new QueryClient({
      defaultOptions: { queries: { retry: false } },
    })
    const view = render(
      <StrictMode>
        <QueryClientProvider client={client}>
          <UpstreamSubscriptions />
        </QueryClientProvider>
      </StrictMode>
    )

    try {
      const user = userEvent.setup()
      const tab = screen.getByRole('tab', { name: 'Coding Plan' })
      await user.click(tab)
      await waitFor(() => expect(firstSignal).toBeDefined())
      await user.click(screen.getByRole('tab', { name: 'Devin' }))
      await user.click(tab)
      expect(
        await screen.findByRole('progressbar', {
          name: 'Quota account · 7-day window',
        })
      ).toHaveAttribute('aria-valuenow', '12')
      expect(
        screen.queryByText('Failed to load subscriptions')
      ).not.toBeInTheDocument()
    } finally {
      view.unmount()
      client.clear()
    }
  })

  it('filters by platform and fuzzy name and sorts expiry with unknown dates last', async () => {
    api.defaults.adapter = async (config) => ({
      config,
      headers: {},
      status: 200,
      statusText: 'OK',
      data: {
        success: true,
        data: [
          { ...subscription, id: 1, name: 'Code Alpha' },
          {
            ...subscription,
            id: 4,
            name: 'Command Account',
            platform: 'commandcode',
            quota: {
              windows: [],
              subscription_period_end: '2027-03-01T00:00:00Z',
            },
          },
          {
            ...subscription,
            id: 2,
            name: 'Design Later',
            account: 'Team-Billing@Example.com',
            platform: 'opendesign',
            quota: {
              windows: [],
              subscription_period_end: '2027-02-01T00:00:00Z',
            },
          },
          {
            ...subscription,
            id: 3,
            name: 'Design Earlier',
            platform: 'opendesign',
            quota: {
              windows: [],
              subscription_period_end: '2027-01-01T00:00:00Z',
            },
          },
        ],
      },
    })
    const client = new QueryClient({
      defaultOptions: { queries: { retry: false } },
    })
    const view = render(
      <QueryClientProvider client={client}>
        <UpstreamSubscriptions />
      </QueryClientProvider>
    )
    const user = userEvent.setup()
    try {
      await user.click(screen.getByRole('tab', { name: 'Coding Plan' }))
      await screen.findByRole('heading', { name: 'Code Alpha' })
      expect(
        screen.getAllByRole('heading', { level: 3 }).map((el) => el.textContent)
      ).toEqual([
        'Command Account',
        'Code Alpha',
        'Design Earlier',
        'Design Later',
      ])
      expect(screen.getByText(/^Current period ends:/)).toBeVisible()
      await user.click(screen.getByRole('button', { name: 'Platform' }))
      await user.click(
        await screen.findByRole('option', { name: /^CommandCode/ })
      )
      expect(screen.getAllByRole('heading', { level: 3 })).toHaveLength(1)
      const selected = await screen.findByRole('option', {
        name: /^CommandCode/,
      })
      expect(selected).toHaveAttribute('aria-selected', 'true')
      await user.click(selected)
      expect(screen.getAllByRole('heading', { level: 3 })).toHaveLength(4)
      await user.keyboard('{Escape}')
      await user.click(
        screen.getByRole('combobox', { name: 'Sort subscriptions by' })
      )
      await user.click(
        screen.getByRole('option', { name: 'Subscription expiry' })
      )
      expect(
        screen.getAllByRole('heading', { level: 3 }).map((el) => el.textContent)
      ).toEqual([
        'Design Earlier',
        'Design Later',
        'Command Account',
        'Code Alpha',
      ])
      await user.click(screen.getByRole('button', { name: 'Ascending' }))
      expect(
        screen.getAllByRole('heading', { level: 3 }).map((el) => el.textContent)
      ).toEqual([
        'Command Account',
        'Design Later',
        'Design Earlier',
        'Code Alpha',
      ])
      await user.click(screen.getByRole('button', { name: 'Platform' }))
      await user.click(
        await screen.findByRole('option', { name: /^OpenDesign Go/ })
      )
      await user.type(
        screen.getByPlaceholderText('Search name or account...'),
        'LATER'
      )
      expect(
        screen.getAllByRole('heading', { level: 3 }).map((el) => el.textContent)
      ).toEqual(['Design Later'])
      await user.clear(screen.getByPlaceholderText('Search name or account...'))
      await user.type(
        screen.getByPlaceholderText('Search name or account...'),
        'billing@EXAMPLE'
      )
      expect(
        screen.getAllByRole('heading', { level: 3 }).map((el) => el.textContent)
      ).toEqual(['Design Later'])
      await user.clear(screen.getByPlaceholderText('Search name or account...'))
      await user.type(
        screen.getByPlaceholderText('Search name or account...'),
        'missing'
      )
      expect(screen.getByText('No matching subscriptions')).toBeVisible()
      await user.click(screen.getByRole('button', { name: 'Clear filters' }))
      expect(screen.getAllByRole('heading', { level: 3 })).toHaveLength(4)
    } finally {
      view.unmount()
      client.clear()
    }
  })

  it('shows all three windows as independently remaining allowance', () => {
    renderSubscription(subscription)

    expect(screen.getAllByRole('progressbar')).toHaveLength(3)
    expect(
      screen.getByRole('progressbar', { name: 'Quota account · 5-hour window' })
    ).toHaveAttribute('aria-valuenow', '98.75')
    expect(
      screen.getByRole('progressbar', { name: 'Quota account · 7-day window' })
    ).toHaveAttribute('aria-valuenow', '12')
    expect(
      screen.getByRole('progressbar', {
        name: 'Quota account · 30-day window',
      })
    ).toHaveAttribute('aria-valuenow', '0')
    expect(screen.getByText('Window quota exhausted')).toBeVisible()
    expect(screen.getByText('Quota exhausted now')).toBeVisible()
    expect(screen.getAllByText(/^Resets in/)).toHaveLength(3)
  })

  it('shows exhaustion for over-limit usage without exposing credit details', () => {
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
      '0'
    )
    expect(screen.getByText('Window quota exhausted')).toBeVisible()
    expect(screen.getByText('Quota exhausted now')).toBeVisible()
    expect(screen.queryByText(/9,007,199,254,740,993/)).not.toBeInTheDocument()
    expect(screen.queryByText('Remaining credits: 7')).not.toBeInTheDocument()
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
    expect(screen.getByText('Quota unknown')).toBeVisible()
    expect(screen.getByText('Quota status unknown')).toBeVisible()
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
      screen.getByRole('progressbar', { name: 'Quota account · 7-day window' })
    ).toHaveAttribute('aria-valuenow', '12')
    expect(screen.getByText('Quota status unknown')).toBeVisible()
    expect(
      screen.getByText('Showing the last successful quota snapshot.')
    ).toBeVisible()
  })

  it.each([
    [0, 100, 'success'],
    [49.99, 50.01, 'success'],
    [50, 50, 'info'],
    [89.99, 10.01, 'info'],
    [90, 10, 'warning'],
    [97.99, 2.01, 'warning'],
    [98, 2, 'destructive'],
    [99.999, 0.001, 'destructive'],
    [100, 0, 'destructive'],
    [101, 0, 'destructive'],
  ])(
    'uses remaining progress and the correct color at %s percent used',
    (used, remaining, tone) => {
      render(
        <QuotaWindow
          window={{
            key: 'rolling',
            duration_seconds: 18000,
            status: 'ok',
            used_percent: used,
            resets_at: null,
          }}
          subscriptionName='Threshold'
        />
      )
      const bar = screen.getByRole('progressbar')
      expect(Number(bar.getAttribute('aria-valuenow'))).toBeCloseTo(
        remaining,
        6
      )
      expect(bar).toHaveClass(`[&_[data-slot=progress-indicator]]:bg-${tone}`)
      expect(screen.queryByText('Window quota exhausted') !== null).toBe(
        used >= 100
      )
    }
  )

  it('shows available status and bound channel routing metadata', () => {
    renderSubscription({
      ...subscription,
      account: 'account@example.com',
      channel_id: 8,
      channel: { id: 8, name: 'Quota account', priority: 9, weight: 20 },
      quota: {
        windows: [
          {
            key: 'rolling',
            duration_seconds: 18000,
            status: 'ok',
            used_percent: 0,
            resets_at: null,
          },
        ],
      },
    })
    expect(screen.getByText('Available now')).toBeVisible()
    expect(
      screen.getByText(/Channel #8 · Priority 9 · Weight 20/)
    ).toBeVisible()
    const account = screen.getByText(
      'Subscription account: account@example.com'
    )
    expect(account.parentElement?.lastElementChild).toBe(account)
  })

  it.each([
    [199020000, 'Resets in 2 days 7 hours 17 minutes'],
    [26220000, 'Resets in 7 hours 17 minutes'],
    [1020000, 'Resets in 17 minutes'],
    [30000, 'Resets in less than 1 minute'],
    [0, 'Reset due'],
    [-60000, 'Reset due'],
  ])(
    'shows the reset countdown for a %s millisecond offset',
    (offset, expected) => {
      const now = Date.parse('2026-11-15T00:00:00Z')
      render(
        <QuotaWindow
          window={{
            key: 'rolling',
            duration_seconds: 18000,
            status: 'ok',
            used_percent: 10,
            resets_at: new Date(now + offset).toISOString(),
          }}
          subscriptionName='Countdown'
          now={now}
        />
      )
      expect(screen.getByText(expected)).toBeVisible()
      expect(screen.getByText(expected)).toHaveAttribute(
        'datetime',
        new Date(now + offset).toISOString()
      )
    }
  )
})
