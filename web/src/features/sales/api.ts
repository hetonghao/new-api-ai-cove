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
  SalesCommissionAdminPage,
  SalesCommissionAdminParams,
  SalesCommissionSettlementPage,
  SalesDataParams,
  SalesStats,
  SalesUserInfo,
  SalesUsersPage,
  SalesUsersParams,
} from './types'

export async function getSalesUsers(
  params: SalesUsersParams
): Promise<ApiResponse<SalesUsersPage>> {
  const res = await api.get('/api/sales/users', { params })
  return res.data
}

export async function getSalesUserInfo(
  userId: number
): Promise<ApiResponse<SalesUserInfo>> {
  const res = await api.get(`/api/sales/users/${userId}`)
  return res.data
}

export async function getSalesData(
  params: SalesDataParams
): Promise<ApiResponse<QuotaDataPoint[]>> {
  const res = await api.get('/api/sales/data', { params })
  return res.data
}

export async function getSalesStats(): Promise<ApiResponse<SalesStats>> {
  const res = await api.get('/api/sales/stats', {
    params: { _t: Date.now() },
  })
  return res.data
}

export async function getSalesCommissionSettlements(params: {
  p?: number
  page_size?: number
}): Promise<ApiResponse<SalesCommissionSettlementPage>> {
  const res = await api.get('/api/sales/commission/settlements', { params })
  return res.data
}

export async function getSalesCommissionAdminRows(
  params: SalesCommissionAdminParams
): Promise<ApiResponse<SalesCommissionAdminPage>> {
  const res = await api.get('/api/sales/admin/commissions', { params })
  return res.data
}

export async function updateSalesCommissionRatio(
  salesUserId: number,
  commissionRatio: number
): Promise<ApiResponse> {
  const res = await api.patch(
    `/api/sales/admin/commissions/${salesUserId}/ratio`,
    { commission_ratio: commissionRatio }
  )
  return res.data
}

export async function createSalesCommissionSettlement(
  salesUserId: number,
  payload: { amount: number; note?: string }
): Promise<ApiResponse> {
  const res = await api.post(
    `/api/sales/admin/commissions/${salesUserId}/settlements`,
    payload
  )
  return res.data
}

export async function getSalesCommissionSettlementsByRoot(
  salesUserId: number,
  params: { p?: number; page_size?: number }
): Promise<ApiResponse<SalesCommissionSettlementPage>> {
  const res = await api.get(
    `/api/sales/admin/commissions/${salesUserId}/settlements`,
    { params }
  )
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
