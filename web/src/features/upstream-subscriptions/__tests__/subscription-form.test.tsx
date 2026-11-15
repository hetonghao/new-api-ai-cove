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
import { act, render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { useState } from 'react'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import {
  cancelOpenDesignLogin,
  listSubscriptionChannels,
  pollOpenDesignLogin,
  saveUpstreamSubscription,
  startOpenDesignLogin,
} from '../api'
import { SubscriptionFormDialog } from '../components/subscription-form-dialog'
import type { UpstreamSubscription } from '../types'

vi.mock('../api', () => ({
  cancelOpenDesignLogin: vi.fn(),
  listSubscriptionChannels: vi.fn(),
  pollOpenDesignLogin: vi.fn(),
  saveUpstreamSubscription: vi.fn(),
  startOpenDesignLogin: vi.fn(),
}))

const subscription: UpstreamSubscription = {
  id: 4,
  name: 'My account',
  platform: 'opendesign',
  refresh_interval_minutes: 10,
  auto_refresh_disabled: false,
  last_success_at: 0,
  last_attempt_at: 0,
  next_refresh_at: 0,
  error_code: '',
  stale: false,
  quota: null,
}
const challenge = {
  flow_token: 'local-flow-token',
  user_code: 'ABCD-EFGH',
  poll_interval: 2,
  activation_url:
    'https://open-design.ai/cloud/cli/activate?deviceId=device-1&userCode=ABCD-EFGH',
  expires_at: '2027-01-01T01:00:00Z',
}

function renderForm(current: UpstreamSubscription | null = null) {
  const saved = vi.fn()
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  })
  function FormHarness() {
    const [open, setOpen] = useState(true)
    return (
      <QueryClientProvider client={queryClient}>
        <SubscriptionFormDialog
          open={open}
          subscription={current}
          onOpenChange={setOpen}
          onSaved={saved}
        />
      </QueryClientProvider>
    )
  }
  render(<FormHarness />)
  return { user: userEvent.setup(), saved }
}

beforeEach(() => {
  vi.mocked(listSubscriptionChannels).mockResolvedValue([
    { id: 8, name: 'Channel account', priority: 9, weight: 20 },
  ])
  vi.mocked(saveUpstreamSubscription).mockResolvedValue(subscription)
  vi.mocked(startOpenDesignLogin).mockResolvedValue(challenge)
  vi.mocked(cancelOpenDesignLogin).mockResolvedValue(undefined)
  vi.mocked(pollOpenDesignLogin).mockResolvedValue({ status: 'pending' })
})

