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
import { act, render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import type { AxiosAdapter } from 'axios'
import { useState } from 'react'
import { afterEach, describe, expect, it, vi } from 'vitest'

import { api } from '@/lib/api'

import { deviMonQueryKey } from '../api'
import { DeviMonMonitor } from '../components/devi-mon-monitor'
import { DevinRequestHistory } from '../components/devi-mon-request-history'
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
  const view = render(
    <QueryClientProvider client={client}>
      <SortableMonitor />
    </QueryClientProvider>
  )
  return { ...view, client }
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
  it('interpolates both halves of the CPA scale and separates cumulative counts from the recent rate', async () => {
    const recent = [
      { success: 0, failed: 4 },
      { success: 1, failed: 3 },
      { success: 2, failed: 2 },
      { success: 3, failed: 1 },
      { success: 4, failed: 0 },
    ]
    render(
      <DevinRequestHistory
        email='lamp@example.test'
        history={{ success: 990, failed: 10, as_of: 1_795_000_000, recent }}
      />
    )
    const history = within(
      screen.getByRole('group', { name: 'Request history · lamp@example.test' })
    )
    expect(history.getByText('990')).toBeVisible()
    expect(history.getByTitle('Success rate')).toHaveTextContent('50%')
    const lamps = history.getAllByRole('button')
    expect(lamps).toHaveLength(20)
    for (const [index, color] of [
      'rgb(239, 68, 68)',
      'rgb(245, 136, 45)',
      'rgb(250, 204, 21)',
      'rgb(142, 201, 58)',
      'rgb(34, 197, 94)',
    ].entries()) {
      expect(lamps[index + 15].firstElementChild).toHaveStyle({
        backgroundColor: color,
      })
    }
    expect(lamps[0]).toHaveAccessibleName(/No requests/)
    await userEvent.hover(lamps[16])
    const tip = await screen.findByRole('dialog')
    expect(tip).toHaveTextContent('Success 1')
    expect(tip).toHaveTextContent('Failed 3')
    expect(tip).toHaveTextContent('(25.0%)')
    await userEvent.keyboard('{Escape}')
    await waitFor(() => expect(screen.queryByRole('dialog')).toBeNull())
  })

  it('opens a request window from the keyboard when operation mode is off', async () => {
    const user = userEvent.setup()
    render(
      <DevinRequestHistory
        email='lamp@example.test'
        history={{
          success: 3,
          failed: 1,
          as_of: 1_795_000_000,
          recent: [{ success: 3, failed: 1 }],
        }}
      />
    )
    await user.tab()
    const lamp = screen.getByRole('button', { name: /Success 3 · Failed 1/ })
    expect(lamp).toHaveFocus()
    await user.keyboard('{Enter}')
    expect(await screen.findByRole('dialog')).toHaveTextContent('75.0%')
    expect(lamp).not.toHaveAttribute('title', 'Please turn on operation mode')
    await user.keyboard('{ArrowLeft}')
    expect(await screen.findByRole('dialog')).toHaveTextContent('No requests')
    await user.keyboard('{ArrowRight}')
    expect(lamp).toHaveFocus()
    expect(await screen.findByRole('dialog')).toHaveTextContent('75.0%')
  })

  it('locks account changes on paired and unpaired rows without blocking a manual refresh or offering card use', async () => {
    const user = userEvent.setup()
    const writes: string[] = []
    let successes = 12
    const paired = {
      ...SNAPSHOT.accounts[0],
      reset_key: 'tracked-account',
      auto_reset: true,
      reset_offer: {
        id: '996feb4e-ffe7-5bc3-817b-88bd8ac1c081',
        expires_at: '2026-12-01T04:03:00Z',
      },
    }
    const emails = [paired.email, 'orphan@example.com']
    api.defaults.adapter = async (config) => {
      let data: unknown
      if (config.method !== 'get') {
        writes.push(config.url ?? '')
        successes++
        data = null
      } else if (config.url?.endsWith('/accounts/devin')) {
        data = Object.fromEntries(
          emails.map((email) => [
            email,
            {
              email,
              disabled: false,
              devin_routing: { priority: 2, weight: 1 },
              requests: {
                success: successes,
                failed: 0,
                as_of: 1_795_000_000,
                recent: [],
              },
            },
          ])
        )
      } else if (config.url?.endsWith('/accounts')) {
        data = []
      } else {
        data = {
          configured: true,
          collecting: false,
          next_run_at: 0,
          snapshot: { ...SNAPSHOT, accounts: [paired] },
        }
      }
      return {
        config,
        data: { success: true, data },
        headers: {},
        status: 200,
        statusText: 'OK',
      }
    }
    renderMonitor()
    const mode = await screen.findByRole('switch', { name: 'Operation mode' })
    expect(mode).not.toBeChecked()
    for (const email of emails) {
      const status = await screen.findByRole('switch', {
        name: `Disable ${email}`,
      })
      const priority = screen.getByRole('button', {
        name: `Priority · ${email}: 2`,
      })
      const weight = screen.getByRole('group', { name: `Weight · ${email}` })
      expect(status).toBeDisabled()
      expect(priority).toBeDisabled()
      expect(
        within(weight).getByRole('button', { name: 'Increment' })
      ).toBeDisabled()
      await user.click(status)
      await user.click(priority)
      expect(screen.queryByRole('textbox')).toBeNull()
    }
    const policy = screen.getByRole('radio', {
      name: `Disable when quota runs out · ${paired.email}`,
    })
    const resume = screen.getByRole('checkbox', {
      name: /Enable after quota resets · busy@example/,
    })
    expect(policy).toBeDisabled()
    expect(resume).toHaveAttribute('aria-disabled', 'true')
    const offer = screen.getByRole('button', { name: 'Confirm card use' })
    expect(offer).toBeDisabled()
    await user.click(policy)
    await user.click(resume)
    await user.click(offer)
    const locked = screen.getByRole('group', {
      name: `Devin · ${paired.email}`,
    })
    expect(locked).toHaveAccessibleDescription('Please turn on operation mode')
    locked.focus()
    await user.keyboard('{Enter} ')
    expect(screen.queryByRole('alertdialog')).toBeNull()
    expect(writes).toEqual([])
    await user.click(
      screen.getByRole('button', { name: `Refresh ${paired.email}` })
    )
    const history = within(
      screen.getByRole('group', { name: `Request history · ${paired.email}` })
    )
    expect(await history.findByText('13')).toBeVisible()
    expect(screen.queryByRole('alertdialog')).toBeNull()
    expect(writes).toEqual(['/api/devin-monitor/accounts/refresh'])
    await user.click(
      screen.getByRole('button', {
        name: `Subscription channel for ${emails[1]}`,
      })
    )
    expect(await screen.findByRole('dialog')).toBeVisible()
    await user.keyboard('{Escape}')
    await user.click(mode)
    expect(policy).toBeEnabled()
    expect(offer).toBeEnabled()
    await user.click(mode)
    expect(policy).toBeDisabled()
    expect(offer).toBeDisabled()
  })

  it('reads request history on entry and at the four-minute CPA interval without collecting quota', async () => {
    const reads: string[] = []
    vi.useFakeTimers()
    try {
      api.defaults.adapter = async (config) => {
        reads.push(`${config.method ?? ''} ${config.url}`)
        const data = config.url?.endsWith('/accounts/devin')
          ? {}
          : {
              snapshot: SNAPSHOT,
              collecting: false,
              configured: true,
              next_run_at: 0,
            }
        return {
          config,
          data: { success: true, data },
          headers: {},
          status: 200,
          statusText: 'OK',
        }
      }
      renderMonitor()
      const historyReads = () =>
        reads.filter((r) => r === 'get /api/devin-monitor/accounts/devin')
          .length
      const advance = (ms: number) =>
        act(async () => {
          await vi.advanceTimersByTimeAsync(ms)
        })
      await advance(10)
      expect(historyReads()).toBe(1)
      await advance(239_000)
      expect(historyReads()).toBe(1)
      await advance(1_100)
      expect(historyReads()).toBe(2)
      expect(reads.some((r) => r.startsWith('post '))).toBe(false)
    } finally {
      vi.useRealTimers()
    }
  })

  it('only offers a card in card mode and dismisses an open confirmation when the account goes silent', async () => {
    const mutations: [string | undefined, unknown][] = []
    let action = 'disable'
    let refreshed = false
    api.defaults.adapter = (async (config) => {
      let data: unknown = null
      if (config.method === 'put') {
        mutations.push([config.url, JSON.parse(config.data)])
        action = JSON.parse(config.data).exhaustion_action
      } else if (config.method === 'post') {
        mutations.push([config.url, JSON.parse(config.data)])
        refreshed = true
      } else {
        data = {
          snapshot: {
            ...SNAPSHOT,
            accounts: SNAPSHOT.accounts.map((row, index) => ({
              ...row,
              reset_key: `tracked-${index}`,
              exhaustion_action: index === 0 ? action : 'disable',
              used_percent: index === 0 && refreshed ? 100 : row.used_percent,
              reset_offer:
                index === 0 && refreshed
                  ? {
                      id: '2d96f420-fd03-53e9-8e77-0bfe30c83ace',
                      expires_at: '2026-12-01T04:03:00Z',
                    }
                  : undefined,
            })),
          },
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
    }) satisfies AxiosAdapter
    const { client } = renderMonitor()
    const user = userEvent.setup()
    await user.click(
      await screen.findByRole('switch', { name: 'Operation mode' })
    )
    const busy = await screen.findByRole('radio', {
      name: 'Auto reset when quota runs out · busy@example.com',
    })
    const fresh = screen.getByRole('radio', {
      name: 'Auto reset when quota runs out · fresh@example.com',
    })
    expect(busy).not.toBeChecked()
    expect(fresh).not.toBeChecked()
    await user.click(busy)
    await waitFor(() => expect(busy).toBeChecked())
    expect(fresh).not.toBeChecked()
    await user.click(
      screen.getByRole('button', { name: 'Refresh busy@example.com' })
    )
    const dialog = await screen.findByRole('alertdialog', {
      name: 'Use one reset card?',
    })
    expect(within(dialog).getByText(/busy@example.com/)).toBeVisible()
    // A second tab saves Silent while this tab still has the card offer open.
    action = 'silent'
    await act(() => client.invalidateQueries({ queryKey: deviMonQueryKey }))
    await waitFor(() => expect(dialog).not.toBeInTheDocument())
    const silent = screen.getByRole('radio', {
      name: /Silent: take no action when quota runs out · busy@example/,
    })
    expect(silent).toBeChecked()
    expect(
      screen.queryByRole('button', { name: 'Confirm card use' })
    ).toBeNull()
    // This server deliberately keeps returning the old offer on refresh.
    const refresh = screen.getByRole('button', {
      name: 'Refresh busy@example.com',
    })
    await user.click(refresh)
    await waitFor(() => expect(refresh).toBeEnabled())
    expect(screen.queryByRole('alertdialog')).toBeNull()
    expect(mutations).toEqual([
      [
        '/api/devin-monitor/accounts/auto-reset',
        {
          email: 'busy@example.com',
          reset_key: 'tracked-0',
          exhaustion_action: 'use_card',
        },
      ],
      ['/api/devin-monitor/accounts/refresh', { email: 'busy@example.com' }],
      ['/api/devin-monitor/accounts/refresh', { email: 'busy@example.com' }],
    ])
    await user.click(busy)
    await waitFor(() => expect(busy).toBeChecked())
    await user.click(silent)
    await waitFor(() => expect(silent).toBeChecked())
    expect(mutations.at(-1)?.[1]).toEqual({
      email: 'busy@example.com',
      reset_key: 'tracked-0',
      exhaustion_action: 'silent',
    })
    expect(
      screen.getByRole('checkbox', {
        name: /Enable after quota resets · busy@example/,
      })
    ).toBeChecked()
    expect(
      screen.queryByRole('button', { name: 'Confirm card use' })
    ).toBeNull()
  })

  it('shows the post-reset reading returned by the mocked confirmation even if polling fails', async () => {
    const offer = {
      id: '996feb4e-ffe7-5bc3-817b-88bd8ac1c081',
      expires_at: '2026-12-01T04:03:00Z',
    }
    const exhausted = {
      ...SNAPSHOT.accounts[0],
      observed_at_us: 1_795_000_000_000_001,
      used_percent: 100,
      reset_key: 'tracked-account',
      auto_reset: true,
      reset_offer: offer,
    }
    let responseSent = false
    const changes: unknown[] = []
    api.defaults.adapter = (async (config) => {
      let data: unknown = {
        snapshot: { ...SNAPSHOT, accounts: [exhausted] },
        collecting: false,
        next_run_at: 1_795_010_000,
        configured: true,
      }
      const success = !responseSent
      if (config.method === 'post') {
        changes.push([config.url, JSON.parse(config.data)])
        responseSent = true
        data = {
          ...exhausted,
          used_percent: 21,
          observed_at_us: 1_795_000_000_000_002,
          reset_offer: null,
          reset_outcome: 'applied',
          cards: [],
        }
      }
      return {
        config,
        data: { success, data },
        headers: {},
        status: 200,
        statusText: 'OK',
      }
    }) satisfies AxiosAdapter

    renderMonitor()
    const user = userEvent.setup()
    await user.click(
      await screen.findByRole('switch', { name: 'Operation mode' })
    )
    const ready = await screen.findByRole('button', {
      name: 'Confirm card use',
    })
    await user.click(ready)
    const dialog = await screen.findByRole('alertdialog')
    await user.click(
      within(dialog).getByRole('button', { name: 'Use reset card' })
    )
    expect(await screen.findByText('79% remaining')).toBeVisible()
    expect(screen.getByText('Reset card used. Usage updated.')).toBeVisible()
    await waitFor(() => expect(dialog).not.toBeInTheDocument())
    expect(
      screen.getByRole('button', { name: 'Refresh busy@example.com' })
    ).toHaveFocus()
    expect(changes).toEqual([
      [
        '/api/devin-monitor/accounts/reset',
        { email: 'busy@example.com', offer_id: offer.id },
      ],
    ])
  })

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
    expect(
      screen.getByRole('switch', { name: 'Operation mode' })
    ).not.toBeChecked()
    expect(picker).toBeEnabled()
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
    const cardExpiry = fresh.querySelector(
      'time[datetime="2026-12-01T04:14:00Z"]'
    )
    expect(cardExpiry).toHaveTextContent(/^12-0[12] \d\d:\d\d$/)
    expect(cardExpiry?.getAttribute('title')).toMatch(/^2026-12-0[12] /)

    expect(screen.getByText('Devin disabled')).toBeInTheDocument()
    expect(screen.getByText('No Devin account')).toBeInTheDocument()
  })

  it('shows at most two cards inline and reveals the rest for the chosen account', async () => {
    const account = SNAPSHOT.accounts[0]
    const first = '2026-12-01T04:03:00Z',
      second = '2026-12-02T04:03:00Z',
      third = '2026-12-03T04:03:00Z'
    serveState({
      ...SNAPSHOT,
      accounts: [
        {
          ...account,
          cards: [first, second, third, third].map((expires_at) => ({
            expires_at,
          })),
        },
        SNAPSHOT.accounts[1],
      ],
    })
    renderMonitor()
    const more = await screen.findByRole('button', {
      name: 'Reset cards · More (2) · busy@example.com',
    })
    const row = more.closest('li')
    if (!row) throw new Error('Missing Devin account row')
    expect(within(row).getByText('×4')).toBeVisible()
    expect(row.querySelector(`time[datetime="${first}"]`)).toBeVisible()
    expect(row.querySelector(`time[datetime="${second}"]`)).toBeVisible()
    expect(row.querySelector(`time[datetime="${third}"]`)).toBeNull()
    expect(
      screen.queryByRole('button', {
        name: /Reset cards · More.*fresh@example/,
      })
    ).toBeNull()
    more.focus()
    const user = userEvent.setup()
    await user.keyboard('{Enter}')
    expect(screen.queryByRole('dialog')).toBeNull()
    await user.click(screen.getByRole('switch', { name: 'Operation mode' }))
    more.focus()
    await user.keyboard('{Enter}')
    const popup = await screen.findByRole('dialog')
    expect(popup.querySelector(`time[datetime="${third}"]`)).toBeVisible()
    expect(within(popup).getByRole('listitem')).toHaveTextContent('×2')
    await user.keyboard('{Escape}')
    await waitFor(() => expect(screen.queryByRole('dialog')).toBeNull())
  })

  // The enable/disable switch is a Devin-side toggle: it must be offered on the
  // row itself, name the account it flips and show the state it just wrote.
  it('flips the per-account Devin switch and keeps the new state on screen', async () => {
    const calls: { method?: string; url?: string; body?: unknown }[] = []
    // The server reflects the write, so a refetch must not resurrect the state
    // the account had when the last collection ran.
    let devinDisabled: boolean | null = false
    api.defaults.adapter = (async (config) => {
      if (config.method === 'get') {
        return {
          config,
          data: {
            success: true,
            data: {
              snapshot: {
                ...SNAPSHOT,
                accounts: [
                  { ...SNAPSHOT.accounts[0], devin_disabled: devinDisabled },
                  SNAPSHOT.accounts[1],
                ],
              },
              collecting: false,
              next_run_at: 1_795_010_000,
              configured: true,
            },
          },
          headers: {},
          status: 200,
          statusText: 'OK',
        }
      }
      devinDisabled = JSON.parse(config.data).disabled
      calls.push({
        method: config.method,
        url: config.url,
        body: config.data ? JSON.parse(config.data) : undefined,
      })
      return {
        config,
        data: { success: true, data: null },
        headers: {},
        status: 200,
        statusText: 'OK',
      }
    }) as AxiosAdapter

    renderMonitor()
    const user = userEvent.setup()
    await user.click(
      await screen.findByRole('switch', { name: 'Operation mode' })
    )
    const toggle = await screen.findByRole('switch', {
      name: 'Disable busy@example.com',
    })
    expect(toggle).toHaveAttribute('aria-checked', 'true')

    await user.click(toggle)

    // The switch must show the flipped state instead of snapping back to the
    // last collection, which is what made the old button look dead.
    await waitFor(() => expect(toggle).toHaveAttribute('aria-checked', 'false'))
    // Only the Devin credential is touched: flipping it must not probe the
    // Codex side, which is what an account refresh would do.
    await waitFor(() =>
      expect(calls).toEqual([
        {
          method: 'put',
          url: '/api/devin-monitor/accounts/devin-status',
          body: { email: 'busy@example.com', disabled: true },
        },
      ])
    )
  })

  it('shows effective Devin routing numbers without a default label and preserves an explicit zero', async () => {
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
    const busy = within(await rowFor('busy@example.com'))
    expect(
      busy.getByRole('button', { name: 'Priority · busy@example.com: -4' })
    ).toBeVisible()
    expect(
      busy.getByRole('button', { name: 'Weight · busy@example.com: 0' })
    ).toBeVisible()
    const orphan = within(await rowFor('orphan@example.com'))
    expect(
      orphan.getByRole('button', { name: 'Priority · orphan@example.com: 0' })
    ).toBeVisible()
    expect(
      orphan.getByRole('button', { name: 'Weight · orphan@example.com: 25' })
    ).toBeVisible()
    const defaults = within(await rowFor('defaults@example.com'))
    expect(
      defaults.getByRole('button', {
        name: 'Priority · defaults@example.com: 0',
      })
    ).toBeVisible()
    expect(
      defaults.getByRole('button', { name: 'Weight · defaults@example.com: 1' })
    ).toBeVisible()
    expect(screen.queryByText('Default')).not.toBeInTheDocument()
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

  it('queues one full collection only after confirmation and keeps the previous inventory visible', async () => {
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
    const collect = await screen.findByRole('button', { name: 'Collect now' })
    const user = userEvent.setup()
    expect(
      screen.getByRole('switch', { name: 'Operation mode' })
    ).not.toBeChecked()
    expect(collect).toBeEnabled()
    await user.click(collect)
    const dialog = await screen.findByRole('alertdialog', {
      name: 'Collect all accounts?',
    })
    expect(posted).toBe(0)
    await user.click(within(dialog).getByRole('button', { name: 'Cancel' }))
    await waitFor(() => expect(collect).toHaveFocus())
    expect(posted).toBe(0)
    await user.click(collect)
    const confirmation = await screen.findByRole('alertdialog', {
      name: 'Collect all accounts?',
    })
    await user.click(
      within(confirmation).getByRole('button', { name: 'Collect now' })
    )
    expect(
      await screen.findByRole('button', { name: 'Collecting...' })
    ).toBeDisabled()
    expect(screen.getByText('busy@example.com')).toBeVisible()
    expect(
      screen.getByText('Determined when this collection finishes')
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
