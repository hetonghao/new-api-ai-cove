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
import { render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import type { AxiosAdapter } from 'axios'
import { useState } from 'react'
import { afterEach, describe, expect, it } from 'vitest'

import { api } from '@/lib/api'

import { DeviMonMonitor } from '../components/devi-mon-monitor'
import { DEFAULT_DEVIN_SORT } from '../sort'

const originalAdapter = api.defaults.adapter

afterEach(() => {
  api.defaults.adapter = originalAdapter
})

const SNAPSHOT = {
  collected_at: 1_795_000_000,
  duration_ms: 132_000,
  interval_minutes: 180,
  accounts: [
    {
      email: 'busy@example.com',
      used_percent: 99,
      reset_at: 1_795_100_000,
      plan_type: 'pro',
      credit_balance: '15873.43',
      unlimited: false,
      cards: [{ expires_at: '2026-12-01T04:03:00Z' }],
      renew_at: '2026-12-18T13:49:00Z',
      renew_source: 'live',
      devin_disabled: true,
      error: '',
    },
    {
      email: 'fresh@example.com',
      used_percent: 4,
      reset_at: 1_795_200_000,
      plan_type: 'pro',
      credit_balance: '40193.05',
      unlimited: false,
      cards: [
        { expires_at: '2026-12-01T04:14:00Z' },
        { expires_at: '2026-12-02T06:43:00Z' },
      ],
      renew_at: '2026-12-14T12:41:00Z',
      renew_source: 'id_token',
      devin_disabled: null,
      error: '',
    },
  ],
  unpaired_devin: [{ email: 'orphan@example.com', disabled: false }],
  errors: [] as string[],
}

function SortableMonitor() {
  const [sort, setSort] = useState(DEFAULT_DEVIN_SORT)
  return <DeviMonMonitor sort={sort} onSortChange={setSort} />
}

function renderMonitor() {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  })
  return render(
    <QueryClientProvider client={client}>
      <SortableMonitor />
    </QueryClientProvider>
  )
}

function serveState(snapshot: unknown) {
  api.defaults.adapter = (async (config) => ({
    config,
    data: {
      success: true,
      data: {
        snapshot,
        collecting: false,
        next_run_at: 1_795_010_000,
        configured: true,
      },
    },
    headers: {},
    status: 200,
    statusText: 'OK',
  })) as AxiosAdapter
}

