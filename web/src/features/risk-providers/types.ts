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
export type RiskProviderType = 'cloudflare' | 'platform_internal'

export type RiskProvider = {
  readonly id: number
  readonly name: string
  readonly provider_type: RiskProviderType
  readonly account_id: string
  readonly channel_id: number
  readonly model: string
  readonly has_credential: boolean
  readonly system_managed: boolean
  readonly timeout_ms: number
  readonly failure_threshold: number
  readonly cooldown_seconds: number
  readonly priority: number
  readonly daily_neurons_limit: number
  readonly daily_reset_time: string
  readonly current_status: 'normal' | 'circuit_open' | 'daily_exhausted'
  readonly daily_neurons_used: number
  readonly daily_neurons_reserved: number
  readonly daily_neurons_remaining: number
  readonly daily_neurons_reset_at: string | null
  readonly validated_at: string | null
  readonly active: boolean
  readonly created_at: string
  readonly updated_at: string
}

export type RiskProviderFormValues = {
  readonly name: string
  readonly provider_type: RiskProviderType
  readonly account_id: string
  readonly channel_id: number | null
  readonly model: string
  readonly credential: string
  readonly timeout_ms: number
  readonly failure_threshold: number
  readonly cooldown_seconds: number
  readonly priority: number
  readonly daily_neurons_limit: number
  readonly daily_reset_time: string
}

export type RiskProviderPayload = {
  readonly name: string
  readonly provider_type: RiskProviderType
  readonly account_id?: string
  readonly channel_id?: number
  readonly model: string
  readonly credential?: string
  readonly timeout_ms: number
  readonly failure_threshold: number
  readonly cooldown_seconds: number
  readonly priority: number
  readonly daily_neurons_limit: number
  readonly daily_reset_time: string
}

export type RiskProviderValidation = {
  readonly status: 'safe' | 'unsafe'
  readonly categories: readonly string[]
  readonly usage: {
    readonly prompt_tokens: number
    readonly completion_tokens: number
    readonly total_tokens: number
  }
}

export type RiskProviderValidationPayload = {
  readonly text: string
}

export type ApiResponse<T> = {
  readonly success: boolean
  readonly message: string
  readonly data?: T
}

export type RiskStatisticsGranularity = 'hour' | 'day' | 'week'

export type RiskStatisticsSummary = {
  readonly records: number
  readonly affected_users: number
  readonly unsafe: number
  readonly unsafe_rate: number
  readonly blocked: number
  readonly blocked_rate: number
  readonly errors: number
  readonly error_rate: number
  readonly cache_hits: number
  readonly cache_hit_rate: number
  readonly provider_calls: number
  readonly neurons: number
  readonly p95_latency_ms: number
}

export type RiskStatisticsUser = {
  readonly user_id: number
  readonly username: string
  readonly safe: number
  readonly unsafe: number
  readonly errors: number
  readonly not_reviewed: number
  readonly total: number
}

export type RiskStatisticsChannel = {
  readonly channel_id: number
  readonly channel_name: string
  readonly safe: number
  readonly unsafe: number
  readonly errors: number
  readonly total: number
}

export type RiskStatisticsSourceBucket = {
  readonly bucket_start: number
  readonly provider: number
  readonly cache: number
  readonly inflight: number
  readonly local: number
  readonly total: number
}

export type RiskStatistics = {
  readonly start_timestamp: number
  readonly end_timestamp: number
  readonly granularity: RiskStatisticsGranularity
  readonly summary: RiskStatisticsSummary
  readonly users: readonly RiskStatisticsUser[]
  readonly channels: readonly RiskStatisticsChannel[]
  readonly source_trend: readonly RiskStatisticsSourceBucket[]
}
