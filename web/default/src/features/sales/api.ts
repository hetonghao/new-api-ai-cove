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
  QuotaDataPoint,
  SalesDataParams,
  SalesUsersPage,
  SalesUsersParams,
} from './types'

export async function getSalesUsers(
  params: SalesUsersParams
): Promise<ApiResponse<SalesUsersPage>> {
  const res = await api.get('/api/sales/users', { params })
  return res.data
}

export async function getSalesData(
  params: SalesDataParams
): Promise<ApiResponse<QuotaDataPoint[]>> {
  const res = await api.get('/api/sales/data', { params })
  return res.data
}

export async function getSalesDataByUser(
  params: SalesDataParams
): Promise<ApiResponse<QuotaDataPoint[]>> {
  const res = await api.get('/api/sales/data/users', { params })
  return res.data
}

export async function getSalesGroups(): Promise<ApiResponse<string[]>> {
  const res = await api.get('/api/sales/groups')
  return res.data
}

export async function updateSalesUserGroup(
  userId: number,
  group: string
): Promise<ApiResponse> {
  const res = await api.patch(`/api/sales/users/${userId}/group`, { group })
  return res.data
}
