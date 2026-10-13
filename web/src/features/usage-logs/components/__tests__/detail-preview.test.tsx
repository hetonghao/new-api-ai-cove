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
  flexRender,
  getCoreRowModel,
  useReactTable,
} from '@tanstack/react-table'
import { act, fireEvent, render, screen, within } from '@testing-library/react'
import { createInstance } from 'i18next'
import { I18nextProvider } from 'react-i18next'
import { afterAll, afterEach, beforeEach, expect, test, vi } from 'vitest'

import en from '@/i18n/locales/en.json'
import zh from '@/i18n/locales/zh.json'
import {
  DEFAULT_CURRENCY_CONFIG,
  useSystemConfigStore,
} from '@/stores/system-config-store'

import type { UsageLog } from '../../data/schema'
import type { LogOtherData } from '../../types'
import { useCommonLogsColumns } from '../columns/common-logs-columns'
import { UsageLogsProvider } from '../usage-logs-provider'

vi.mock('@lobehub/icons', () => ({}))
vi.hoisted(() => {
  vi.stubGlobal('localStorage', {
    getItem: () => null,
    setItem: () => undefined,
    removeItem: () => undefined,
  })
})
afterAll(() => vi.unstubAllGlobals())

function makeLog(
  other: LogOtherData,
  overrides: Partial<UsageLog> = {}
): UsageLog {
  return {
    id: 1,
    user_id: 1,
    created_at: 1,
    type: 2,
    content: '',
    username: 'user',
    token_name: 'token',
    model_name: 'wan2.5-i2v-preview',
    quota: 5000,
    prompt_tokens: 0,
    completion_tokens: 0,
    use_time: 0,
    is_stream: false,
    channel: 1,
    channel_name: '',
    token_id: 1,
    group: 'default',
    ip: '',
    other: JSON.stringify(other),
    request_id: 'req-1',
    upstream_request_id: '',
    ...overrides,
  }
}

function DetailPreview(props: {
  other: LogOtherData
  isAdmin: boolean
  log?: Partial<UsageLog>
}) {
  const table = useReactTable({
    data: [makeLog(props.other, props.log)],
    columns: useCommonLogsColumns(props.isAdmin, false),
    getCoreRowModel: getCoreRowModel(),
  })
  const cell = table
    .getRowModel()
    .rows[0].getAllCells()
    .find((item) => item.column.id === 'content')
  if (!cell) throw new Error('The log must have a content column')
  return flexRender(cell.column.columnDef.cell, cell.getContext())
}
const plugin = {
  key: 'incho',
  name: 'Incho',
  version: '1.0.1',
  author: { name: 'Plugin maintainer' },
}
const previousConfig = useSystemConfigStore.getState().config
let client: QueryClient
const i18n = createInstance()
beforeEach(async () => {
  await i18n.init({
    lng: 'en',
    resources: { en },
    interpolation: { escapeValue: false },
  })
  useSystemConfigStore
    .getState()
    .setConfig({ currency: { ...DEFAULT_CURRENCY_CONFIG } })
  client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  client.setQueryData(['status'], {}, { updatedAt: Date.now() + 60_000 })
  client.setQueryData(
    ['pricing'],
    { data: [], vendors: [] },
    { updatedAt: Date.now() + 60_000 }
  )
})
afterEach(() => {
  client.clear()
  useSystemConfigStore.getState().setConfig(previousConfig)
})
function renderPreview(
  other: LogOtherData,
  isAdmin = true,
  log?: Partial<UsageLog>
) {
  render(
    <I18nextProvider i18n={i18n}>
      <QueryClientProvider client={client}>
        <UsageLogsProvider search={{}} navigateSearch={() => undefined}>
          <DetailPreview other={other} isAdmin={isAdmin} log={log} />
        </UsageLogsProvider>
      </QueryClientProvider>
    </I18nextProvider>
  )
  return screen.getByRole('button', { name: /./ })
}

