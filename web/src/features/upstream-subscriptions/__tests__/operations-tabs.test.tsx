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
import userEvent from '@testing-library/user-event'
import type { AxiosAdapter } from 'axios'
import { afterEach, describe, expect, it } from 'vitest'

import { api } from '@/lib/api'

import { UpstreamSubscriptions } from '..'

const originalAdapter = api.defaults.adapter

afterEach(() => {
  api.defaults.adapter = originalAdapter
})

const EMPTY_SNAPSHOT = {
  collected_at: 1_795_000_000,
  duration_ms: 0,
  interval_minutes: 180,
  accounts: [],
  unpaired_devin: [],
  errors: [],
}

function serveBothPages() {
  api.defaults.adapter = (async (config) => {
    if (config.url?.includes('/devin-monitor')) {
      return {
        config,
        data: {
          success: true,
          data: {
            snapshot: EMPTY_SNAPSHOT,
            collecting: false,
            next_run_at: 0,
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
      data: { success: true, data: [] },
      headers: {},
      status: 200,
      statusText: 'OK',
    }
  }) as AxiosAdapter
}

function renderPage() {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  })
  return render(
    <QueryClientProvider client={client}>
      <UpstreamSubscriptions />
    </QueryClientProvider>
  )
}

describe('subscription operations tabs', () => {
  it('keeps the existing Coding Plan content as the default tab', async () => {
    serveBothPages()
    renderPage()

    expect(
      await screen.findByRole('tab', { name: 'Coding Plan' })
    ).toHaveAttribute('aria-selected', 'true')
    // The subscription list still owns its own toolbar and empty state.
    expect(
      (await screen.findAllByRole('button', { name: 'Create subscription' }))
        .length
    ).toBeGreaterThan(0)
  })

  it('switches to the Devin monitor without unmounting the switcher', async () => {
    serveBothPages()
    renderPage()

    await userEvent.click(await screen.findByRole('tab', { name: 'Devin' }))

    expect(await screen.findByText('No Codex accounts collected yet')).toBeInTheDocument()
    expect(screen.getByRole('tab', { name: 'Coding Plan' })).toBeInTheDocument()

    await userEvent.click(screen.getByRole('tab', { name: 'Coding Plan' }))
    expect(
      (await screen.findAllByRole('button', { name: 'Create subscription' }))
        .length
    ).toBeGreaterThan(0)
    expect(screen.queryByText('No Codex accounts collected yet')).toBeNull()
  })
})