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
  channel: 'cpa' | 'chatgpt' = 'cpa',
  proxyUrl = ''
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
          proxy_url: proxyUrl,
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

    // Selecting a channel only stages it; nothing is written until save.
    expect(calls).toEqual([])
    await user.click(screen.getByRole('button', { name: 'Save channel' }))

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

  // Regression: clicking a channel used to switch it immediately, so closing
  // the dialog without saving still changed the account.
  it('does not switch the channel when the dialog closes without saving', async () => {
    const calls: Call[] = []
    serveChannels(true, calls, 'chatgpt')
    const { rerender } = renderDialog()
    const user = userEvent.setup()

    // Wait for the stored channel to arrive before asserting the selection.
    await waitFor(() =>
      expect(screen.getAllByRole('radio')[1]).toHaveAttribute(
        'aria-checked',
        'true'
      )
    )
    await user.click(screen.getAllByRole('radio')[0])
    expect(calls).toEqual([])

    // Close the dialog the way the operator would.
    await user.click(screen.getByRole('button', { name: 'Close' }))
    rerender(
      <QueryClientProvider
        client={
          new QueryClient({ defaultOptions: { queries: { retry: false } } })
        }
      >
        <Harness />
      </QueryClientProvider>
    )
    expect(calls).toEqual([])
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
        // Saving reads the account immediately so the row can move up.
        {
          method: 'post',
          url: '/api/devin-monitor/accounts/refresh',
          body: { email: 'owned@example.com' },
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

  // CPA reads through its own proxy settings, so offering an account proxy
  // while CPA is selected promised something the gateway never applies.
  it('hides the proxy field on the CPA channel and shows it on the ChatGPT channel', async () => {
    const calls: Call[] = []
    serveChannels(true, calls)
    renderDialog()
    const user = userEvent.setup()

    await screen.findAllByRole('radio')
    expect(screen.queryByLabelText('Proxy address')).toBeNull()

    await user.click(screen.getAllByRole('radio')[1])
    expect(await screen.findByLabelText('Proxy address')).toBeVisible()
  })

  it('saves the proxy address for the account', async () => {
    const calls: Call[] = []
    serveChannels(true, calls, 'chatgpt')
    renderDialog()
    const user = userEvent.setup()

    const field = await screen.findByLabelText('Proxy address')
    await user.clear(field)
    await user.type(field, 'socks5://user:pass@127.0.0.1:1080')
    await user.click(screen.getByRole('button', { name: 'Save proxy' }))

    await waitFor(() =>
      expect(calls).toEqual([
        {
          method: 'put',
          url: '/api/devin-monitor/accounts/proxy',
          body: {
            email: 'owned@example.com',
            proxy_url: 'socks5://user:pass@127.0.0.1:1080',
          },
        },
      ])
    )
  })
})