test('keeps log details open when the parent refreshes with unchanged data', async () => {
  const other = { model_price: 0.25 }
  const wrapper = ({ children }: { children: React.ReactNode }) => (
    <I18nextProvider i18n={i18n}>
      <QueryClientProvider client={client}>
        <UsageLogsProvider search={{}} navigateSearch={() => undefined}>
          {children}
        </UsageLogsProvider>
      </QueryClientProvider>
    </I18nextProvider>
  )
  const { rerender } = render(<DetailPreview other={other} isAdmin />, {
    wrapper,
  })
  fireEvent.click(screen.getByRole('button', { name: 'Per-call · $0.25' }))
  expect(await screen.findByRole('dialog')).toBeVisible()
  rerender(<DetailPreview other={other} isAdmin />)
  expect(screen.getByRole('dialog')).toBeVisible()
})

test.each([
  {
    name: 'fixed expression zero price',
    other: {
      billing_mode: 'tiered_expr',
      billing_unit: 'request' as const,
      fixed_price: 0,
      matched_tier: 'free',
      expr_b64: btoa('tier("free", fixed(0))'),
    },
    expected: 'free · Per-call $0/request',
  },
  {
    name: 'fixed expression trace outside the display grammar',
    other: {
      billing_mode: 'tiered_expr',
      billing_unit: 'request' as const,
      fixed_price: 0.01,
      matched_tier: 'priority',
      expr_b64: btoa(
        'param("fast") == true ? tier("priority", fixed(0.01)) : tier("tokens", p * 2)'
      ),
    },
    expected: 'priority · Per-call $0.01/request',
  },
  {
    name: 'parsed token expression',
    other: {
      billing_mode: 'tiered_expr',
      matched_tier: 'standard',
      expr_b64: btoa('tier("standard", p * 2 + c * 10)'),
    },
    expected: 'standard · $2 / $10/M',
  },
  {
    name: 'per-call',
    other: { model_price: 0.25 },
    expected: 'Per-call · $0.25',
  },
  {
    name: 'standard',
    other: { model_ratio: 1, completion_ratio: 2 },
    expected: 'Standard · $2 / $4/M',
  },
  {
    name: 'zero price fallback',
    other: { model_price: 0, group_ratio: 1 },
    expected: 'Group Ratio 1x',
  },
  { name: 'missing price fallback', other: {}, expected: '—' },
])('$name stays visible without a plugin counter', ({ other, expected }) => {
  const preview = renderPreview({
    ...other,
    admin_info: { task_plugin: plugin },
  })
  expect(preview.textContent).toBe(expected)
})

test.each([
  {
    tier: 'standard',
    billingUnit: 'token' as const,
    fixedPrice: undefined,
    expected: 'Dynamic Pricing · standard',
  },
  {
    tier: 'long_context',
    billingUnit: 'token' as const,
    fixedPrice: undefined,
    expected: 'Dynamic Pricing · long_context',
  },
  {
    tier: 'free_small_output',
    billingUnit: 'request' as const,
    fixedPrice: 0,
    expected: 'free_small_output · Per-call $0/request',
  },
])(
  'nested expression preserves the recorded $tier settlement in the list',
  ({ tier, billingUnit, fixedPrice, expected }) => {
    const expression = `len <= 272000
      ? (c > 5
        ? tier("standard", p * 2 + c * 10 + cr * 0.1 + cc * 2.5)
        : tier("free_small_output", fixed(0)))
      : (c > 5
        ? tier("long_context", p * 5 + c * 22.5 + cr * 0.5)
        : tier("free_small_output", fixed(0)))`
    const preview = renderPreview({
      billing_mode: 'tiered_expr',
      billing_unit: billingUnit,
      fixed_price: fixedPrice,
      matched_tier: tier,
      expr_b64: btoa(expression),
    })
    expect(preview.textContent).toBe(expected)
  }
)

test.each([
  { name: 'missing expression', expr: undefined },
  { name: 'invalid encoded expression', expr: '%' },
  {
    name: 'tier absent from the parsed expression',
    expr: btoa('tier("different", p * 99 + c * 99)'),
  },
])(
  'token pricing with $name preserves the saved tier without guessing prices',
  ({ expr }) => {
    const preview = renderPreview({
      billing_mode: 'tiered_expr',
      matched_tier: 'recorded',
      expr_b64: expr,
    })
    expect(preview.textContent).toBe('Dynamic Pricing · recorded')
  }
)

