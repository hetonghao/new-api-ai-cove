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
import {
  createMemoryHistory,
  createRootRoute,
  createRouter,
  RouterContextProvider,
} from '@tanstack/react-router'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { useState } from 'react'
import { useForm } from 'react-hook-form'
import { beforeEach, describe, expect, test, vi } from 'vitest'

import { Form } from '@/components/ui/form'
import { api } from '@/lib/api'

import {
  CHANNEL_TYPE_NEW_API,
  CHANNEL_TYPE_SUB2API,
  CHANNEL_TYPE_TASK_PLUGIN,
} from '../../constants'
import { CHANNEL_TYPE_ADVANCED_CUSTOM } from '../../lib/advanced-custom'
import {
  CHANNEL_FORM_DEFAULT_VALUES,
  buildSettingJSON,
  transformChannelToFormDefaults,
  transformFormDataToCreatePayload,
  transformFormDataToUpdatePayload,
  type ChannelFormValues,
} from '../../lib/channel-form'
import { channelSchema } from '../../types'
import { ChannelQuickOptions } from '../channel-quick-options'
import type { PassthroughKind } from '../dialogs/passthrough-warning-dialog'

type HarnessProps = {
  channelType?: number
  layout?: 'stacked' | 'inline'
  values?: Partial<ChannelFormValues>
  sensitiveLocked?: boolean
  confirm: (kind: PassthroughKind) => Promise<boolean>
  onSave?: (values: ChannelFormValues) => void
}

function QuickOptionsHarness(props: HarnessProps) {
  const channelType = props.channelType ?? 1
  const [client] = useState(
    () => new QueryClient({ defaultOptions: { queries: { retry: false } } })
  )
  const [router] = useState(() =>
    createRouter({
      routeTree: createRootRoute(),
      history: createMemoryHistory({ initialEntries: ['/'] }),
    })
  )
  const form = useForm<ChannelFormValues>({
    defaultValues: {
      ...CHANNEL_FORM_DEFAULT_VALUES,
      type: channelType,
      ...props.values,
    },
  })
  return (
    <QueryClientProvider client={client}>
      <RouterContextProvider router={router}>
        <Form {...form}>
          <form
            onSubmit={form.handleSubmit((values) => props.onSave?.(values))}
          >
            <ChannelQuickOptions
              channelType={channelType}
              sensitiveLocked={props.sensitiveLocked ?? false}
              disabled={false}
              layout={props.layout}
              confirmEnablePassthrough={props.confirm}
            />
            <button type='submit'>Save</button>
          </form>
        </Form>
      </RouterContextProvider>
    </QueryClientProvider>
  )
}

const HEADERS_SWITCH = { name: 'Pass Through Request Headers' }
const BODY_SWITCH = { name: 'Pass Through Body' }

