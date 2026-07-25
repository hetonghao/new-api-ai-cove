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
export const RISK_CHANNELS = ['cpa-pro'] as const
export const RISK_REVIEW_MODES = ['selective', 'full'] as const
export const RISK_ACTION_MODES = ['observe', 'block'] as const
export const LOCAL_RISK_RULE_TYPES = ['keyword', 'phrase', 'regex'] as const

export type RiskChannel = (typeof RISK_CHANNELS)[number]
export type RiskReviewMode = (typeof RISK_REVIEW_MODES)[number]
export type RiskActionMode = (typeof RISK_ACTION_MODES)[number]
export type LocalRiskRuleType = (typeof LOCAL_RISK_RULE_TYPES)[number]

export type RiskPolicy = {
  readonly configured: boolean
  readonly enabled: boolean
  readonly provider_id: number | null
  readonly enabled_channels: readonly RiskChannel[]
  readonly review_mode: RiskReviewMode
  readonly action_mode: RiskActionMode
}

export type RiskPolicyPayload = {
  readonly provider_id: number | null
  readonly enabled_channels: readonly RiskChannel[]
  readonly review_mode: RiskReviewMode
  readonly action_mode: RiskActionMode
}

export type LocalRiskRule = {
  readonly id: number
  readonly rule_type: LocalRiskRuleType
  readonly pattern: string
  readonly enabled: boolean
  readonly created_at: string
  readonly updated_at: string
}

export type LocalRiskRulePayload = {
  readonly rule_type: LocalRiskRuleType
  readonly pattern: string
  readonly enabled?: boolean
}

export type LocalRiskRuleTestPayload = {
  readonly rule_type: LocalRiskRuleType
  readonly pattern: string
  readonly text: string
}

export type LocalRiskRuleTestResult = {
  readonly normalized_text: string
  readonly matched: boolean
}

export type ApiResponse<T> = {
  readonly success: boolean
  readonly message: string
  readonly data?: T
}