test.each([false, true])(
  'dynamic pricing without a saved tier uses a localized neutral fallback for task=%s',
  async (isTask) => {
    i18n.addResourceBundle('zh', 'translation', zh.translation)
    const preview = renderPreview({
      is_task: isTask,
      billing_mode: 'tiered_expr',
      expr_b64: btoa(
        isTask
          ? 'tier("base", u("clips") * 0.25)'
          : 'tier("base", p * 2 + c * 10)'
      ),
    })
    expect(preview.textContent).toBe(
      'Dynamic Pricing · Special billing expression'
    )
    await act(() => i18n.changeLanguage('zh'))
    expect(
      screen.getByRole('button', { name: '动态计费 · 特殊计费表达式' })
    ).toBeVisible()
  }
)

test('top-up content_parts render in the viewer language', async () => {
  i18n.addResourceBundle('zh', 'translation', {
    'Online top-up succeeded, amount added: {{quota}}, payment amount: {{amount}}':
      '在线充值成功，充值额度：{{quota}}，支付金额：{{amount}}',
  })
  await i18n.changeLanguage('zh')
  const preview = renderPreview(
    {
      content_parts: [
        {
          key: 'Online top-up succeeded, amount added: {{quota}}, payment amount: {{amount}}',
          params: { quota: '$10.000000', amount: '10.00' },
        },
      ],
    },
    false,
    { type: 1 }
  )
  expect(preview.textContent).toBe(
    '在线充值成功，充值额度：$10.000000，支付金额：10.00'
  )
})

test('quota saturation remains first and only billing adds to the counter', () => {
  const preview = renderPreview({
    model_price: 0.25,
    admin_info: {
      task_plugin: plugin,
      quota_saturation: {
        op: 'round',
        kind: 'overflow',
        original: 3e9,
        clamped: 2147483647,
      },
    },
  })
  expect(preview.textContent).toBe('Quota clamped+1')
})

test.each([true, false])(
  'plugin information in the opened dialog respects admin=%s',
  async (isAdmin) => {
    const preview = renderPreview(
      { model_price: 0.25, admin_info: { task_plugin: plugin } },
      isAdmin
    )
    expect(preview.textContent).toBe('Per-call · $0.25')
    fireEvent.click(preview)
    const dialog = within(await screen.findByRole('dialog'))
    if (isAdmin) {
      expect(dialog.getByText('Incho')).toBeVisible()
      expect(dialog.getByText('1.0.1')).toBeVisible()
      expect(dialog.getByText('Plugin maintainer')).toBeVisible()
    } else {
      expect(dialog.queryByText('Incho')).not.toBeInTheDocument()
      expect(dialog.queryByText('Plugin maintainer')).not.toBeInTheDocument()
    }
  }
)

test.each([
  {
    expression: 'tier("music", u("clips") * 0.25)',
    tier: 'music',
    expected: 'music · clips $0.25/unit',
  },
  {
    expression:
      'u("mode") == "pro" ? tier("pro", u("seconds") * 0.8) : tier("std", u("seconds") * 0.4)',
    tier: 'pro',
    expected: 'pro · seconds $0.8/second',
  },
  {
    expression: 'tier("tokens", u("tokens") * 9.8 / 1000000)',
    tier: 'tokens',
    expected: 'tokens · tokens $9.8/1M token',
  },
  {
    expression: 'tier("free", u("clips") * 0)',
    tier: 'free',
    expected: 'free · clips $0/unit',
  },
  {
    expression: 'tier("mixed", 0.1 + u("clips") * 0.25 + u("units") * 0.14)',
    tier: 'mixed',
    expected:
      'mixed · clips $0.25/unit · units $0.14/credit · Additional charge $0.1/request',
  },
])(
  'task expression $tier shows its recorded unit price',
  ({ expression, tier, expected }) => {
    client.setQueryData(['pricing'], {
      data: [
        {
          model_name: 'wan2.5-i2v-preview',
          billing_expr: 'tier("current", u("clips") * 99)',
          billing_usage_schema: {
            clips: { type: 'number', unit: 'count' },
            seconds: { type: 'number', unit: 'second' },
            tokens: { type: 'number', unit: 'token' },
            units: { type: 'number', unit: 'credit' },
            mode: { enum: ['pro', 'std'] },
          },
        },
      ],
      vendors: [],
    })
    const preview = renderPreview({
      is_task: true,
      billing_mode: 'tiered_expr',
      expr_b64: Buffer.from(expression).toString('base64'),
      matched_tier: tier,
      model_price: 0,
      admin_info: { task_plugin: plugin },
    })
    expect(preview.textContent).toBe(expected)
  }
)

