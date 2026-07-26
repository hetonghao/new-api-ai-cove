import { getChannels } from '@/features/channels/api'
import type { Channel } from '@/features/channels/types'
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
import { api } from '@/lib/api'

import type {
  ApiResponse,
  LocalRiskRule,
  LocalRiskRulePayload,
  LocalRiskRuleTestPayload,
  LocalRiskRuleTestResult,
  RiskPolicy,
  RiskPolicyPayload,
} from './types'

const POLICY_PATH = '/api/risk/policy'
const RULES_PATH = '/api/risk/rules'

export async function getRiskPolicyChannels(): Promise<readonly Channel[]> {
  const channels: Channel[] = []
  let page = 1
  while (true) {
    const response = await getChannels({ p: page, page_size: 100 })
    if (!response.success || !response.data) {
      throw new Error(response.message || 'Failed to load channels')
    }
    channels.push(...response.data.items)
    if (channels.length >= response.data.total) return channels
    page += 1
  }
}

export async function getRiskPolicy(): Promise<ApiResponse<RiskPolicy>> {
  const response = await api.get<ApiResponse<RiskPolicy>>(POLICY_PATH)
  return response.data
}

export async function updateRiskPolicy(
  payload: RiskPolicyPayload
): Promise<ApiResponse<RiskPolicy>> {
  const response = await api.put<ApiResponse<RiskPolicy>>(POLICY_PATH, payload)
  return response.data
}

export async function listLocalRiskRules(): Promise<
  ApiResponse<readonly LocalRiskRule[]>
> {
  const response =
    await api.get<ApiResponse<readonly LocalRiskRule[]>>(RULES_PATH)
  return response.data
}

export async function createLocalRiskRule(
  payload: LocalRiskRulePayload
): Promise<ApiResponse<LocalRiskRule>> {
  const response = await api.post<ApiResponse<LocalRiskRule>>(
    RULES_PATH,
    payload
  )
  return response.data
}

export async function updateLocalRiskRule(
  ruleId: number,
  payload: LocalRiskRulePayload
): Promise<ApiResponse<LocalRiskRule>> {
  const response = await api.put<ApiResponse<LocalRiskRule>>(
    `${RULES_PATH}/${ruleId}`,
    payload
  )
  return response.data
}

export async function deleteLocalRiskRule(
  ruleId: number
): Promise<ApiResponse<null>> {
  const response = await api.delete<ApiResponse<null>>(
    `${RULES_PATH}/${ruleId}`
  )
  return response.data
}

export async function testLocalRiskRule(
  payload: LocalRiskRuleTestPayload
): Promise<ApiResponse<LocalRiskRuleTestResult>> {
  const response = await api.post<ApiResponse<LocalRiskRuleTestResult>>(
    `${RULES_PATH}/test`,
    payload
  )
  return response.data
}
