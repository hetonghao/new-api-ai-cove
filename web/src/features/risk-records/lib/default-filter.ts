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
import { getDefaultTimeRange } from '@/features/usage-logs/lib/utils'

import type { RiskRecordFilters } from '../types'
import type { RiskRecordFilterFormValues } from './risk-records'

export function createDefaultRiskRecordFilterDraft(): RiskRecordFilterFormValues {
  const { start, end } = getDefaultTimeRange()
  return {
    start_time: start,
    end_time: end,
    channel_id: '',
    user_id: '',
    username: '',
    provider_id: '',
    provider_type: '',
    result: '',
    source: '',
  }
}

export function createRiskRecordFilterDraftFromFilters(
  filters: RiskRecordFilters
): RiskRecordFilterFormValues {
  const defaults = createDefaultRiskRecordFilterDraft()
  const source = filters.source
  const validSource =
    source === 'provider' ||
    source === 'cache' ||
    source === 'inflight' ||
    source === 'local'
      ? source
      : ''
  return {
    ...defaults,
    start_time:
      filters.start_timestamp === undefined
        ? defaults.start_time
        : new Date(filters.start_timestamp * 1000),
    end_time:
      filters.end_timestamp === undefined
        ? defaults.end_time
        : new Date(filters.end_timestamp * 1000),
    channel_id:
      filters.channel_id === undefined ? '' : String(filters.channel_id),
    user_id: filters.user_id === undefined ? '' : String(filters.user_id),
    username: filters.username ?? '',
    source: validSource,
  }
}
