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
import { afterEach, describe, expect, it } from 'vitest'

import { api } from '@/lib/api'

import { DeviMonMonitor } from '../components/devi-mon-monitor'

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

function renderMonitor() {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  })
  return render(
    <QueryClientProvider client={client}>
      <DeviMonMonitor />
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
  it('shows weekly allowance, reset cards and the Devin enable state per account', async () => {
    serveState(SNAPSHOT)
    renderMonitor()

    expect(await screen.findByText('busy@example.com')).toBeInTheDocument()
    expect(screen.getByText('fresh@example.com')).toBeInTheDocument()

    // Remaining allowance follows the Coding Plan rule: the bar is what is left.
    expect(screen.getByText('1% remaining')).toBeInTheDocument()
    expect(screen.getByText('96% remaining')).toBeInTheDocument()

    expect(screen.getByText(/x2 ·/)).toBeInTheDocument()
    expect(screen.getByText('x1 ·', { exact: false })).toBeInTheDocument()

    expect(screen.getByText('Devin disabled')).toBeInTheDocument()
    expect(screen.getByText('No Devin account')).toBeInTheDocument()
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

  it('collects on demand and renders the returned snapshot', async () => {
    let posted = 0
    api.defaults.adapter = (async (config) => {
      if (config.method === 'post') {
        posted += 1
        return {
          config,
          data: {
            success: true,
            data: {
              snapshot: SNAPSHOT,
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
      return {
        config,
        data: {
          success: true,
          data: {
            snapshot: null,
            collecting: false,
            next_run_at: 0,
            configured: true,
          },
        },
        headers: {},
        status: 200,
        statusText: 'OK',
      }
    }) as AxiosAdapter
    renderMonitor()

    // Both the header and the empty state offer "Collect now"; either works.
    const triggers = await screen.findAllByRole('button', {
      name: 'Collect now',
    })
    await userEvent.click(triggers[0])
    await waitFor(() => expect(posted).toBe(1))
    expect(await screen.findByText('busy@example.com')).toBeInTheDocument()
  })
})
