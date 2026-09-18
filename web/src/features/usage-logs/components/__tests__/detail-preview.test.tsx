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
  'task pricing with %s shows an explicit unavailable summary',
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
    expect(preview.textContent).toBe('Dynamic Pricing · No matching results')
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
  { user_group_ratio: 0.2, group_ratio: 0.8, expected: 'Input $7 $1.4' },
  { user_group_ratio: 0, group_ratio: 0.8, expected: 'Input $7 $0' },
  { user_group_ratio: -1, group_ratio: 0.8, expected: 'Input $7 $5.6' },
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
