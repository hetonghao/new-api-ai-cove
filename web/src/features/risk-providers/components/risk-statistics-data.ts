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
*/
import type { TFunction } from 'i18next'

import type {
  RiskStatisticsGranularity,
  RiskStatisticsSourceBucket,
} from '../types'

export const SOURCE_KEYS = ['provider', 'cache', 'inflight', 'local'] as const
export const SOURCE_COLORS = {
  provider: 'var(--info)',
  cache: 'var(--chart-2)',
  inflight: 'var(--warning)',
  local: 'var(--neutral)',
} as const

export type SourceKey = (typeof SOURCE_KEYS)[number]

export type SourceChartRow = RiskStatisticsSourceBucket &
  Record<`${SourceKey}_pct`, number> & {
    label: string
  }

export function formatBucketLabel(
  timestamp: number,
  granularity: RiskStatisticsGranularity
) {
  const options: Intl.DateTimeFormatOptions =
    granularity === 'hour'
      ? {
          month: '2-digit',
          day: '2-digit',
          hour: '2-digit',
          minute: '2-digit',
          hour12: false,
        }
      : { month: '2-digit', day: '2-digit' }
  return new Intl.DateTimeFormat(undefined, {
    ...options,
    timeZone: 'Asia/Shanghai',
  }).format(new Date(timestamp * 1000))
}

export function toSourceChartRows(
  buckets: readonly RiskStatisticsSourceBucket[],
  granularity: RiskStatisticsGranularity
): SourceChartRow[] {
  return buckets.map((bucket) => {
    const total =
      bucket.total ||
      bucket.provider + bucket.cache + bucket.inflight + bucket.local
    const percentage = (value: number) => (total > 0 ? value / total : 0)
    return {
      ...bucket,
      total,
      label: formatBucketLabel(bucket.bucket_start, granularity),
      provider_pct: percentage(bucket.provider),
      cache_pct: percentage(bucket.cache),
      inflight_pct: percentage(bucket.inflight),
      local_pct: percentage(bucket.local),
    }
  })
}

export function sourceLabel(t: TFunction, key: SourceKey) {
  if (key === 'provider') return t('Cloud review source')
  if (key === 'cache') return t('Cache source')
  if (key === 'inflight') return t('In-flight source')
  return t('Local source')
}
