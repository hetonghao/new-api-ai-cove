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
import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import type { ReactNode } from 'react'
import { beforeEach, describe, expect, test, vi } from 'vitest'

import type { ModelQualitySample } from '../../types'
import { SampleDetailDialog } from '../sample-detail-dialog'

const { getQualityArtifact } = vi.hoisted(() => ({
  getQualityArtifact: vi.fn(),
}))

vi.mock('../../api', async (importOriginal) => {
  const original = await importOriginal<typeof import('../../api')>()
  return { ...original, getQualityArtifact }
})

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

function renderDialog(sample: ModelQualitySample) {
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
      <SampleDetailDialog
        sample={sample}
        open
        onOpenChange={vi.fn()}
        canOperate={false}
        showChannel
      />
    </Wrapper>
  )
}

describe('SampleDetailDialog', () => {
  beforeEach(() => {
    getQualityArtifact.mockReset()
  })

  test('a failed sample fetches its artifact and the source tab shows raw text', async () => {
    getQualityArtifact.mockResolvedValue({
      success: true,
      data: {
        sample_id: 1,
        text: 'What would you like help with?',
        svg: '',
        sha256: 'x',
        validation_version: 'svg-static-v1',
        expired: false,
      },
    })
    renderDialog(makeSample({ status: 'failed', error_code: 'missing_svg' }))
    await waitFor(() => {
      expect(getQualityArtifact).toHaveBeenCalledWith(1)
    })
    fireEvent.click(screen.getByRole('tab', { name: 'Source' }))
    expect(
      await screen.findByText('What would you like help with?')
    ).toBeInTheDocument()
  })

  test('a succeeded svg sample shows the svg source, not the text field', async () => {
    getQualityArtifact.mockResolvedValue({
      success: true,
      data: {
        sample_id: 1,
        text: 'raw',
        svg: '<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 1 1"><rect width="1" height="1"/></svg>',
        sha256: 'x',
        validation_version: 'svg-static-v1',
        expired: false,
      },
    })
    renderDialog(makeSample({}))
    await waitFor(() => {
      expect(getQualityArtifact).toHaveBeenCalledWith(1)
    })
    fireEvent.click(screen.getByRole('tab', { name: 'Source' }))
    expect(
      await screen.findByText(/<svg xmlns="http:\/\/www\.w3\.org\/2000\/svg"/)
    ).toBeInTheDocument()
    expect(screen.queryByText('raw')).not.toBeInTheDocument()
  })
})
