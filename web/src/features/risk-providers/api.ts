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
  RiskProvider,
  RiskProviderPayload,
  RiskProviderValidation,
  RiskProviderValidationPayload,
} from './types'

const BASE_PATH = '/api/risk/providers'

export async function listRiskProviders(): Promise<
  ApiResponse<readonly RiskProvider[]>
> {
  const response =
    await api.get<ApiResponse<readonly RiskProvider[]>>(BASE_PATH)
  return response.data
}

export async function createRiskProvider(
  payload: RiskProviderPayload
): Promise<ApiResponse<RiskProvider>> {
  const response = await api.post<ApiResponse<RiskProvider>>(BASE_PATH, payload)
  return response.data
}

export async function updateRiskProvider(
  providerId: number,
  payload: RiskProviderPayload
): Promise<ApiResponse<RiskProvider>> {
  const response = await api.put<ApiResponse<RiskProvider>>(
    `${BASE_PATH}/${providerId}`,
    payload
  )
  return response.data
}

export async function deleteRiskProvider(
  providerId: number
): Promise<ApiResponse<null>> {
  const response = await api.delete<ApiResponse<null>>(
    `${BASE_PATH}/${providerId}`
  )
  return response.data
}

export async function validateRiskProvider(
  providerId: number,
  payload: RiskProviderValidationPayload
): Promise<ApiResponse<RiskProviderValidation>> {
  const response = await api.post<ApiResponse<RiskProviderValidation>>(
    `${BASE_PATH}/${providerId}/validate`,
    payload,
    {
      skipBusinessError: true,
      skipErrorHandler: true,
    }
  )
  return response.data
}

export async function activateRiskProvider(
  providerId: number
): Promise<ApiResponse<RiskProvider>> {
  const response = await api.put<ApiResponse<RiskProvider>>(
    `${BASE_PATH}/${providerId}/active`
  )
  return response.data
}
