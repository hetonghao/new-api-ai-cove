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
  ModelQualityRun,
  ModelQualitySample,
  QualityArtifact,
  QualityCapabilities,
  QualityCaseView,
  QualityCaseWrite,
  QualityDashboardData,
  QualityOutputContract,
  QualityRunRequest,
  QualitySettingsConfig,
  QualitySettingsResponse,
} from './types'

const BASE = '/api/model-quality'

export async function getQualityCapabilities(): Promise<
  ApiResponse<QualityCapabilities>
> {
  const res = await api.get<ApiResponse<QualityCapabilities>>(
    `${BASE}/capabilities`
  )
  return res.data
}

export async function getQualityCases(): Promise<
  ApiResponse<QualityCaseView[]>
> {
  const res = await api.get<ApiResponse<QualityCaseView[]>>(`${BASE}/cases`)
  return res.data
}

export async function getQualityCase(
  id: number
): Promise<ApiResponse<QualityCaseView>> {
  const res = await api.get<ApiResponse<QualityCaseView>>(`${BASE}/cases/${id}`)
  return res.data
}

export async function getQualityOutputContract(): Promise<
  ApiResponse<QualityOutputContract>
> {
  const res = await api.get<ApiResponse<QualityOutputContract>>(
    `${BASE}/output-contract`
  )
  return res.data
}

export async function createQualityCase(
  body: QualityCaseWrite
): Promise<ApiResponse<QualityCaseView>> {
  const res = await api.post<ApiResponse<QualityCaseView>>(
    `${BASE}/cases`,
    body
  )
  return res.data
}

export async function updateQualityCase(
  id: number,
  body: QualityCaseWrite
): Promise<ApiResponse<QualityCaseView>> {
  const res = await api.put<ApiResponse<QualityCaseView>>(
    `${BASE}/cases/${id}`,
    body
  )
  return res.data
}

export async function reorderQualityCases(
  ids: number[]
): Promise<ApiResponse<null>> {
  const res = await api.put<ApiResponse<null>>(`${BASE}/cases/order`, { ids })
  return res.data
}

export async function deleteQualityCase(
  id: number,
  expectedEditVersion: number
): Promise<ApiResponse<null>> {
  const res = await api.delete<ApiResponse<null>>(`${BASE}/cases/${id}`, {
    params: { expected_edit_version: expectedEditVersion },
  })
  return res.data
}

export async function createQualityRun(
  caseId: number,
  body: QualityRunRequest,
  idempotencyKey: string
): Promise<
  ApiResponse<{ run: ModelQualityRun; newly_created: boolean }>
> {
  const res = await api.post<
    ApiResponse<{ run: ModelQualityRun; newly_created: boolean }>
  >(`${BASE}/cases/${caseId}/runs`, body, {
    headers: { 'Idempotency-Key': idempotencyKey },
  })
  return res.data
}

export type QualityRunsPage = {
  items: ModelQualityRun[]
  total: number
  page: number
  page_size: number
}

export async function getQualityRuns(params: {
  p?: number
  page_size?: number
  case_id?: number
  source?: string
  status?: string
  start?: number
  end?: number
}): Promise<ApiResponse<QualityRunsPage>> {
  const res = await api.get<ApiResponse<QualityRunsPage>>(`${BASE}/runs`, {
    params,
  })
  return res.data
}

export async function getQualityRun(
  id: string
): Promise<ApiResponse<{ run: ModelQualityRun; samples: ModelQualitySample[] }>> {
  const res = await api.get<
    ApiResponse<{ run: ModelQualityRun; samples: ModelQualitySample[] }>
  >(`${BASE}/runs/${id}`)
  return res.data
}

export async function cancelQualityRun(
  id: string
): Promise<ApiResponse<null>> {
  const res = await api.post<ApiResponse<null>>(`${BASE}/runs/${id}/cancel`)
  return res.data
}

export async function getQualityDashboard(params: {
  caseId: number
  version?: number
  channel_id?: number
}): Promise<ApiResponse<QualityDashboardData>> {
  const { caseId, ...query } = params
  const res = await api.get<ApiResponse<QualityDashboardData>>(
    `${BASE}/cases/${caseId}/dashboard`,
    { params: query }
  )
  return res.data
}

export async function getQualitySamples(params: {
  caseId: number
  version?: number
  channel_id: number
  from_ms?: number
  to_ms?: number
  before_ms?: number
  before_id?: number
  limit?: number
}): Promise<ApiResponse<ModelQualitySample[]>> {
  const { caseId, ...query } = params
  const res = await api.get<ApiResponse<ModelQualitySample[]>>(
    `${BASE}/cases/${caseId}/samples`,
    { params: query }
  )
  return res.data
}

export async function getQualityArtifact(
  sampleId: number
): Promise<ApiResponse<QualityArtifact>> {
  const res = await api.get<ApiResponse<QualityArtifact>>(
    `${BASE}/samples/${sampleId}/artifact`
  )
  return res.data
}

export async function annotateQualitySample(
  sampleId: number,
  body: { tag: string; note: string; pinned: boolean }
): Promise<ApiResponse<null>> {
  const res = await api.put<ApiResponse<null>>(
    `${BASE}/samples/${sampleId}/annotation`,
    body
  )
  return res.data
}

export async function getQualitySettings(): Promise<
  ApiResponse<QualitySettingsResponse>
> {
  const res = await api.get<ApiResponse<QualitySettingsResponse>>(
    `${BASE}/settings`
  )
  return res.data
}

export async function updateQualitySettings(body: {
  config: QualitySettingsConfig
  expected_version: number
}): Promise<ApiResponse<null>> {
  const res = await api.put<ApiResponse<null>>(`${BASE}/settings`, body)
  return res.data
}
