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
import { afterEach, describe, expect, test } from 'vitest'

import type { UsageLog } from '../../data/schema'
import type { LogOtherData } from '../../types'
import { DetailsDialog } from '../dialogs/details-dialog'

const queryClients: QueryClient[] = []

function makeLog(other: LogOtherData): UsageLog {
  return {
    id: 1,
    user_id: 1,
    created_at: 1,
    type: 5,
    content: '',
    username: 'user',
    token_name: 'token',
    model_name: 'swe-2',
    quota: 0,
    prompt_tokens: 0,
    completion_tokens: 0,
    use_time: 0,
    is_stream: false,
    channel: 1,
    channel_name: 'channel',
    token_id: 1,
    group: 'default',
    ip: '',
    other: JSON.stringify(other),
    request_id: 'req-1',
    upstream_request_id: '',
  }
}

function renderDetails(isAdmin: boolean, other: LogOtherData): void {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  const freshAt = Date.now() + 60_000
  queryClient.setQueryData(['status'], {}, { updatedAt: freshAt })
  queryClients.push(queryClient)

  render(
    <QueryClientProvider client={queryClient}>
      <DetailsDialog
        log={makeLog(other)}
        isAdmin={isAdmin}
        isRoot={false}
        open
        onOpenChange={() => undefined}
      />
    </QueryClientProvider>
  )
}

afterEach(() => {
  for (const queryClient of queryClients) {
    queryClient.clear()
  }
  queryClients.length = 0
})

describe.each([false, true])(
  'response model details (admin-only: %s)',
  (adminOnly) => {
    test.each([
      ['swe-2', 'swe-2', 'swe-2'],
      ['swe-2', 'swe-2', 'devin/swe-2'],
      ['swe-2', 'swe-2', 'SWE-2'],
      ['swe-2', 'swe-2', 'swe-2-2026-09-17'],
      ['gpt-5.6-terra', 'gpt-5.6-terra', 'devin/gpt-5-6-terra'],
      ['swe-2', 'mapped-model', 'vendor/mapped-model-2026-09-17'],
    ])(
      'hides compatible response %s / %s / %s',
      (requested, upstream, returned) => {
        const responseModel = {
          requested_model: requested,
          upstream_model: upstream,
          returned_model: returned,
          mismatch: true,
        }
        renderDetails(
          adminOnly,
          adminOnly
            ? { admin_info: { response_model: responseModel } }
            : { response_model: responseModel }
        )

        expect(screen.getByRole('dialog')).toBeInTheDocument()
        expect(screen.queryAllByText('Response Model')).toHaveLength(0)
        expect(screen.queryByText('Upstream Model')).not.toBeInTheDocument()
        expect(screen.queryByText(/^Response model:/)).not.toBeInTheDocument()
      }
    )

    test('shows a genuine mismatch and all three model names', () => {
      const responseModel = {
        requested_model: 'swe-2',
        upstream_model: 'mapped-model',
        returned_model: 'different-model',
        mismatch: false,
      }
      renderDetails(
        adminOnly,
        adminOnly
          ? { admin_info: { response_model: responseModel } }
          : { response_model: responseModel }
      )

      expect(screen.getByText('Request Model')).toBeInTheDocument()
      expect(screen.getByText('Upstream Model')).toBeInTheDocument()
      expect(screen.getByText('mapped-model')).toBeInTheDocument()
      expect(screen.getByText('different-model')).toBeInTheDocument()
      expect(
        screen.getByText('Response model: different-model')
      ).toBeInTheDocument()
    })
  }
)

test('hides admin-only response observations from non-admin users', () => {
  renderDetails(false, {
    admin_info: {
      response_model: {
        requested_model: 'swe-2',
        upstream_model: 'swe-2',
        returned_model: 'different-model',
      },
    },
  })

  expect(screen.queryByText('different-model')).not.toBeInTheDocument()
  expect(screen.queryByText('Response Model')).not.toBeInTheDocument()
})