test('task log prices use localized unit labels from pricing metadata', async () => {
  client.setQueryData(['pricing'], {
    data: [
      {
        model_name: 'wan2.5-i2v-preview',
        billing_usage_schema: {
          images: {
            type: 'number',
            unit: 'count',
            unitLabel: { en: 'image', zh: '张' },
          },
        },
      },
    ],
    vendors: [],
  })
  const preview = renderPreview({
    is_task: true,
    billing_mode: 'tiered_expr',
    expr_b64: btoa('tier("images", u("images") * 0.25)'),
    matched_tier: 'images',
  })
  expect(preview).toHaveTextContent('images · images $0.25/image')
  await act(() => i18n.changeLanguage('zh-CN'))
  expect(
    screen.getByRole('button', { name: /images · images/ })
  ).toHaveTextContent('images · images $0.25/张')
})

test('task log prices select the executing provider’s schema', () => {
  client.setQueryData(['pricing'], {
    data: [
      {
        model_name: 'wan2.5-i2v-preview',
        billing_usage_schema: { seconds: { type: 'number', unit: 'second' } },
        billing_plugin_variants: [
          {
            plugin_key: 'beta',
            plugin_name: 'Beta',
            billing_expr: 'tier("images", u("images") * 0.25)',
            billing_usage_schema: {
              images: {
                type: 'number',
                unit: 'count',
                unitLabel: { en: 'image' },
              },
            },
          },
        ],
      },
    ],
    vendors: [],
  })
  const preview = renderPreview({
    is_task: true,
    billing_mode: 'tiered_expr',
    expr_b64: btoa('tier("images", u("images") * 0.25)'),
    matched_tier: 'images',
    admin_info: {
      task_plugin: { key: 'beta', name: 'Beta', version: '1.0.0' },
    },
  })
  expect(preview).toHaveTextContent('images · images $0.25/image')
})

test.each(['missing schema', 'unsupported expression', 'unknown tier'])(
  'task pricing with %s preserves the saved tier without guessing prices',
  (scenario) => {
    if (scenario !== 'missing schema') {
      client.setQueryData(['pricing'], {
        data: [
          {
            model_name: 'wan2.5-i2v-preview',
            billing_usage_schema: { clips: { type: 'number', unit: 'count' } },
          },
        ],
        vendors: [],
      })
    }
    const expression =
      scenario === 'unsupported expression'
        ? 'tier("music", max(u("clips"), 1) * 0.25)'
        : 'tier("music", u("clips") * 0.25)'
    const preview = renderPreview({
      is_task: true,
      billing_mode: 'tiered_expr',
      expr_b64: Buffer.from(expression).toString('base64'),
      matched_tier: scenario === 'unknown tier' ? 'old' : 'music',
    })
    expect(preview.textContent).toBe(
      `Dynamic Pricing · ${scenario === 'unknown tier' ? 'old' : 'music'}`
    )
  }
)

test('settled unit prices apply only recorded matched rules and the group ratio', async () => {
  const preview = renderPreview(
    {
      billing_mode: 'tiered_expr',
      expr_b64: btoa('tier("base", p * 7 + c * 30 + cr * 0.16)'),
      matched_tier: 'base',
      group_ratio: 0.8,
      cache_tokens: 100,
      request_rules: [
        {
          cond: 'hour("Asia/Shanghai") >= 18',
          multiplier: 0.5,
          matched: false,
        },
        { cond: 'hour("Asia/Shanghai") >= 12', multiplier: 0.5, matched: true },
      ],
    },
    false
  )
  fireEvent.click(preview)
  const table = within(
    await screen.findByRole('table', { name: 'Settled unit prices' })
  )
  expect(table.getByRole('row', { name: 'Input $7 $2.8' })).toBeVisible()
  expect(table.getByRole('row', { name: 'Output $30 $12' })).toBeVisible()
  expect(
    table.getByRole('columnheader', {
      name: 'Final unit price (×0.5 ×0.8)',
    })
  ).toBeVisible()
  expect(
    table.getByRole('row', { name: 'Cache Read $0.16 $0.064' })
  ).toBeVisible()
  fireEvent.click(screen.getByRole('button', { name: 'View calculation' }))
  expect(await screen.findByText('$7 × 0.5 × 0.8 = $2.8')).toBeVisible()
})

