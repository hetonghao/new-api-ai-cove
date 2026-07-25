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