describe('Codex access quick option', () => {
  // The stacked layout renders one radio group; the compact inline strip uses
  // a select. Both must persist the same three-state value.
  const pickPolicy = async (
    user: ReturnType<typeof userEvent.setup>,
    layout: 'stacked' | 'inline',
    label: string
  ) => {
    if (layout === 'stacked') {
      await user.click(screen.getByRole('radio', { name: label }))
      return
    }
    await user.selectOptions(
      screen.getByRole('combobox', { name: 'Codex access' }),
      label
    )
  }

  const expectPolicy = (
    layout: 'stacked' | 'inline',
    label: string,
    checked: boolean
  ) => {
    const control =
      layout === 'stacked'
        ? screen.getByRole('radio', { name: label })
        : screen.getByRole('combobox', { name: 'Codex access' })
    if (checked) {
      expect(control).toBeChecked()
      return
    }
    expect(control).not.toBeChecked()
  }

  test.each(['stacked', 'inline'] as const)(
    'defaults to no restriction in %s layout',
    (layout) => {
      render(
        <QuickOptionsHarness
          layout={layout}
          confirm={() => Promise.resolve(true)}
        />
      )
      if (layout === 'stacked') {
        expect(
          screen.getByRole('radio', { name: 'No restriction' })
        ).toBeChecked()
        return
      }
      expect(
        screen.getByRole('combobox', { name: 'Codex access' })
      ).toHaveValue('')
    }
  )

  test.each([
    ['Codex only', 'only'],
    ['Block Codex', 'forbidden'],
  ])('saves %s as codex_access=%s and restores it', async (label, expected) => {
    const user = userEvent.setup()
    const saved: ChannelFormValues[] = []
    render(
      <QuickOptionsHarness
        confirm={() => Promise.resolve(true)}
        onSave={(values) => saved.push(values)}
      />
    )
    await pickPolicy(user, 'stacked', label)
    await user.click(screen.getByRole('button', { name: 'Save' }))
    await waitFor(() => expect(saved).toHaveLength(1))
    const setting = buildSettingJSON(saved[0])
    expect(JSON.parse(setting).codex_access).toBe(expected)
    const channel = channelSchema.parse({
      id: 1,
      type: 1,
      key: '',
      status: 1,
      name: 'OpenAI API',
      created_time: 0,
      test_time: 0,
      response_time: 0,
      balance_updated_time: 0,
      setting,
    })
    expect(transformChannelToFormDefaults(channel).codex_access).toBe(expected)
  })

  test('saves the inline select as codex_access', async () => {
    const user = userEvent.setup()
    const saved: ChannelFormValues[] = []
    render(
      <QuickOptionsHarness
        layout='inline'
        confirm={() => Promise.resolve(true)}
        onSave={(values) => saved.push(values)}
      />
    )
    await pickPolicy(user, 'inline', 'Block Codex')
    await user.click(screen.getByRole('button', { name: 'Save' }))
    await waitFor(() => expect(saved).toHaveLength(1))
    expect(JSON.parse(buildSettingJSON(saved[0])).codex_access).toBe(
      'forbidden'
    )
  })

  test('omits codex_access when no restriction is chosen', async () => {
    const user = userEvent.setup()
    const saved: ChannelFormValues[] = []
    render(
      <QuickOptionsHarness
        values={{ codex_access: 'only' }}
        confirm={() => Promise.resolve(true)}
        onSave={(values) => saved.push(values)}
      />
    )
    expect(screen.getByRole('radio', { name: 'Codex only' })).toBeChecked()
    await user.click(screen.getByRole('radio', { name: 'No restriction' }))
    await user.click(screen.getByRole('button', { name: 'Save' }))
    await waitFor(() => expect(saved).toHaveLength(1))
    expect(JSON.parse(buildSettingJSON(saved[0])).codex_access).toBeUndefined()
  })

  test('selects exactly one policy at a time', async () => {
    const user = userEvent.setup()
    render(
      <QuickOptionsHarness
        values={{ codex_access: 'only' }}
        confirm={() => Promise.resolve(true)}
      />
    )
    await user.click(screen.getByRole('radio', { name: 'Block Codex' }))
    expectPolicy('stacked', 'Block Codex', true)
    expectPolicy('stacked', 'Codex only', false)
  })

  test('prevents changes when sensitive settings are locked', async () => {
    const user = userEvent.setup()
    render(
      <QuickOptionsHarness
        sensitiveLocked
        values={{ codex_access: 'forbidden' }}
        confirm={() => Promise.resolve(true)}
      />
    )
    // Base UI radios expose the disabled state through aria-disabled.
    const restricted = screen.getByRole('radio', { name: 'No restriction' })
    expect(restricted).toHaveAttribute('aria-disabled', 'true')
    await user.click(restricted)
    expect(screen.getByRole('radio', { name: 'Block Codex' })).toBeChecked()
  })
})

beforeEach(() => {
  vi.spyOn(api, 'get').mockResolvedValue({
    data: { success: true, data: {} },
  })
})

