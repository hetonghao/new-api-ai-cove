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
import type { StatusVariant } from '@/components/status-badge'

import type { QualitySampleStatus } from './types'

export const SAMPLE_STATUS_VARIANT: Record<string, StatusVariant> = {
  pending: 'warning',
  requesting: 'info',
  succeeded: 'success',
  failed: 'danger',
  cancelled: 'neutral',
  interrupted: 'warning',
  skipped: 'neutral',
}

export const SAMPLE_STATUS_LABEL: Record<string, string> = {
  pending: 'Pending',
  requesting: 'Running',
  succeeded: 'Succeeded',
  failed: 'Failed',
  cancelled: 'Cancelled',
  interrupted: 'Interrupted',
  skipped: 'Skipped',
}

export const RUN_STATUS_VARIANT: Record<string, StatusVariant> = {
  queued: 'warning',
  running: 'info',
  completed: 'success',
  cancelled: 'neutral',
  interrupted: 'warning',
}

export const RUN_STATUS_LABEL: Record<string, string> = {
  queued: 'Queued',
  running: 'Running',
  completed: 'Completed',
  cancelled: 'Cancelled',
  interrupted: 'Interrupted',
}

export const RUN_SOURCE_LABEL: Record<string, string> = {
  manual: 'Manual',
  scheduled: 'Scheduled',
}

export const ERROR_CODE_LABEL: Record<string, string> = {
  timeout: 'Request timed out',
  upstream_error: 'Upstream error',
  empty_output: 'Empty output',
  missing_terminal: 'Response ended without a terminal event',
  svg_too_large: 'SVG output too large',
  svg_too_complex: 'SVG rejected by safety checks',
  artifact_too_large: 'Artifact exceeds the size limit',
  invalid_configuration: 'Invalid configuration',
  interrupted_unknown: 'Interrupted; result unknown',
  cancelled: 'Cancelled',
  not_approved: 'Execution token is not approved',
  budget_exhausted: 'Daily budget exhausted',
  truncated: 'Output truncated by the token limit',
  incomplete_response: 'Response ended incomplete',
  refusal: 'Model refused the request',
  invalid_response: 'Upstream response could not be parsed',
  invalid_stream: 'Stream event could not be parsed',
  stream_error: 'Upstream reported a stream error',
  duplicate_terminal: 'Stream sent more than one terminal event',
  content_after_terminal: 'Content arrived after the terminal event',
  stream_interrupted: 'Stream interrupted before completion',
  response_read_error: 'Failed to read the upstream response',
  response_too_large: 'Output text exceeds the 2 MiB limit',
  stream_too_large: 'Upstream stream exceeds the 64 MiB limit',
  stream_event_too_large: 'A stream event exceeds the 8 MiB limit',
  executor_interrupted: 'Executor interrupted',
  unsafe_svg: 'SVG contains unsafe content',
  missing_svg: 'No SVG found in the output',
  ambiguous_svg: 'Multiple SVGs found in the output',
  invalid_xml: 'SVG is not valid XML',
  empty_svg: 'SVG has no drawable content',
}

export function errorCodeLabel(
  code: string,
  t: (key: string, options?: Record<string, unknown>) => string
): string {
  const labelKey = ERROR_CODE_LABEL[code]
  if (labelKey) return t(labelKey)
  if (code.startsWith('http_')) {
    return t('Upstream returned HTTP {{status}}', {
      status: code.slice('http_'.length),
    })
  }
  return code
}

export const ANNOTATION_OPTIONS: { value: string; labelKey: string }[] = [
  { value: '', labelKey: 'Not annotated' },
  { value: 'accepted', labelKey: 'Acceptable' },
  { value: 'subject_missing', labelKey: 'Subject missing' },
  { value: 'riding_wrong', labelKey: 'Riding relation wrong' },
  { value: 'bicycle_broken', labelKey: 'Bicycle structure abnormal' },
  { value: 'seaside_missing', labelKey: 'Seaside missing' },
  { value: 'blank', labelKey: 'Possibly blank' },
  { value: 'review', labelKey: 'Needs review' },
]

export function sampleStatusLabel(status: QualitySampleStatus): string {
  return SAMPLE_STATUS_LABEL[status] ?? status
}

export function runStatusLabel(status: string): string {
  return RUN_STATUS_LABEL[status] ?? status
}

export const SAMPLES_PAGE_SIZE = 24
export const RUNS_PAGE_SIZE = 20
export const MAX_COMPARE_SAMPLES = 4
