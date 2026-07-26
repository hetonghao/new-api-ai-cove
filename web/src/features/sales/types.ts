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
import type { UserInfo } from '@/features/usage-logs/types'
import type { User } from '@/features/users/types'

export type SalesUser = User & {
  topup_amount?: number
}

export interface ApiResponse<T = unknown> {
  success: boolean
  message?: string
  data?: T
}

export interface SalesUsersParams {
  keyword?: string
  group?: string
  p?: number
  page_size?: number
}

export interface SalesUsersPage {
  items: SalesUser[]
  total: number
  page: number
  page_size: number
}

export interface QuotaDataPoint {
  model_name?: string
  username?: string
  created_at: number
  count: number
  quota: number
  token_used: number
}

export interface SalesDataParams {
  start_timestamp?: number
  end_timestamp?: number
}

export interface SalesStats {
  topup_amount: number
  commission_ratio: number
  settled_commission_amount: number
  settled_commission_revenue: number
  pending_commission_revenue: number
  pending_commission_amount: number
  total_commission_amount: number
  last_settlement_created_at: number
}

export type SalesUserInfo = UserInfo

export interface SalesCommissionSettlement {
  id: number
  sales_user_id: number
  operator_user_id: number
  amount: number
  commission_ratio: number
  covered_revenue: number
  note: string
  created_at: number
}

export interface SalesCommissionSettlementPage {
  items: SalesCommissionSettlement[]
  total: number
  page: number
  page_size: number
}

export interface SalesCommissionAdminParams {
  keyword?: string
  p?: number
  page_size?: number
}

export interface SalesCommissionAdminRow {
  sales_user_id: number
  username: string
  email?: string
  display_name?: string
  commission_ratio: number
  total_revenue: number
  settled_commission_amount: number
  settled_commission_revenue: number
  pending_commission_revenue: number
  pending_commission_amount: number
  total_commission_amount: number
  last_settlement_created_at: number
}

export interface SalesCommissionAdminPage {
  items: SalesCommissionAdminRow[]
  total: number
  page: number
  page_size: number
}