test('settled usage cost shows recorded token formula behind a collapsed control', async () => {
  const preview = renderPreview(
    {
      billing_mode: 'tiered_expr',
      expr_b64: btoa('tier("base", p * 7 + c * 30 + cr * 0.16)'),
      matched_tier: 'base',
      group_ratio: 0.8,
      billing_tokens: { p: 1000, c: 200, cr: 500 },
      request_rules: [
        { cond: 'hour("Asia/Shanghai") >= 12', multiplier: 0.5, matched: true },
      ],
    },
    false
  )
  fireEvent.click(preview)

  const usageTable = within(
    await screen.findByRole('table', { name: 'Usage cost' })
  )
  expect(
    usageTable.getByRole('columnheader', {
      name: 'Final unit price (×0.5 ×0.8)',
    })
  ).toBeVisible()
  expect(usageTable.getByRole('row', { name: /Input 1,000/ })).toBeVisible()
  expect(usageTable.getByRole('row', { name: /Output 200/ })).toBeVisible()
  expect(usageTable.getByRole('row', { name: /Cache Read 500/ })).toBeVisible()
  expect(screen.getByText('Total usage cost')).toBeVisible()
  expect(screen.queryByText(/1,000,000 × 1,000/)).not.toBeInTheDocument()
  expect(screen.queryByRole('textbox')).not.toBeInTheDocument()

  fireEvent.click(
    screen.getByRole('button', { name: 'View usage cost formula' })
  )
  expect(
    await screen.findByText(/\$2\.8 ÷ 1,000,000 × 1,000 = \$0\.0028/)
  ).toBeVisible()
})

test('settled usage cost rebuilds the formula from the logged token counts', async () => {
  // Production log: cache hits are already included in prompt_tokens, so the
  // billed prompt count must subtract them before applying the final price.
  const preview = renderPreview(
    {
      billing_mode: 'tiered_expr',
      expr_b64: btoa('tier("base", p * 7 + c * 30 + cr * 0.16)'),
      matched_tier: 'base',
      group_ratio: 0.27,
      cache_tokens: 372736,
      request_rules: [
        {
          cond: 'hour("Asia/Shanghai") >= 18',
          multiplier: 0.5,
          matched: false,
        },
        {
          cond: 'hour("Asia/Shanghai") >= 12',
          multiplier: 0.5,
          matched: false,
        },
      ],
    },
    false,
    { prompt_tokens: 373220, completion_tokens: 1713, quota: 15446 }
  )
  fireEvent.click(preview)

  const usageTable = within(
    await screen.findByRole('table', { name: 'Usage cost' })
  )
  expect(
    usageTable.getByRole('row', { name: /Input 484 \$1\.89/ })
  ).toBeVisible()
  expect(
    usageTable.getByRole('row', { name: /Output 1,713 \$8\.1/ })
  ).toBeVisible()
  expect(
    usageTable.getByRole('row', { name: /Cache Read 372,736 \$0\.0432/ })
  ).toBeVisible()
  const usageSection = within(
    screen.getByRole('region', { name: 'Usage cost' })
  )
  expect(usageSection.getByText('$0.030892')).toBeVisible()

  fireEvent.click(
    screen.getByRole('button', { name: 'View usage cost formula' })
  )
  expect(
    await screen.findByText(/\$1\.89 ÷ 1,000,000 × 484 = \$0\.00091476/)
  ).toBeVisible()
  expect(
    usageSection.queryByText(
      'Token subtotals differ from the recorded charge. Rounding or additional charges may apply; the log charge is authoritative.'
    )
  ).not.toBeInTheDocument()
})

