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
      if (config.url === '/api/devin-monitor/accounts/chatgpt/session') {
        data = { email: 'owned@example.com', account_id: 'acct-owned' }
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

const SESSION_JSON = JSON.stringify({
  accessToken: 'browser-access-token',
  sessionToken: 'browser-session-token',
  user: { email: 'owned@example.com' },
  account: { id: 'acct-owned' },
})

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
  it('switches to the chatgpt channel without asking for a new session when one is stored', async () => {
    const calls: Call[] = []
    serveChannels(true, calls)
    renderDialog()
    const user = userEvent.setup()

    // Base UI names each radio from its wrapping label text, so the list is
    // addressed by order: CPA first, ChatGPT session second.
    const radios = await screen.findAllByRole('radio')
    expect(radios).toHaveLength(2)
    expect(radios[0]).toHaveAttribute('aria-checked', 'true')
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

  it('offers the session page link and, without a session, sends the click to the paste box', async () => {
    const calls: Call[] = []
    serveChannels(false, calls)
    renderDialog()
    const user = userEvent.setup()

    // With CPA selected nothing of the ChatGPT branch is rendered; choosing it
    // reveals the link and the paste box without switching (no session yet).
    expect(screen.queryByLabelText('ChatGPT session JSON')).toBeNull()
    expect(
      screen.queryByRole('link', { name: 'Open the session page' })
    ).toBeNull()

    const radios = await screen.findAllByRole('radio')
    await user.click(radios[1])

    const link = await screen.findByRole('link', {
      name: 'Open the session page',
    })
    expect(link).toHaveAttribute('href', 'https://chatgpt.com/api/auth/session')
    expect(link).toHaveAttribute('target', '_blank')
    expect(await screen.findByLabelText('ChatGPT session JSON')).toBeVisible()
    expect(calls).toEqual([])
  })

  it('posts the pasted session JSON as-is', async () => {
    const calls: Call[] = []
    serveChannels(false, calls)
    renderDialog()
    const user = userEvent.setup()

    // The paste box lives on the ChatGPT branch.
    const radios = await screen.findAllByRole('radio')
    await user.click(radios[1])
    const field = await screen.findByLabelText('ChatGPT session JSON')
    await user.click(field)
    // paste, not type: userEvent reads "{...}" in type() as key descriptors.
    await user.paste(SESSION_JSON)
    await user.click(
      screen.getByRole('button', { name: 'Save ChatGPT session' })
    )

    await waitFor(() =>
      expect(calls).toEqual([
        {
          method: 'post',
          url: '/api/devin-monitor/accounts/chatgpt/session',
          body: JSON.parse(SESSION_JSON),
        },
      ])
    )
  })

  it('rejects a paste that is not JSON without calling the server', async () => {
    const calls: Call[] = []
    serveChannels(false, calls)
    renderDialog()
    const user = userEvent.setup()

    const radios = await screen.findAllByRole('radio')
    await user.click(radios[1])
    const field = await screen.findByLabelText('ChatGPT session JSON')
    await user.click(field)
    await user.paste('not json')
    await user.click(
      screen.getByRole('button', { name: 'Save ChatGPT session' })
    )

    await waitFor(() => expect(calls).toEqual([]))
  })

  it('removes the stored session only after confirmation', async () => {
    const calls: Call[] = []
    serveChannels(true, calls, 'chatgpt')
    renderDialog()
    const user = userEvent.setup()

    await user.click(
      await screen.findByRole('button', { name: 'Remove session' })
    )
    const confirm = await screen.findByRole('alertdialog', {
      name: 'Remove the ChatGPT session?',
    })
    expect(calls).toEqual([])
    await user.click(
      screen.getByRole('button', { name: 'Remove session', hidden: false })
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