describe('header passthrough quick option', () => {
  test('is checked when the header override contains the wildcard rule', () => {
    render(
      <QuickOptionsHarness
        values={{ header_override: '{"*": true, "X-Foo": "bar"}' }}
        confirm={() => Promise.resolve(true)}
      />
    )
    expect(screen.getByRole('switch', HEADERS_SWITCH)).toBeChecked()
  })

  test('is unchecked when the header override has no wildcard rule', () => {
    render(
      <QuickOptionsHarness
        values={{ header_override: '{"X-Foo": "bar"}' }}
        confirm={() => Promise.resolve(true)}
      />
    )
    expect(screen.getByRole('switch', HEADERS_SWITCH)).not.toBeChecked()
  })

  test('enabling asks for confirmation and merges the wildcard rule into existing overrides on approval', async () => {
    const user = userEvent.setup()
    const confirm = vi.fn(() => Promise.resolve(true))
    const saved: ChannelFormValues[] = []
    render(
      <QuickOptionsHarness
        values={{ header_override: '{"X-Foo": "bar"}' }}
        confirm={confirm}
        onSave={(values) => saved.push(values)}
      />
    )
    const toggle = screen.getByRole('switch', HEADERS_SWITCH)
    await user.click(toggle)
    expect(confirm).toHaveBeenCalledWith('headers')
    await waitFor(() => expect(toggle).toBeChecked())
    await user.click(screen.getByRole('button', { name: 'Save' }))
    await waitFor(() => expect(saved).toHaveLength(1))
    expect(JSON.parse(saved[0].header_override ?? '')).toEqual({
      '*': true,
      'X-Foo': 'bar',
    })
  })

  test('declining the confirmation leaves the header override untouched', async () => {
    const user = userEvent.setup()
    const saved: ChannelFormValues[] = []
    render(
      <QuickOptionsHarness
        values={{ header_override: '' }}
        confirm={() => Promise.resolve(false)}
        onSave={(values) => saved.push(values)}
      />
    )
    const toggle = screen.getByRole('switch', HEADERS_SWITCH)
    await user.click(toggle)
    await user.click(screen.getByRole('button', { name: 'Save' }))
    await waitFor(() => expect(saved).toHaveLength(1))
    expect(toggle).not.toBeChecked()
    expect(saved[0].header_override).toBe('')
  })

  test('disabling removes the wildcard rule immediately without asking', async () => {
    const user = userEvent.setup()
    const confirm = vi.fn(() => Promise.resolve(true))
    const saved: ChannelFormValues[] = []
    render(
      <QuickOptionsHarness
        values={{ header_override: '{"*": true}' }}
        confirm={confirm}
        onSave={(values) => saved.push(values)}
      />
    )
    const toggle = screen.getByRole('switch', HEADERS_SWITCH)
    expect(toggle).toBeChecked()
    await user.click(toggle)
    expect(confirm).not.toHaveBeenCalled()
    expect(toggle).not.toBeChecked()
    await user.click(screen.getByRole('button', { name: 'Save' }))
    await waitFor(() => expect(saved).toHaveLength(1))
    expect(saved[0].header_override).toBe('')
  })

  test('invalid header override JSON disables the switch and explains why', async () => {
    const user = userEvent.setup()
    const confirm = vi.fn(() => Promise.resolve(true))
    render(
      <QuickOptionsHarness
        values={{ header_override: '{"*": tru' }}
        confirm={confirm}
      />
    )
    const toggle = screen.getByRole('switch', HEADERS_SWITCH)
    expect(toggle).toHaveAttribute('aria-disabled', 'true')
    expect(toggle).not.toBeChecked()
    expect(
      screen.getByText('Fix the Request Header Override JSON first')
    ).toBeVisible()
    await user.click(toggle)
    expect(confirm).not.toHaveBeenCalled()
  })

  test('inline layout surfaces the warning as the toggle tooltip', () => {
    render(
      <QuickOptionsHarness
        layout='inline'
        confirm={() => Promise.resolve(true)}
      />
    )
    const toggle = screen.getByRole('switch', HEADERS_SWITCH)
    expect(toggle.closest('[title]')).toHaveAttribute(
      'title',
      'Enable when the upstream needs client information from Codex or Claude Code headers; may expose client information'
    )
  })
})