test.each([
  {
    user_group_ratio: 0.2,
    group_ratio: 0.8,
    expected: 'Input $7 $1.4',
    header: 'Final unit price (×2 ×0.5 ×0.2)',
  },
  {
    user_group_ratio: 0,
    group_ratio: 0.8,
    expected: 'Input $7 $0',
    header: 'Final unit price (×2 ×0.5 ×0)',
  },
  {
    user_group_ratio: -1,
    group_ratio: 0.8,
    expected: 'Input $7 $5.6',
    header: 'Final unit price (×2 ×0.5 ×0.8)',
  },
])(
  'settled prices respect the recorded effective group ratio: $expected',
  async (scenario) => {
    fireEvent.click(
      renderPreview({
        billing_mode: 'tiered_expr',
        expr_b64: btoa('tier("base", p * 7 + c * 0)'),
        matched_tier: 'base',
        group_ratio: scenario.group_ratio,
        user_group_ratio: scenario.user_group_ratio,
        request_rules: [
          { cond: 'param("fast") == true', multiplier: 2, matched: true },
          {
            cond: 'hour("Asia/Shanghai") >= 12',
            multiplier: 0.5,
            matched: true,
          },
        ],
      })
    )
    const table = within(
      await screen.findByRole('table', { name: 'Settled unit prices' })
    )
    expect(table.getByRole('row', { name: scenario.expected })).toBeVisible()
    expect(
      table.getByRole('columnheader', { name: scenario.header })
    ).toBeVisible()
    expect(table.getByRole('row', { name: 'Output $0 $0' })).toBeVisible()
  }
)

test.each([
  { name: 'missing group ratio', fields: { group_ratio: undefined } },
  { name: 'unknown tier', fields: { matched_tier: 'unknown' } },
  {
    name: 'missing rule traces',
    fields: {
      expr_b64: btoa(
        '(tier("base", p * 7)) * (hour("Asia/Shanghai") >= 12 ? 0.5 : 1)'
      ),
    },
  },
  {
    name: 'ambiguous tier',
    fields: {
      expr_b64: btoa('len > 100 ? tier("base", p * 9) : tier("base", p * 7)'),
    },
  },
  {
    name: 'same label with different billing units',
    fields: {
      billing_unit: 'token' as const,
      expr_b64: btoa(
        'len > 100 ? tier("base", fixed(0.1)) : tier("base", p * 7)'
      ),
    },
  },
  { name: 'negative ratio', fields: { group_ratio: -2 } },
])('does not invent settled prices for $name', async ({ fields }) => {
  fireEvent.click(
    renderPreview({
      billing_mode: 'tiered_expr',
      expr_b64: btoa('tier("base", p * 7)'),
      matched_tier: 'base',
      group_ratio: 0.8,
      ...fields,
    })
  )
  expect(
    await screen.findByText(
      'Historical billing data is incomplete; final unit prices are unavailable.'
    )
  ).toBeVisible()
  expect(
    screen.queryByRole('table', { name: 'Settled unit prices' })
  ).not.toBeInTheDocument()
})

test('settled cache-write prices preserve both durations and small nonzero prices', async () => {
  fireEvent.click(
    renderPreview({
      billing_mode: 'tiered_expr',
      expr_b64: btoa('tier("base", p * 7 + cc * 0.00001 + cc1h * 0.00002)'),
      matched_tier: 'base',
      group_ratio: 0.8,
      cache_creation_tokens_5m: 100,
      cache_creation_tokens_1h: 100,
    })
  )
  const table = within(
    await screen.findByRole('table', { name: 'Settled unit prices' })
  )
  expect(table.getByRole('row', { name: /\$0.00001 \$0.000008/ })).toBeVisible()
  expect(table.getByRole('row', { name: /\$0.00002 \$0.000016/ })).toBeVisible()
})

