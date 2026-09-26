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
import DOMPurify from 'dompurify'
import type { TFunction } from 'i18next'

import type {
  ModelQualitySample,
  QualityBucket,
  QualityConfig,
  QualitySchedule,
} from '../types'

export type BucketTone =
  | 'failure'
  | 'mostly_failure'
  | 'mostly_success'
  | 'success'
  | 'empty'

// A bucket is red when every sample failed, amber/blue when mixed, green
// only on pure success, and stays empty otherwise; never paint empty green.
export function bucketTone(bucket: QualityBucket): BucketTone {
  if (bucket.failure > 0 && bucket.success === 0) return 'failure'
  if (bucket.failure > 0) {
    return bucket.failure >= bucket.success
      ? 'mostly_failure'
      : 'mostly_success'
  }
  if (bucket.success > 0) return 'success'
  return 'empty'
}

export function scheduleSummary(
  schedule: QualitySchedule,
  t: TFunction
): string {
  if (!schedule.enabled) return t('Schedule off')
  if (schedule.kind === 'interval') {
    return t('Every {{minutes}} min', { minutes: schedule.interval_minutes })
  }
  if (schedule.kind === 'daily') {
    return t('Daily at {{time}}', { time: schedule.time })
  }
  const dayNames = [
    t('Mon'),
    t('Tue'),
    t('Wed'),
    t('Thu'),
    t('Fri'),
    t('Sat'),
    t('Sun'),
  ]
  const days = [...schedule.weekdays]
    .sort((a, b) => a - b)
    .map((day) => dayNames[day - 1])
    .filter(Boolean)
    .join(', ')
  return t('Weekly {{days}} at {{time}}', {
    days,
    time: schedule.time,
  })
}

// Second defense layer for stored SVG: strict allowlist profile plus explicit
// scriptable-content bans. Never rendered as inline DOM — img src only.
export function sanitizeSvg(raw: string): string {
  return DOMPurify.sanitize(raw, {
    USE_PROFILES: { svg: true, svgFilters: true },
    FORBID_TAGS: ['foreignObject', 'script', 'a', 'image', 'iframe'],
  })
}

export function svgDataUri(sanitizedSvg: string): string {
  return `data:image/svg+xml;charset=utf-8,${encodeURIComponent(sanitizedSvg)}`
}

export function sampleEffectiveTime(sample: ModelQualitySample): number {
  return sample.started_at > 0 ? sample.started_at : sample.created_at
}

export function runTargetCount(config: QualityConfig): number {
  if (config.mode === 'channel') {
    return Math.max(config.channel_ids?.length ?? 0, 1)
  }
  return 1
}

export function runSampleTotal(config: QualityConfig): number {
  return runTargetCount(config) * config.samples_per_target
}

export function formatDurationMs(ms: number | null): string {
  if (ms == null) return '—'
  return `${(ms / 1000).toFixed(1)}s`
}

export function formatPercentileMs(
  ms: number | null,
  total: number,
  kind: 'p50' | 'p95',
  t: (key: string) => string
): string {
  if (ms != null) return `${(ms / 1000).toFixed(1)}s`
  if (kind === 'p95' && total > 0 && total < 20) {
    return t('Insufficient samples')
  }
  return '—'
}

const TERMINAL_STATUSES = new Set([
  'succeeded',
  'failed',
  'cancelled',
  'interrupted',
  'skipped',
])

export function isTerminalSample(sample: ModelQualitySample): boolean {
  return TERMINAL_STATUSES.has(sample.status)
}
