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
import { z } from 'zod'

import type { StatusVariant } from '@/components/status-badge'

import type { RiskRecordFilterDraft, RiskRecordFilters } from '../types'

const RISK_RECORD_FILTER_RESULTS = [
  '',
  'safe',
  'unsafe',
  'error',
  'not_reviewed',
] as const

const RISK_RECORD_FILTER_SOURCES = [
  '',
  'provider',
  'cache',
  'inflight',
  'local',
] as const

type Translate = (key: string) => string

export function createRiskRecordFilterFormSchema(t: Translate) {
  const validDateTime = z.date({ error: t('Invalid configuration') }).optional()
  const positiveInteger = z
    .string()
    .refine(
      (value) => value === '' || /^[1-9]\d*$/.test(value),
      t('Please enter a valid number')
    )
  const nonnegativeInteger = z
    .string()
    .refine(
      (value) => value === '' || /^(0|[1-9]\d*)$/.test(value),
      t('Please enter a valid number')
    )

  return z
    .object({
      start_time: validDateTime,
      end_time: validDateTime,
      channel_id: positiveInteger,
      username: z.string().trim().max(20, t('Invalid configuration')),
      provider_id: nonnegativeInteger,
      result: z.enum(RISK_RECORD_FILTER_RESULTS, {
        error: t('Invalid configuration'),
      }),
      source: z.enum(RISK_RECORD_FILTER_SOURCES, {
        error: t('Invalid configuration'),
      }),
    })
    .superRefine((values, context) => {
      if (!values.start_time || !values.end_time) return
      if (values.end_time.getTime() >= values.start_time.getTime()) {
        return
      }
      context.addIssue({
        code: 'custom',
        path: ['end_time'],
        message: t('Invalid configuration'),
      })
    })
}

export type RiskRecordFilterFormValues = z.infer<
  ReturnType<typeof createRiskRecordFilterFormSchema>
>

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
  provider: 'Cloud review source',
  cache: 'Cache source',
  inflight: 'In-flight source',
  local: 'Local source',
}

const RISK_RECORD_CATEGORY_LABELS: Readonly<Record<string, string>> = {
  S1: 'Violent crimes',
  S2: 'Non-violent crimes',
  S3: 'Sex-related crimes',
  S4: 'Child sexual exploitation',
  S5: 'Defamation',
  S6: 'Specialized advice',
  S7: 'Privacy risk',
  S8: 'Intellectual property',
  S9: 'Indiscriminate weapons',
  S10: 'Hate',
  S11: 'Suicide and self-harm',
  S12: 'Sexual content',
  S13: 'Election misinformation',
  S14: 'Code interpreter abuse',
}

const RISK_RECORD_RESULT_FILTER_LABELS: Readonly<Record<string, string>> = {
  'all-results': 'All results',
  ...RISK_RECORD_RESULT_LABELS,
}

const RISK_RECORD_SOURCE_FILTER_LABELS: Readonly<Record<string, string>> = {
  'all-sources': 'All sources',
  ...RISK_RECORD_SOURCE_LABELS,
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

export function getRiskRecordCategoryLabel(category: string) {
  return RISK_RECORD_CATEGORY_LABELS[category.toUpperCase()]
}

export function getRiskRecordResultFilterLabel(result: string) {
  return RISK_RECORD_RESULT_FILTER_LABELS[result] ?? result
}

export function getRiskRecordSourceFilterLabel(source: string) {
  return RISK_RECORD_SOURCE_FILTER_LABELS[source] ?? source
}

export function getRiskRecordSourceVariant(source: string): StatusVariant {
  return RISK_RECORD_SOURCE_LABELS[source] ? 'info' : 'neutral'
}

export function getRiskRecordTotalPages(total: number, pageSize: number) {
  return Math.max(1, Math.ceil(total / pageSize))
}

function toUnixSeconds(value?: Date): number | undefined {
  if (!value) return undefined
  const timestamp = value.getTime()
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
  const username = draft.username.trim()
  const providerId = toPositiveInteger(draft.provider_id)

  return {
    ...(startTimestamp === undefined
      ? {}
      : { start_timestamp: startTimestamp }),
    ...(endTimestamp === undefined ? {} : { end_timestamp: endTimestamp }),
    ...(channelId === undefined ? {} : { channel_id: channelId }),
    ...(username ? { username } : {}),
    ...(providerId === undefined ? {} : { provider_id: providerId }),
    ...(draft.result ? { result: draft.result } : {}),
    ...(draft.source ? { source: draft.source } : {}),
  }
}

export function shouldRefetchRiskRecords(
  pageIndex: number,
  left: RiskRecordFilters,
  right: RiskRecordFilters
) {
  return (
    pageIndex === 0 &&
    left.start_timestamp === right.start_timestamp &&
    left.end_timestamp === right.end_timestamp &&
    left.channel_id === right.channel_id &&
    left.username === right.username &&
    left.provider_id === right.provider_id &&
    left.result === right.result &&
    left.source === right.source
  )
}

export function buildRiskRecordQueryParams(
  page: number,
  pageSize: number,
  filters: RiskRecordFilters
) {
  return { p: page, page_size: pageSize, ...filters }
}
