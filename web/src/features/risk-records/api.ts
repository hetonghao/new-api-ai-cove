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

import {
  usageLogSchema,
  type UsageLog,
} from '@/features/usage-logs/data/schema'
import { api } from '@/lib/api'

import { buildRiskRecordQueryParams } from './lib/risk-records'
import {
  riskRecordGovernanceResponseSchema,
  riskRecordResponseSchema,
  type RiskRecordGovernance,
  type RiskRecordGovernanceResponse,
  type RiskRecordFilters,
  type RiskRecordResponse,
} from './types'

const BASE_PATH = '/api/risk/records'
const SETTINGS_PATH = `${BASE_PATH}/settings`
const requestLogResponseSchema = z
  .object({
    success: z.boolean(),
    message: z.string(),
    data: usageLogSchema.nullable(),
  })
  .readonly()

export class RiskRecordResponseError extends Error {
  readonly name = 'RiskRecordResponseError'
}

export async function listRiskRecords(
  page: number,
  pageSize: number,
  filters: RiskRecordFilters
): Promise<RiskRecordResponse> {
  const response = await api.get<unknown>(BASE_PATH, {
    params: buildRiskRecordQueryParams(page, pageSize, filters),
    skipErrorHandler: true,
  })
  const parsed = riskRecordResponseSchema.safeParse(response.data)
  if (!parsed.success) {
    throw new RiskRecordResponseError('Invalid risk record response')
  }
  return parsed.data
}

export async function getRiskRequestLog(requestId: string): Promise<{
  readonly success: boolean
  readonly message: string
  readonly data: UsageLog | null
}> {
  const response = await api.get<unknown>(
    `/api/log/request/${encodeURIComponent(requestId)}`,
    { skipErrorHandler: true }
  )
  const parsed = requestLogResponseSchema.safeParse(response.data)
  if (!parsed.success) {
    throw new RiskRecordResponseError('Invalid request log response')
  }
  return parsed.data
}

export async function getRiskRecordGovernance(): Promise<RiskRecordGovernanceResponse> {
  const response = await api.get<unknown>(SETTINGS_PATH, {
    skipErrorHandler: true,
  })
  const parsed = riskRecordGovernanceResponseSchema.safeParse(response.data)
  if (!parsed.success) {
    throw new RiskRecordResponseError('Invalid risk record settings response')
  }
  return parsed.data
}

export async function updateRiskRecordGovernance(
  payload: RiskRecordGovernance
): Promise<RiskRecordGovernanceResponse> {
  const response = await api.put<unknown>(SETTINGS_PATH, payload, {
    skipErrorHandler: true,
  })
  const parsed = riskRecordGovernanceResponseSchema.safeParse(response.data)
  if (!parsed.success) {
    throw new RiskRecordResponseError('Invalid risk record settings response')
  }
  return parsed.data
}
