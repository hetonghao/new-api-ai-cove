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
import { describe, expect, test, vi } from 'vitest'

import type { QualityCaseView } from '../../types'
import { RunControls } from '../run-controls'

function makeCase(): QualityCaseView {
  return {
    id: 1,
    name: 'Public quality case',
    description: '',
    enabled: true,
    archived: false,
    version: 1,
    edit_version: 1,
    schedule_version: 1,
    next_run_at: 0,
    last_schedule_reason: '',
    active_run_id: '',
    created_by: 1,
    updated_by: 1,
    created_at: 0,
    updated_at: 0,
    config: {
      model: 'test-model',
      output_type: 'svg',
      mode: 'channel',
      protocol: 'responses',
      token_id: 0,
      group: '',
      channel_ids: null,
      prompt: 'Draw a pelican',
      instruction: '',
      instruction_role: '',
      max_output_tokens: 16384,
      reasoning_effort: '',
      samples_per_target: 2,
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

function renderControls(qualityCase: QualityCaseView, canOperate = false) {
  const client = new QueryClient()
  return render(
    <QueryClientProvider client={client}>
      <RunControls
        qualityCase={qualityCase}
        canOperate={canOperate}
        onEdit={vi.fn()}
      />
    </QueryClientProvider>
  )
}

describe('RunControls', () => {
  test('public viewers with redacted channels see a disabled run button without crashing', () => {
    renderControls(makeCase())

    expect(screen.getByRole('button', { name: 'Run now' })).toBeDisabled()
    expect(
      screen.queryByRole('button', { name: 'Edit' })
    ).not.toBeInTheDocument()
    expect(screen.queryByRole('alertdialog')).not.toBeInTheDocument()
  })

  test('public viewers with redacted channels see an active run but cannot stop it', () => {
    const qualityCase = makeCase()
    qualityCase.active_run_id = 'running-case'
    renderControls(qualityCase)

    expect(screen.getByText('Run in progress')).toBeVisible()
    expect(screen.getByRole('button', { name: 'Stop' })).toBeDisabled()
    expect(
      screen.queryByRole('button', { name: 'Edit' })
    ).not.toBeInTheDocument()
  })

  test.each([
    { mode: 'channel' as const, channels: [7], targets: 1, samples: 2 },
    { mode: 'channel' as const, channels: [7, 8, 9], targets: 3, samples: 6 },
    { mode: 'route' as const, channels: [], targets: 1, samples: 2 },
  ])(
    'operators confirm $samples samples for $targets targets in $mode mode',
    async (scenario) => {
      const user = userEvent.setup()
      const qualityCase = makeCase()
      qualityCase.config.mode = scenario.mode
      qualityCase.config.channel_ids = scenario.channels
      renderControls(qualityCase, true)

      await user.click(screen.getByRole('button', { name: 'Run now' }))

      expect(screen.getByRole('alertdialog')).toHaveTextContent(
        `This will start ${scenario.samples} sample(s) against ${scenario.targets} target(s). Sampling consumes quota.`
      )
    }
  )
})