describe('body passthrough quick option', () => {
  test.each([
    { channelType: 1, enabled: true },
    { channelType: CHANNEL_TYPE_ADVANCED_CUSTOM, enabled: true },
    { channelType: CHANNEL_TYPE_ADVANCED_CUSTOM, enabled: false },
    { channelType: CHANNEL_TYPE_ADVANCED_CUSTOM, enabled: undefined },
  ])(
    'channel type $channelType preserves body passthrough $enabled through create, update and reload',
    ({ channelType, enabled }) => {
      const channel = channelSchema.parse({
        id: 42,
        name: 'Body passthrough channel',
        type: channelType,
        key: '',
        status: 1,
        created_time: 0,
        test_time: 0,
        response_time: 0,
        balance_updated_time: 0,
        setting: JSON.stringify({ pass_through_body_enabled: enabled }),
      })
      const values = transformChannelToFormDefaults(channel)
      const payloads = [
        transformFormDataToCreatePayload(values).channel,
        transformFormDataToUpdatePayload(values, channel.id),
      ]
      for (const payload of payloads) {
        expect(
          JSON.parse(payload.setting ?? '{}').pass_through_body_enabled
        ).toBe(enabled === true)
        expect(
          transformChannelToFormDefaults({ ...channel, ...payload })
            .pass_through_body_enabled
        ).toBe(enabled === true)
      }
    }
  )

  test('enabling asks for confirmation and disabling does not', async () => {
    const user = userEvent.setup()
    const confirm = vi.fn(() => Promise.resolve(true))
    render(<QuickOptionsHarness confirm={confirm} />)
    const toggle = screen.getByRole('switch', BODY_SWITCH)
    expect(toggle).not.toBeChecked()
    await user.click(toggle)
    expect(confirm).toHaveBeenCalledWith('body')
    await waitFor(() => expect(toggle).toBeChecked())
    await user.click(toggle)
    expect(toggle).not.toBeChecked()
    expect(confirm).toHaveBeenCalledTimes(1)
  })

  test('declining the confirmation keeps body passthrough off', async () => {
    const user = userEvent.setup()
    const saved: ChannelFormValues[] = []
    render(
      <QuickOptionsHarness
        confirm={() => Promise.resolve(false)}
        onSave={(values) => saved.push(values)}
      />
    )
    const toggle = screen.getByRole('switch', BODY_SWITCH)
    await user.click(toggle)
    await user.click(screen.getByRole('button', { name: 'Save' }))
    await waitFor(() => expect(saved).toHaveLength(1))
    expect(toggle).not.toBeChecked()
    expect(saved[0].pass_through_body_enabled).toBe(false)
  })

  test('shows the consequence warning as the switch description', () => {
    render(<QuickOptionsHarness confirm={() => Promise.resolve(true)} />)
    expect(
      screen.getByText(
        'Preserve upstream-specific fields when API formats match; bypasses model redirect, parameter override and format conversion'
      )
    ).toBeVisible()
  })
})

test('task plugin channels hide both passthrough switches', () => {
  render(
    <QuickOptionsHarness
      channelType={CHANNEL_TYPE_TASK_PLUGIN}
      confirm={() => Promise.resolve(true)}
    />
  )
  expect(screen.queryByRole('switch', BODY_SWITCH)).not.toBeInTheDocument()
  expect(screen.queryByRole('switch', HEADERS_SWITCH)).not.toBeInTheDocument()
  expect(
    screen.getByRole('switch', { name: 'Auto-disable channel' })
  ).toBeInTheDocument()
})

describe('responses websocket quick option', () => {
  const WEBSOCKET_SWITCH = { name: 'Responses WebSocket' }

  test.each([
    1,
    57,
    CHANNEL_TYPE_ADVANCED_CUSTOM,
    CHANNEL_TYPE_SUB2API,
    CHANNEL_TYPE_NEW_API,
  ])('is offered for channel type %s', (channelType) => {
    render(
      <QuickOptionsHarness
        channelType={channelType}
        confirm={() => Promise.resolve(true)}
      />
    )
    expect(screen.getByRole('switch', WEBSOCKET_SWITCH)).toBeInTheDocument()
  })

  test.each([14, CHANNEL_TYPE_TASK_PLUGIN])(
    'is hidden for channel type %s',
    (channelType) => {
      render(
        <QuickOptionsHarness
          channelType={channelType}
          confirm={() => Promise.resolve(true)}
        />
      )
      expect(
        screen.queryByRole('switch', WEBSOCKET_SWITCH)
      ).not.toBeInTheDocument()
    }
  )
})
