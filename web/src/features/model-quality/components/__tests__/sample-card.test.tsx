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
import type { ReactNode } from 'react'
import { beforeEach, describe, expect, test, vi } from 'vitest'

import { SampleCard } from '../sample-wall'
import type { ModelQualitySample } from '../../types'

const { get } = vi.hoisted(() => ({ get: vi.fn() }))

vi.mock('@/lib/api', () => ({
  api: { get },
}))

function makeSample(partial: Partial<ModelQualitySample>): ModelQualitySample {
  return {
    id: 1,
    run_id: 'run-1',
    ordinal: 1,
    case_id: 1,
    version: 1,
    target_channel_id: 0,
    channel_id: 5,
    channel_name: 'chan-a',
    model: 'm',
    response_model: 'm',
    output_type: 'svg',
    source: 'manual',
    status: 'succeeded',
    request_success: true,
    request_id: '',
    started_at: 1,
    created_at: 1,
    finished_at: 2,
    duration_ms: 1200,
    first_text_ms: null,
    input_tokens: null,
    output_tokens: null,
    error_code: '',
    validation: '',
    finish_reason: '',
    annotation: '',
    note: '',
    annotated_by: 0,
    annotated_at: 0,
    artifact_expired: false,
    pinned: false,
    ...partial,
  }
}

function renderCard(sample: ModelQualitySample) {
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
      <SampleCard
        sample={sample}
        selectable
        selected={false}
        onToggleSelect={vi.fn()}
        onOpen={vi.fn()}
      />
    </Wrapper>
  )
}

describe('SampleCard', () => {
  beforeEach(() => {
    get.mockReset()
  })

  test('a succeeded svg sample renders the sanitized artifact image', async () => {
    get.mockResolvedValue({
      data: {
        success: true,
        data: {
          sample_id: 1,
          text: '',
          svg: '<svg xmlns="http://www.w3.org/2000/svg"><script>bad()</script><rect width="10" height="10"/></svg>',
          sha256: 'x',
          validation_version: 'svg-static-v1',
          expired: false,
        },
      },
    })
    renderCard(makeSample({}))
    const image = await screen.findByAltText('Sample preview')
    await waitFor(() => {
      expect(image.getAttribute('src')).toMatch(/^data:image\/svg\+xml/)
    })
    const src = decodeURIComponent(image.getAttribute('src') ?? '')
    expect(src).toContain('rect')
    expect(src).not.toContain('script')
  })

  test('a failed sample shows the mapped error label without artifact fetch', () => {
    renderCard(makeSample({ status: 'failed', error_code: 'timeout' }))
    expect(screen.getByText('Request timed out')).toBeInTheDocument()
    expect(get).not.toHaveBeenCalled()
  })

  test('failed preview sits on muted canvas, image branch keeps white', async () => {
    get.mockResolvedValue({
      data: {
        success: true,
        data: {
          sample_id: 1,
          text: '',
          svg: '<svg xmlns="http://www.w3.org/2000/svg"><rect width="10" height="10"/></svg>',
          sha256: 'x',
          validation_version: 'svg-static-v1',
          expired: false,
        },
      },
    })
    const failed = renderCard(
      makeSample({ status: 'failed', error_code: 'timeout' })
    )
    const failedCanvas = failed.container.querySelector('.aspect-\\[3\\/2\\]')
    expect(failedCanvas?.className).not.toContain('bg-white')
    failed.unmount()

    renderCard(makeSample({}))
    const image = await screen.findByAltText('Sample preview')
    expect(image.parentElement?.className).toContain('bg-white')
  })

  test('an unknown error code falls back to the raw code in monospace', () => {
    renderCard(
      makeSample({ status: 'failed', error_code: 'vendor_xyz_500' })
    )
    expect(
      screen.getAllByText('vendor_xyz_500').length
    ).toBeGreaterThanOrEqual(1)
  })

  test('an http_ code maps to a readable upstream status label', () => {
    renderCard(makeSample({ status: 'failed', error_code: 'http_500' }))
    expect(
      screen.getByText('Upstream returned HTTP 500')
    ).toBeInTheDocument()
    expect(screen.getByText('http_500')).toBeInTheDocument()
  })

  test('a pending sample shows status text and never fetches an artifact', () => {
    renderCard(makeSample({ status: 'pending' }))
    expect(screen.getAllByText('Pending').length).toBeGreaterThan(0)
    expect(get).not.toHaveBeenCalled()
  })
})
