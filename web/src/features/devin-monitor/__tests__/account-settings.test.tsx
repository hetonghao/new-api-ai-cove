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
import { useState } from 'react'
import { afterEach, describe, expect, it } from 'vitest'

import { api } from '@/lib/api'

import { DeviMonAccountSettings } from '../components/devi-mon-account-settings'

const originalAdapter = api.defaults.adapter

afterEach(() => {
  api.defaults.adapter = originalAdapter
})

type Call = { method?: string; url?: string; body?: unknown }

// Serve the channel list plus the mutation replies, recording every write so
// the test can assert which endpoint the switch actually called.
function serveChannels(
  signedIn: boolean,
  calls: Call[],
  channel: 'cpa' | 'chatgpt' = 'cpa'
) {
  api.defaults.adapter = (async (config) => {
    let data: unknown = null
    if (config.method === 'get') {
      data = [
        {
          email: 'owned@example.com',
          account_id: 'acct-owned',
          channel,
          signed_in: signedIn,
          expires_at: 1_795_768_756,
        },
      ]
    } else {
      calls.push({
        method: config.method,
        url: config.url,
        body: config.data ? JSON.parse(config.data) : undefined,
      })
      if (config.url === '/api/devin-monitor/accounts/chatgpt/login') {
        data = {
          device_auth_id: 'deviceauth_test',
          user_code: 'TEST-CODE',
          verification_url: 'https://auth.openai.com/codex/device',
          interval: 3600,
          expires_at: '2026-12-01T00:00:00Z',
        }
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
}

function Harness() {
  const [open, setOpen] = useState(true)
  return (
    <DeviMonAccountSettings
      email='owned@example.com'
      open={open}
      onOpenChange={setOpen}
    />
  )
}

function renderDialog() {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  })
  return render(
    <QueryClientProvider client={client}>
      <Harness />
    </QueryClientProvider>
  )
}

describe('devin account subscription channel', () => {
  it('switches to the chatgpt channel without asking for a new sign-in when one is stored', async () => {
    const calls: Call[] = []
    serveChannels(true, calls)
    renderDialog()
    const user = userEvent.setup()

    // Base UI names each radio from its wrapping label text, so the list is
    // addressed by order: CPA first, ChatGPT sign-in second.
    const radios = await screen.findAllByRole('radio')
    expect(radios).toHaveLength(2)
    expect(radios[0]).toHaveAttribute('aria-checked', 'true')
    expect(radios[1]).toHaveAttribute('aria-checked', 'false')
    await user.click(radios[1])

    await waitFor(() =>
      expect(calls).toEqual([
        {
          method: 'put',
          url: '/api/devin-monitor/accounts/channel',
          body: { email: 'owned@example.com', channel: 'chatgpt' },
        },
        {
          method: 'post',
          url: '/api/devin-monitor/accounts/refresh',
          body: { email: 'owned@example.com' },
        },
      ])
    )
  })

  it('starts a device sign-in instead of switching when no sign-in is stored', async () => {
    const calls: Call[] = []
    serveChannels(false, calls)
    renderDialog()
    const user = userEvent.setup()

    const radios = await screen.findAllByRole('radio')
    await user.click(radios[1])

    expect(await screen.findByText('TEST-CODE', { exact: false })).toBeVisible()
    expect(screen.getByRole('link')).toHaveAttribute(
      'href',
      'https://auth.openai.com/codex/device'
    )
    expect(calls).toEqual([
      { method: 'post', url: '/api/devin-monitor/accounts/chatgpt/login' },
    ])
  })

  it('removes the stored sign-in only after confirmation', async () => {
    const calls: Call[] = []
    serveChannels(true, calls, 'chatgpt')
    renderDialog()
    const user = userEvent.setup()

    await user.click(
      await screen.findByRole('button', { name: 'Remove sign-in' })
    )
    const confirm = await screen.findByRole('alertdialog', {
      name: 'Remove the ChatGPT sign-in?',
    })
    expect(calls).toEqual([])
    await user.click(
      screen.getByRole('button', { name: 'Remove sign-in', hidden: false })
    )
    await waitFor(() =>
      expect(calls).toEqual([
        {
          method: 'delete',
          url: '/api/devin-monitor/accounts/chatgpt',
          body: { email: 'owned@example.com' },
        },
        {
          method: 'post',
          url: '/api/devin-monitor/accounts/refresh',
          body: { email: 'owned@example.com' },
        },
      ])
    )
    expect(confirm).not.toBeNull()
  })
})