test('fixed per-image settlement shows unit price without multiplying image count again', async () => {
  fireEvent.click(
    renderPreview({
      billing_mode: 'tiered_expr',
      expr_b64: btoa('tier("base", fixed(0.1)) * image_count'),
      matched_tier: 'base',
      billing_unit: 'request',
      fixed_price: 0.1,
      image_count: 4,
      group_ratio: 0.8,
    })
  )
  const table = within(
    await screen.findByRole('table', { name: 'Settled unit prices' })
  )
  expect(table.getByRole('row', { name: 'Per image $0.1 $0.08' })).toBeVisible()
  expect(screen.getByText('Unit: USD / image')).toBeVisible()
})

const videoExpression =
  'u("resolution") == "480p" ? tier("480p", u("input_images") * 0.1 + u("seconds") * 0.5) : u("resolution") == "720p" ? tier("720p", u("input_images") * 0.1 + u("seconds") * 0.7) : tier("1080p", u("input_images") * 0.1 + u("seconds") * 1)'
const videoSchema = {
  seconds: { type: 'number' as const, unit: 'second' as const },
  input_images: {
    type: 'number' as const,
    unit: 'count' as const,
    unitLabel: { en: 'image', zh: '张' },
  },
  resolution: { enum: ['480p', '720p', '1080p'] },
}
const videoOther: LogOtherData = {
  is_task: true,
  billing_mode: 'tiered_expr',
  expr_b64: btoa(videoExpression),
  matched_tier: '720p',
  group_ratio: 0.27,
  usage_facts: { seconds: 5, resolution: '720p', input_images: 0 },
}

test('video settlement shows per-second and per-image prices, costs and persistent multipliers', async () => {
  client.setQueryData(['pricing'], {
    data: [
      {
        model_name: 'grok-imagine-video-1.5',
        billing_usage_schema: videoSchema,
      },
    ],
    vendors: [],
  })
  fireEvent.click(
    renderPreview(videoOther, false, {
      model_name: 'grok-imagine-video-1.5',
      quota: 472500,
    })
  )
  const prices = within(
    await screen.findByRole('table', { name: 'Settled unit prices' })
  )
  expect(
    prices.getByRole('row', { name: 'seconds $0.7/second $0.189/second' })
  ).toBeVisible()
  expect(
    prices.getByRole('row', { name: 'input_images $0.1/image $0.027/image' })
  ).toBeVisible()
  const usage = within(await screen.findByRole('table', { name: 'Usage cost' }))
  expect(
    usage.getByRole('row', { name: 'seconds 5 s $0.189/second $0.945' })
  ).toBeVisible()
  expect(
    usage.getByRole('row', { name: 'input_images 0 image $0.027/image $0' })
  ).toBeVisible()
  for (const table of [prices, usage]) {
    expect(
      table.getByRole('columnheader', { name: 'Final unit price (×0.27)' })
    ).toBeVisible()
  }
  expect(
    within(screen.getByRole('region', { name: 'Usage cost' })).getByText(
      '$0.945',
      { selector: 'strong' }
    )
  ).toBeVisible()
})

test('historical task settlement uses logged prices instead of the current price expression', async () => {
  client.setQueryData(['pricing'], {
    data: [
      {
        model_name: 'wan2.5-i2v-preview',
        billing_usage_schema: videoSchema,
        billing_expr: videoExpression,
      },
    ],
    vendors: [],
  })
  fireEvent.click(
    renderPreview(
      {
        ...videoOther,
        expr_b64: btoa('tier("base", u("seconds") * 0.5)'),
        matched_tier: 'base',
      },
      false,
      { quota: 337500 }
    )
  )
  const prices = within(
    await screen.findByRole('table', { name: 'Settled unit prices' })
  )
  expect(
    prices.getByRole('row', { name: 'seconds $0.5/second $0.135/second' })
  ).toBeVisible()
  const usage = within(await screen.findByRole('table', { name: 'Usage cost' }))
  expect(
    usage.getByRole('row', { name: 'seconds 5 s $0.135/second $0.675' })
  ).toBeVisible()
})