describe('devin monitor', () => {
  it('refreshes only via the selected account button and reads the updated row without a full collection', async () => {
    const posts: unknown[] = []
    let release = () => {}
    const hold = new Promise<void>((done) => {
      release = done
    })
    let updated = false
    api.defaults.adapter = (async (config) => {
      let data: unknown = null
      if (config.method === 'post') {
        posts.push([config.url, JSON.parse(config.data)])
        await hold
        updated = true
      } else {
        const snapshot = {
          ...SNAPSHOT,
          accounts: SNAPSHOT.accounts.map((row, i) =>
            i === 0 && updated ? { ...row, used_percent: 40 } : row
          ),
        }
        data = {
          snapshot,
          collecting: false,
          next_run_at: 1_795_010_000,
          configured: true,
        }
      }
      return {
        config,
        data: { success: true, data },
        headers: {},
        status: 200,
        statusText: 'OK',
      }
    }) as AxiosAdapter
    renderMonitor()
    const user = userEvent.setup()
    const heading = await screen.findByRole('heading', {
      name: 'busy@example.com',
    })
    const refresh = screen.getByRole('button', {
      name: 'Refresh busy@example.com',
    })
    await user.click(heading)
    expect(posts).toEqual([])
    await user.click(refresh)
    expect(refresh).toBeDisabled()
    expect(await screen.findByText('1% remaining')).toBeVisible()
    expect(
      screen.getByRole('button', { name: 'Refresh fresh@example.com' })
    ).toBeEnabled()
    expect(posts).toEqual([
      ['/api/devin-monitor/accounts/refresh', { email: 'busy@example.com' }],
    ])
    release()
    expect(await screen.findByText('60% remaining')).toBeVisible()
    expect(screen.getByText('96% remaining')).toBeVisible()
    await waitFor(() => expect(refresh).toBeEnabled())
  })

  it('sorts by Devin priority then weight and switches to known reset and renewal times first', async () => {
    const sample = SNAPSHOT.accounts[0]
    const account = (
      email: string,
      priority: number | null,
      weight: number | null,
      reset_at: number | null,
      renew_at: string
    ) => ({
      ...sample,
      email,
      reset_at,
      renew_at,
      devin_routing: { priority, weight },
    })
    serveState({
      ...SNAPSHOT,
      accounts: [
        account('light', 8, 0, 3, '2026-12-02T09:00:00+08:00'),
        account('negative', -1, 99, null, 'invalid'),
        account('default', null, null, 0, ''),
        account('heavy', 8, 5, 7, '2026-12-02T03:00:00Z'),
        {
          ...sample,
          email: 'unknown',
          reset_at: 5,
          renew_at: '2026-12-01T20:00:00-08:00',
        },
      ],
    })
    const user = userEvent.setup()
    renderMonitor()
    const picker = await screen.findByRole('combobox', {
      name: 'Sort subscriptions by',
    })
    const order = () =>
      screen.getAllByRole('heading', { level: 3 }).map((el) => el.textContent)
    expect(order()).toEqual([
      'heavy',
      'light',
      'default',
      'negative',
      'unknown',
    ])
    await user.click(screen.getByRole('button', { name: 'Descending' }))
    expect(order()).toEqual([
      'negative',
      'default',
      'light',
      'heavy',
      'unknown',
    ])
    await user.click(picker)
    await user.click(await screen.findByRole('option', { name: 'Reset time' }))
    expect(order()).toEqual([
      'light',
      'unknown',
      'heavy',
      'default',
      'negative',
    ])
    await user.click(screen.getByRole('button', { name: 'Ascending' }))
    expect(order()).toEqual([
      'heavy',
      'unknown',
      'light',
      'default',
      'negative',
    ])
    await user.click(picker)
    await user.click(
      await screen.findByRole('option', { name: 'Renewal time' })
    )
    expect(order()).toEqual([
      'light',
      'heavy',
      'unknown',
      'default',
      'negative',
    ])
    await user.click(screen.getByRole('button', { name: 'Ascending' }))
    expect(order()).toEqual([
      'unknown',
      'heavy',
      'light',
      'default',
      'negative',
    ])
  })

  it('shows weekly allowance, reset cards and the Devin enable state per account', async () => {
    serveState(SNAPSHOT)
    renderMonitor()

    expect(await screen.findByText('busy@example.com')).toBeInTheDocument()
    expect(screen.getByText('fresh@example.com')).toBeInTheDocument()

    // Remaining allowance follows the Coding Plan rule: the bar is what is left.
    expect(screen.getByText('1% remaining')).toBeInTheDocument()
    expect(screen.getByText('96% remaining')).toBeInTheDocument()

    const fresh = screen.getByText('fresh@example.com').closest('li')
    if (!fresh) throw new Error('Missing Codex account row')
    expect(within(fresh).getByText('×2')).toBeVisible()
    expect(within(fresh).getAllByRole('listitem')).toHaveLength(2)
    expect(
      fresh.querySelector('time[datetime="2026-12-01T04:14:00Z"]')
    ).toHaveTextContent(/^2026-12-0[12] \d\d:\d\d$/)

    expect(screen.getByText('Devin disabled')).toBeInTheDocument()
    expect(screen.getByText('No Devin account')).toBeInTheDocument()
  })

  it('shows only Devin routing, distinguishes zero from defaults and reads older snapshots', async () => {
    serveState({
      ...SNAPSHOT,
      accounts: [
        { ...SNAPSHOT.accounts[0], devin_routing: { priority: -4, weight: 0 } },
        SNAPSHOT.accounts[1],
        { ...SNAPSHOT.accounts[0], email: 'legacy@example.com' },
      ],
      unpaired_devin: [
        {
          email: 'orphan@example.com',
          disabled: false,
          devin_routing: { priority: null, weight: 25 },
        },
        {
          email: 'defaults@example.com',
          disabled: true,
          devin_routing: { priority: 0, weight: null },
        },
      ],
    })
    renderMonitor()
    const rowFor = async (email: string) => {
      const row = (await screen.findByText(email)).closest('li')
      if (!row) throw new Error('Missing account row')
      return row
    }
    expect(await rowFor('busy@example.com')).toHaveTextContent(
      /Priority -4\s*Weight 0/
    )
    expect(await rowFor('orphan@example.com')).toHaveTextContent(
      /Priority Default 0\s*Weight 25/
    )
    expect(await rowFor('defaults@example.com')).toHaveTextContent(
      /Priority 0\s*Weight Default 1/
    )
    expect(await rowFor('legacy@example.com')).toHaveTextContent(
      'Priority / Weight: Not fetched yet'
    )
    expect(await rowFor('fresh@example.com')).not.toHaveTextContent(
      /Priority|Weight/
    )
  })

  it('marks a renewal read from the sign-in snapshot instead of the live endpoint', async () => {
    serveState(SNAPSHOT)
    renderMonitor()

    expect(await screen.findByText(/\(sign-in snapshot\)/)).toBeInTheDocument()
  })

  it('lists Devin accounts that have no Codex twin', async () => {
    serveState(SNAPSHOT)
    renderMonitor()

    expect(
      await screen.findByText('Devin accounts without a Codex twin')
    ).toBeInTheDocument()
    expect(screen.getByText('orphan@example.com')).toBeInTheDocument()
  })

  it('surfaces collection errors instead of hiding them', async () => {
    serveState({ ...SNAPSHOT, accounts: [], errors: ['cpa-pro-2: HTTP 500'] })
    renderMonitor()

    expect(await screen.findByText('cpa-pro-2: HTTP 500')).toBeInTheDocument()
  })

  it('asks for the management key when the gateway is not configured', async () => {
    api.defaults.adapter = (async (config) => ({
      config,
      data: {
        success: true,
        data: {
          snapshot: null,
          collecting: false,
          next_run_at: 0,
          configured: false,
        },
      },
      headers: {},
      status: 200,
      statusText: 'OK',
    })) as AxiosAdapter
    renderMonitor()

    expect(
      await screen.findByText(
        'Set CPA_MANAGEMENT_KEY on the gateway before collecting.'
      )
    ).toBeInTheDocument()
  })

  it('keeps the previous inventory visible when a new collection is queued', async () => {
    let posted = 0
    let complete = false
    api.defaults.adapter = (async (config) => {
      if (config.method === 'post') posted++
      const busy = posted > 0 && !complete
      return {
        config,
        data: {
          success: true,
          data: {
            snapshot: complete
              ? { ...SNAPSHOT, accounts: [SNAPSHOT.accounts[1]] }
              : SNAPSHOT,
            collecting: busy,
            configured: true,
            next_run_at: busy ? 0 : 1_795_010_000,
            interval_minutes: 180,
          },
        },
        headers: {},
        status: 200,
        statusText: 'OK',
      }
    }) as AxiosAdapter
    renderMonitor()
    await userEvent.click(
      await screen.findByRole('button', { name: 'Collect now' })
    )
    expect(
      await screen.findByRole('button', { name: 'Collecting...' })
    ).toBeDisabled()
    expect(screen.getByText('busy@example.com')).toBeVisible()
    expect(
      screen.getByText('180 minutes after this collection finishes')
    ).toBeVisible()
    expect(screen.queryByText('Not scheduled')).toBeNull()
    complete = true
    await waitFor(
      () => expect(screen.queryByText('busy@example.com')).toBeNull(),
      { timeout: 4500 }
    )
    expect(screen.getByText('fresh@example.com')).toBeVisible()
    expect(screen.getByRole('button', { name: 'Collect now' })).toBeEnabled()
    expect(posted).toBe(1)
  })

  it.each([
    ['cpa-pro-2: HTTP 502', 'cpa-pro-2: HTTP 502'],
    ['task lease expired', 'Collection interrupted'],
  ])('keeps the last good accounts and explains %s', async (error, message) => {
    api.defaults.adapter = (async (config) => ({
      config,
      data: {
        success: true,
        data: {
          snapshot: SNAPSHOT,
          collecting: false,
          configured: true,
          next_run_at: 1_795_010_000,
          last_error: error,
        },
      },
      headers: {},
      status: 200,
      statusText: 'OK',
    })) as AxiosAdapter
    renderMonitor()
    expect(
      await screen.findByText(/Showing the last complete collection/)
    ).toHaveTextContent(message)
    expect(screen.getByText('busy@example.com')).toBeVisible()
    expect(screen.queryByText('Not scheduled')).toBeNull()
  })
})
