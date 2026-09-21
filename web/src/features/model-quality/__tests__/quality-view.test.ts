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
import type { TFunction } from 'i18next'
import { describe, expect, test } from 'vitest'

import {
  bucketTone,
  formatPercentileMs,
  runTargetCount,
  sanitizeSvg,
  scheduleSummary,
} from '../lib/quality-view'
import type { QualityBucket, QualityConfig, QualitySchedule } from '../types'

function bucket(partial: Partial<QualityBucket>): QualityBucket {
  return { start: 0, end: 0, success: 0, failure: 0, latest_at: 0, ...partial }
}

describe('bucketTone', () => {
  test('a failure-only bucket is red', () => {
    expect(bucketTone(bucket({ failure: 2 }))).toBe('failure')
  })
  test('a mixed bucket with failures >= successes is warning', () => {
    expect(bucketTone(bucket({ success: 2, failure: 2 }))).toBe(
      'mostly_failure'
    )
    expect(bucketTone(bucket({ success: 1, failure: 3 }))).toBe(
      'mostly_failure'
    )
  })
  test('a mixed bucket dominated by successes is info', () => {
    expect(bucketTone(bucket({ success: 3, failure: 1 }))).toBe(
      'mostly_success'
    )
  })
  test('a success-only bucket is green', () => {
    expect(bucketTone(bucket({ success: 2 }))).toBe('success')
  })
  test('an empty bucket is neither red nor green', () => {
    expect(bucketTone(bucket({}))).toBe('empty')
  })
})

describe('sanitizeSvg', () => {
  test('removes script elements and event handlers, keeps shapes', () => {
    const dirty =
      '<svg viewBox="0 0 10 10" onload="alert(1)"><script>alert(2)</script><rect width="10" height="10"/></svg>'
    const sanitized = sanitizeSvg(dirty)
    expect(sanitized).not.toContain('script')
    expect(sanitized).not.toContain('onload')
    expect(sanitized).toContain('rect')
  })
  test('drops forbidden foreignObject content', () => {
    const dirty = '<svg><foreignObject><div>x</div></foreignObject></svg>'
    expect(sanitizeSvg(dirty)).not.toContain('foreignObject')
  })
})

describe('runTargetCount', () => {
  const base: QualityConfig = {
    model: 'm',
    output_type: 'svg',
    mode: 'route',
    protocol: 'responses',
    token_id: 1,
    group: '',
    channel_ids: [],
    prompt: 'p',
    instruction: '',
    instruction_role: '',
    max_output_tokens: 16384,
    reasoning_effort: '',
    samples_per_target: 2,
    timeout_seconds: 180,
    daily_limit: 50,
  }
  test('route mode always targets the gateway route once', () => {
    expect(runTargetCount(base)).toBe(1)
  })
  test('channel mode counts pinned channels', () => {
    expect(
      runTargetCount({ ...base, mode: 'channel', channel_ids: [1, 2, 3] })
    ).toBe(3)
  })
})

describe('formatPercentileMs', () => {
  const t = ((key: string) => key) as TFunction
  test('p50 null shows a plain dash', () => {
    expect(formatPercentileMs(null, 5, 'p50', t)).toBe('—')
  })
  test('p95 null with few samples explains insufficiency', () => {
    expect(formatPercentileMs(null, 5, 'p95', t)).toBe('Insufficient samples')
  })
  test('p95 null with zero samples shows a dash', () => {
    expect(formatPercentileMs(null, 0, 'p95', t)).toBe('—')
  })
  test('a real p95 formats as seconds', () => {
    expect(formatPercentileMs(1234, 50, 'p95', t)).toBe('1.2s')
  })
})

describe('scheduleSummary', () => {
  const t = ((key: string) => key) as TFunction
  const schedule: QualitySchedule = {
    enabled: false,
    kind: 'interval',
    interval_minutes: 60,
    time: '09:00',
    weekdays: [],
    timezone: 'UTC',
  }
  test('disabled schedules report as off', () => {
    expect(scheduleSummary(schedule, t)).toBe('Schedule off')
  })
  test('interval schedules include the minutes', () => {
    expect(
      scheduleSummary({ ...schedule, enabled: true }, t)
    ).toBe('Every {{minutes}} min')
  })
})
