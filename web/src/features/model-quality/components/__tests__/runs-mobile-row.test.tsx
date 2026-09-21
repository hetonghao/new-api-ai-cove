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
import type { ReactNode } from 'react'
import { beforeEach, describe, expect, test, vi } from 'vitest'

import { RunsTable } from '../runs-table'
import type {
  ModelQualityRun,
  QualityCapabilities,
  QualityCaseView,
} from '../../types'

const { get } = vi.hoisted(() => ({ get: vi.fn() }))

vi.mock('@/lib/api', () => ({
  api: { get, post: vi.fn() },
}))

function makeRun(): ModelQualityRun {
  return {
    id: 'run-abc-1',
    case_id: 1,
    version: 2,
    source: 'manual',
    status: 'running',
    cancel_requested: false,
    created_by: 1,
    created_at: 1700000000000,
    started_at: 1700000001000,
    finished_at: 0,
    sample_count: 3,
    executor_id: 'exec-1',
  }
}

function makeCase(): QualityCaseView {
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
    active_run_id: 'run-abc-1',
    created_by: 1,
    updated_by: 1,
    created_at: 0,
    updated_at: 0,
    config: {
      model: 'gpt-6-astra',
      output_type: 'svg',
      mode: 'route',
      protocol: 'responses',
      token_id: 1,
      group: '',
      channel_ids: [],
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
  }
}

const CAPABILITIES: QualityCapabilities = {
  channels: [],
  tokens: [],
  can_operate: true,
  can_configure: false,
}

describe('RunsTable mobile row', () => {
  beforeEach(() => {
    get.mockReset()
  })

  test('stop button is a sibling, never nested inside the row button', async () => {
    get.mockResolvedValue({
      data: {
        success: true,
        data: { items: [makeRun()], total: 1, page: 1, page_size: 20 },
      },
    })
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
    const { container } = render(
      <Wrapper>
        <RunsTable cases={[makeCase()]} capabilities={CAPABILITIES} />
      </Wrapper>
    )

    const stopButtons = await screen.findAllByRole('button', { name: 'Stop' })
    expect(stopButtons.length).toBeGreaterThan(0)
    expect(container.querySelectorAll('button button')).toHaveLength(0)
  })
})