test('missing recorded task quantities keep the charge visible without inventing zero usage', async () => {
  client.setQueryData(['pricing'], {
    data: [
      { model_name: 'wan2.5-i2v-preview', billing_usage_schema: videoSchema },
    ],
    vendors: [],
  })
  fireEvent.click(
    renderPreview(
      { ...videoOther, usage_facts: { resolution: '720p' } },
      false,
      { quota: 472500 }
    )
  )
  const section = within(
    await screen.findByRole('region', { name: 'Usage cost' })
  )
  expect(section.queryByRole('table')).not.toBeInTheDocument()
  expect(section.getByText('$0.945')).toBeVisible()
})

test('task settlement does not invent prices for ambiguous historical tiers', async () => {
  client.setQueryData(['pricing'], {
    data: [
      { model_name: 'wan2.5-i2v-preview', billing_usage_schema: videoSchema },
    ],
    vendors: [],
  })
  fireEvent.click(
    renderPreview({
      ...videoOther,
      expr_b64: btoa(
        'u("resolution") == "480p" ? tier("same", u("seconds") * 0.5) : tier("same", u("seconds") * 0.7)'
      ),
      matched_tier: 'same',
      usage_facts: undefined,
    })
  )
  expect(
    await screen.findByText(
      'Historical billing data is incomplete; final unit prices are unavailable.'
    )
  ).toBeVisible()
  expect(
    screen.queryByRole('table', { name: 'Settled unit prices' })
  ).not.toBeInTheDocument()
})

test('task settlement requires schema for every usage field in the logged expression', async () => {
  client.setQueryData(['pricing'], {
    data: [
      {
        model_name: 'wan2.5-i2v-preview',
        billing_usage_schema: {
          seconds: videoSchema.seconds,
          resolution: videoSchema.resolution,
        },
      },
    ],
    vendors: [],
  })
  fireEvent.click(renderPreview(videoOther))
  expect(
    await screen.findByText(
      'Historical billing data is incomplete; final unit prices are unavailable.'
    )
  ).toBeVisible()
  expect(
    screen.queryByRole('table', { name: 'Settled unit prices' })
  ).not.toBeInTheDocument()
})

test('task settlement scales token usage once and adds a per-request charge once', async () => {
  client.setQueryData(['pricing'], {
    data: [
      {
        model_name: 'wan2.5-i2v-preview',
        billing_usage_schema: { tokens: { type: 'number', unit: 'token' } },
      },
    ],
    vendors: [],
  })
  fireEvent.click(
    renderPreview(
      {
        ...videoOther,
        expr_b64: btoa('tier("base", u("tokens") * 2 / 1000000 + 0.05)'),
        matched_tier: 'base',
        usage_facts: { tokens: 1000 },
      },
      false,
      { quota: 7020 }
    )
  )
  const usage = within(await screen.findByRole('table', { name: 'Usage cost' }))
  expect(
    usage.getByRole('row', {
      name: 'tokens 1,000 token $0.54/1M token $0.00054',
    })
  ).toBeVisible()
  expect(
    usage.getByRole('row', {
      name: 'Additional charge 1 request $0.0135/request $0.0135',
    })
  ).toBeVisible()
  expect(
    within(screen.getByRole('region', { name: 'Usage cost' })).getByText(
      '$0.01404',
      { selector: 'strong' }
    )
  ).toBeVisible()
})

test.each([
  ['zhCN', '1,234.56789'],
  ['zhTW', '1,234.56789'],
  ['en', '1,234.56789'],
  ['fr', '1\u202f234,56789'],
  ['ru', '1\u00a0234,56789'],
  ['ja', '1,234.56789'],
  ['vi', '1.234,56789'],
])('task quantities keep full precision in %s', async (language, quantity) => {
  i18n.addResourceBundle(language, 'translation', en.translation)
  await i18n.changeLanguage(language)
  client.setQueryData(['pricing'], {
    data: [
      { model_name: 'wan2.5-i2v-preview', billing_usage_schema: videoSchema },
    ],
    vendors: [],
  })
  fireEvent.click(
    renderPreview({
      ...videoOther,
      expr_b64: btoa('tier("base", u("seconds") * 0.5)'),
      matched_tier: 'base',
      usage_facts: { seconds: 1234.56789 },
    })
  )
  const usage = within(await screen.findByRole('table', { name: 'Usage cost' }))
  expect(usage.getByRole('cell', { name: `${quantity} s` })).toBeVisible()
})
