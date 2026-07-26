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
export type RiskProviderType = 'cloudflare'

export type RiskProvider = {
  readonly id: number
  readonly name: string
  readonly provider_type: RiskProviderType
  readonly account_id: string
  readonly model: string
  readonly has_credential: boolean
  readonly timeout_ms: number
  readonly failure_threshold: number
  readonly cooldown_seconds: number
  readonly validated_at: string | null
  readonly active: boolean
  readonly created_at: string
  readonly updated_at: string
}

export type RiskProviderFormValues = {
  readonly name: string
  readonly provider_type: RiskProviderType
  readonly account_id: string
  readonly model: string
  readonly credential: string
  readonly timeout_ms: number
  readonly failure_threshold: number
  readonly cooldown_seconds: number
}

export type RiskProviderPayload = Omit<RiskProviderFormValues, 'credential'> & {
  readonly credential?: string
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

export type ApiResponse<T> = {
  readonly success: boolean
  readonly message: string
  readonly data?: T
}
