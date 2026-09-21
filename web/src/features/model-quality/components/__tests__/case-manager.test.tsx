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
import { fireEvent, render, screen } from '@testing-library/react'
import type { ReactNode } from 'react'
import { describe, expect, test, vi } from 'vitest'

import { CaseManager } from '../case-manager'
import type { QualityCapabilities, QualityCaseView } from '../../types'

let currentSearch: Record<string, unknown> = {}

vi.mock('@tanstack/react-router', async (importOriginal) => {
  const original =
    await importOriginal<typeof import('@tanstack/react-router')>()
  return {
    ...original,
    getRouteApi: () => ({
      useSearch: () => currentSearch,
    }),
    useNavigate: () => vi.fn(),
  }
})

function makeCase(partial: Partial<QualityCaseView>): QualityCaseView {
  return {
    id: 1,
    name: 'pelican svg',
    description: '',
    enabled: true,
    archived: false,
    version: 2,
    edit_version: 3,
    schedule_version: 1,
    next_run_at: 0,
    last_schedule_reason: '',
    active_run_id: '',
    created_by: 1,
    updated_by: 1,
    created_at: 0,
    updated_at: 0,
    config: {
      model: 'gpt-6-astra',
      output_type: 'svg',
      mode: 'channel',
      protocol: 'responses',
      token_id: 1,
      group: '',
      channel_ids: [7],
      prompt: 'prompt',
      instruction: '',
      instruction_role: '',
      max_output_tokens: 16384,
      reasoning_effort: '',
      samples_per_target: 1,
      timeout_seconds: 180,
      daily_limit: 50,
    },
    schedule: {
      enabled: false,
      kind: 'interval',
      interval_minutes: 60,
      time: '09:00',
      weekdays: [],
      timezone: 'UTC',
    },
    ...partial,
  }
}

const CAPABILITIES: QualityCapabilities = {
  channels: [
    { id: 7, name: 'chan-a', models: ['gpt-6-astra'], groups: ['default'] },
    { id: 8, name: 'chan-b', models: ['other'], groups: ['default'] },
  ],
  tokens: [{ id: 1, name: 'exec', group: 'default' }],
  can_operate: true,
  can_configure: false,
}

function renderManager(cases: QualityCaseView[], search = {}) {
  currentSearch = search
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  function Wrapper(props: { children: ReactNode }) {
    return (
      <QueryClientProvider client={client}>
        {props.children}
      </QueryClientProvider>
    )
  }
  return render(
    <Wrapper>
      <CaseManager cases={cases} capabilities={CAPABILITIES} />
    </Wrapper>
  )
}

describe('CaseManager', () => {
  test('renders the master/detail responsive grid contract', () => {
    const { container } = renderManager([makeCase({})])
    const grid = container.firstElementChild
    expect(grid).toHaveClass('grid-cols-1')
    expect(grid?.className).toContain('lg:grid-cols-[280px_minmax(0,1fr)]')
  })

  test('selecting a case fills the editor name field', () => {
    renderManager(
      [
        makeCase({ id: 1, name: 'first case' }),
        makeCase({ id: 2, name: 'second case' }),
      ],
      { case: 2 }
    )
    expect(screen.getByLabelText('Name')).toHaveValue('second case')
  })

  test('switching mode to normal routing removes the channel checkbox area', () => {
    renderManager([makeCase({})], { case: 1 })
    expect(screen.getByText('chan-a')).toBeInTheDocument()
    const modeSelect = screen.getByLabelText('Route mode')
    fireEvent.change(modeSelect, { target: { value: 'route' } })
    expect(screen.queryByText('Target channels')).toBeNull()
  })
})
