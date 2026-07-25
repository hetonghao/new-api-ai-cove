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

import type { RiskRecordFilterDraft, RiskRecordFilters } from '../types'

export const EMPTY_RISK_RECORD_FILTER_DRAFT: RiskRecordFilterDraft = {
  start_time: '',
  end_time: '',
  channel_id: '',
  user_id: '',
  provider_id: '',
  result: '',
  source: '',
}

const RISK_RECORD_RESULT_LABELS: Readonly<Record<string, string>> = {
  safe: 'Safe',
  unsafe: 'Unsafe',
  error: 'Error',
  not_reviewed: 'Not reviewed',
}

const RISK_RECORD_RESULT_VARIANTS: Readonly<Record<string, StatusVariant>> = {
  safe: 'success',
  unsafe: 'danger',
  error: 'warning',
  not_reviewed: 'neutral',
}

const RISK_RECORD_SOURCE_LABELS: Readonly<Record<string, string>> = {
  provider: 'Provider source',
  cache: 'Cache source',
  inflight: 'In-flight source',
  local: 'Local source',
}

export function getRiskRecordResultLabel(result: string) {
  return RISK_RECORD_RESULT_LABELS[result]
}

export function getRiskRecordResultVariant(result: string): StatusVariant {
  return RISK_RECORD_RESULT_VARIANTS[result] ?? 'neutral'
}

export function getRiskRecordSourceLabel(source: string) {
  return RISK_RECORD_SOURCE_LABELS[source]
}

export function getRiskRecordSourceVariant(source: string): StatusVariant {
  return RISK_RECORD_SOURCE_LABELS[source] ? 'info' : 'neutral'
}

export function getRiskRecordTotalPages(total: number, pageSize: number) {
  return Math.max(1, Math.ceil(total / pageSize))
}

function toUnixSeconds(value: string): number | undefined {
  if (!value) return undefined
  const timestamp = new Date(value).getTime()
  return Number.isNaN(timestamp) ? undefined : Math.floor(timestamp / 1000)
}

function toPositiveInteger(value: string): number | undefined {
  return value ? Number(value) : undefined
}

export function commitRiskRecordFilters(
  draft: RiskRecordFilterDraft
): RiskRecordFilters {
  const startTimestamp = toUnixSeconds(draft.start_time)
  const endTimestamp = toUnixSeconds(draft.end_time)
  const channelId = toPositiveInteger(draft.channel_id)
  const userId = toPositiveInteger(draft.user_id)
  const providerId = toPositiveInteger(draft.provider_id)

  return {
    ...(startTimestamp === undefined
      ? {}
      : { start_timestamp: startTimestamp }),
    ...(endTimestamp === undefined ? {} : { end_timestamp: endTimestamp }),
    ...(channelId === undefined ? {} : { channel_id: channelId }),
    ...(userId === undefined ? {} : { user_id: userId }),
    ...(providerId === undefined ? {} : { provider_id: providerId }),
    ...(draft.result ? { result: draft.result } : {}),
    ...(draft.source ? { source: draft.source } : {}),
  }
}

export function buildRiskRecordQueryParams(
  page: number,
  pageSize: number,
  filters: RiskRecordFilters
) {
  return { p: page, page_size: pageSize, ...filters }
}