describe('subscription creation', () => {
  it('can turn off automatic quota updates while retaining the interval', async () => {
    const { user, saved } = renderForm()
    const toggle = screen.getByRole('switch', {
      name: 'Automatically update quota',
    })
    expect(toggle).toBeChecked()
    toggle.focus()
    await user.keyboard(' ')
    expect(toggle).not.toBeChecked()
    expect(
      screen.getByRole('spinbutton', { name: 'Refresh interval (minutes)' })
    ).toBeDisabled()
    await user.type(screen.getByLabelText('API Key *'), 'oc_sk_test-account')
    await user.click(
      screen.getByRole('button', { name: 'Create subscription' })
    )
    await waitFor(() => expect(saved).toHaveBeenCalledOnce())
    expect(
      vi.mocked(saveUpstreamSubscription).mock.calls[0]?.[1]
    ).toMatchObject({
      auto_refresh_disabled: true,
      refresh_interval_minutes: 10,
    })
  })

  it('requires only an OpenCode API key and leaves the name optional', async () => {
    const { user, saved } = renderForm()
    await user.type(screen.getByLabelText('API Key *'), 'oc_sk_test-account')
    await user.click(
      screen.getByRole('button', { name: 'Create subscription' })
    )

    await waitFor(() => expect(saved).toHaveBeenCalledOnce())
    expect(vi.mocked(saveUpstreamSubscription).mock.calls[0]?.[1]).toEqual({
      name: '',
      account: '',
      channel_id: 0,
      platform: 'opencode',
      credential: 'oc_sk_test-account',
      refresh_interval_minutes: 10,
      auto_refresh_disabled: false,
    })
    expect(screen.queryByRole('dialog')).not.toBeInTheDocument()
  })

  it('rejects an inference key with a field error and keeps the dialog open', async () => {
    const { user } = renderForm()
    await user.type(screen.getByLabelText('API Key *'), 'sk-amr-wrong-key')
    await user.click(
      screen.getByRole('button', { name: 'Create subscription' })
    )

    expect(await screen.findByRole('alert')).toHaveTextContent(
      'Use the platform subscription credential'
    )
    expect(screen.getByLabelText('API Key *')).toHaveAttribute(
      'aria-invalid',
      'true'
    )
    expect(saveUpstreamSubscription).not.toHaveBeenCalled()
  })

  it('connects OpenDesign after official authorization without asking for a credential', async () => {
    let approve: (
      result: Awaited<ReturnType<typeof pollOpenDesignLogin>>
    ) => void = () => {
      throw new Error('Approval promise is not initialized')
    }
    const approval = new Promise<
      Awaited<ReturnType<typeof pollOpenDesignLogin>>
    >((resolve) => {
      approve = resolve
    })
    vi.mocked(pollOpenDesignLogin).mockReturnValue(approval)
    const { user, saved } = renderForm()
    await user.click(screen.getByRole('combobox', { name: 'Platform' }))
    await user.click(screen.getByRole('option', { name: 'OpenDesign Go' }))
    expect(screen.queryByLabelText('API Key *')).not.toBeInTheDocument()
    await user.click(
      screen.getByRole('button', { name: 'Sign in to OpenDesign' })
    )

    const link = await screen.findByRole('link', {
      name: 'Open OpenDesign and authorize',
    })
    expect(link).toHaveAttribute('href', challenge.activation_url)
    expect(screen.getByText('ABCD-EFGH')).toBeVisible()
    expect(screen.getByLabelText('Name (optional)')).toBeDisabled()
    expect(
      screen.getByRole('spinbutton', { name: 'Refresh interval (minutes)' })
    ).toBeDisabled()
    const platform = screen.getByRole('combobox', { name: 'Platform' })
    expect(platform).toBeDisabled()
    await user.click(platform)
    expect(platform).toHaveTextContent('OpenDesign Go')
    await act(async () => {
      approve({ status: 'approved', subscription })
    })

    await waitFor(() => expect(saved).toHaveBeenCalledOnce())
    expect(screen.queryByRole('dialog')).not.toBeInTheDocument()
    expect(saveUpstreamSubscription).not.toHaveBeenCalled()
  })

  it('cancels the pending flow when the dialog is dismissed', async () => {
    const { user, saved } = renderForm()
    await user.click(screen.getByRole('combobox', { name: 'Platform' }))
    await user.click(screen.getByRole('option', { name: 'OpenDesign Go' }))
    await user.click(
      screen.getByRole('button', { name: 'Sign in to OpenDesign' })
    )
    await screen.findByRole('link', { name: 'Open OpenDesign and authorize' })
    await user.click(screen.getByRole('button', { name: 'Cancel' }))

    await waitFor(() =>
      expect(screen.queryByRole('dialog')).not.toBeInTheDocument()
    )
    expect(vi.mocked(cancelOpenDesignLogin).mock.calls[0]?.[0]).toBe(
      'local-flow-token'
    )
    expect(saved).not.toHaveBeenCalled()
  })

  it('keeps manual OpenDesign controlKey entry available as a fallback', async () => {
    const { user, saved } = renderForm()
    await user.click(screen.getByRole('combobox', { name: 'Platform' }))
    await user.click(screen.getByRole('option', { name: 'OpenDesign Go' }))
    await user.click(
      screen.getByRole('button', { name: 'Enter a controlKey manually' })
    )
    await user.type(screen.getByLabelText('controlKey *'), 'vela_ctrl_manual')
    await user.click(
      screen.getByRole('button', { name: 'Create subscription' })
    )

    await waitFor(() => expect(saved).toHaveBeenCalledOnce())
    expect(vi.mocked(saveUpstreamSubscription).mock.calls[0]?.[1]).toEqual({
      name: '',
      account: '',
      channel_id: 0,
      platform: 'opendesign',
      credential: 'vela_ctrl_manual',
      refresh_interval_minutes: 10,
      auto_refresh_disabled: false,
    })
    expect(startOpenDesignLogin).not.toHaveBeenCalled()
  })

  it('discards a manual key when switching back to OpenDesign sign-in', async () => {
    const { user } = renderForm()
    await user.click(screen.getByRole('combobox', { name: 'Platform' }))
    await user.click(screen.getByRole('option', { name: 'OpenDesign Go' }))
    await user.click(
      screen.getByRole('button', { name: 'Enter a controlKey manually' })
    )
    await user.type(screen.getByLabelText('controlKey *'), 'sk-amr-wrong-key')
    await user.click(
      screen.getByRole('button', { name: 'Use OpenDesign sign-in instead' })
    )
    await user.click(
      screen.getByRole('button', { name: 'Sign in to OpenDesign' })
    )
    expect(
      await screen.findByRole('link', { name: 'Open OpenDesign and authorize' })
    ).toBeVisible()
    expect(saveUpstreamSubscription).not.toHaveBeenCalled()
  })

  it('saves the custom refresh interval rather than the default', async () => {
    const { user, saved } = renderForm()
    const interval = screen.getByRole('spinbutton', {
      name: 'Refresh interval (minutes)',
    })
    expect(interval).toHaveValue(10)
    await user.clear(interval)
    await user.type(interval, '25')
    await user.type(screen.getByLabelText('API Key *'), 'oc_sk_custom-cadence')
    await user.click(
      screen.getByRole('button', { name: 'Create subscription' })
    )
    await waitFor(() => expect(saved).toHaveBeenCalledOnce())
    expect(
      vi.mocked(saveUpstreamSubscription).mock.calls[0]?.[1]
        .refresh_interval_minutes
    ).toBe(25)
  })

  it('rejects refresh intervals below one minute without sending the configuration', async () => {
    const { user } = renderForm()
    const interval = screen.getByRole('spinbutton', {
      name: 'Refresh interval (minutes)',
    })
    await user.clear(interval)
    await user.type(interval, '0')
    await user.type(screen.getByLabelText('API Key *'), 'oc_sk_custom-cadence')
    await user.click(
      screen.getByRole('button', { name: 'Create subscription' })
    )
    expect(await screen.findByRole('alert')).toHaveTextContent(
      'Refresh interval must be between 1 and 1440 minutes'
    )
    expect(interval).toHaveAttribute('aria-invalid', 'true')
    expect(saveUpstreamSubscription).not.toHaveBeenCalled()
  })

  it('uses the selected channel name read-only and can switch back to an independent name', async () => {
    const { user, saved } = renderForm()
    await user.click(screen.getByRole('radio', { name: 'Bind to channel' }))
    const channel = screen.getByRole('combobox', { name: 'Channel' })
    await user.click(channel)
    await user.click(
      await screen.findByRole('option', { name: /Channel account/ })
    )
    expect(screen.queryByLabelText('Name (optional)')).not.toBeInTheDocument()
    await user.click(screen.getByRole('radio', { name: 'Independent name' }))
    const name = screen.getByLabelText('Name (optional)')
    expect(name).not.toHaveAttribute('readonly')
    expect(name).toHaveValue('Channel account')
    await user.clear(name)
    await user.type(name, 'Independent')
    await user.type(screen.getByLabelText('API Key *'), 'oc_sk_independent')
    await user.click(
      screen.getByRole('button', { name: 'Create subscription' })
    )
    await waitFor(() => expect(saved).toHaveBeenCalledOnce())
    expect(
      vi.mocked(saveUpstreamSubscription).mock.calls[0]?.[1]
    ).toMatchObject({ channel_id: 0, name: 'Independent' })
  })

  it('binds an existing subscription without replacing its credential', async () => {
    const consoleError = vi.spyOn(console, 'error')
    const { user, saved } = renderForm(subscription)
    await user.type(
      screen.getByLabelText('Subscription account (optional)'),
      'billing@example.com'
    )
    await user.click(screen.getByRole('radio', { name: 'Bind to channel' }))
    await user.click(screen.getByRole('combobox', { name: 'Channel' }))
    await user.click(
      await screen.findByRole('option', { name: /Channel account/ })
    )
    await user.click(screen.getByRole('button', { name: 'Save changes' }))
    await waitFor(() => expect(saved).toHaveBeenCalledOnce())
    expect(vi.mocked(saveUpstreamSubscription).mock.calls[0]).toEqual([
      4,
      {
        name: 'My account',
        account: 'billing@example.com',
        auto_refresh_disabled: false,
        channel_id: 8,
        platform: 'opendesign',
        credential: '',
        refresh_interval_minutes: 10,
      },
    ])
    expect(consoleError.mock.calls.flat().join(' ')).not.toContain(
      'nativeButton'
    )
  })

  it('requires an explicit channel selection and keeps an empty selector recoverable', async () => {
    vi.mocked(listSubscriptionChannels).mockResolvedValue([])
    const { user } = renderForm()
    await user.click(screen.getByRole('radio', { name: 'Bind to channel' }))
    await user.type(screen.getByLabelText('API Key *'), 'oc_sk_bound')
    await user.click(
      screen.getByRole('button', { name: 'Create subscription' })
    )
    expect(await screen.findByRole('alert')).toHaveTextContent(
      'Select a channel'
    )
    expect(saveUpstreamSubscription).not.toHaveBeenCalled()
    await user.click(screen.getByRole('radio', { name: 'Independent name' }))
    expect(screen.getByLabelText('Name (optional)')).not.toHaveAttribute(
      'readonly'
    )
  })
})
