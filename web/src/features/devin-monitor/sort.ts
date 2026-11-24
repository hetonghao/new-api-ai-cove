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

import type { DeviMonRouting } from './types'

export type DevinSort = {
  field: 'priority' | 'reset' | 'renewal'
  descending: boolean
}

export const DEFAULT_DEVIN_SORT: DevinSort = {
  field: 'priority',
  descending: true,
}

type SortableAccount = {
  email: string
  devin_routing?: DeviMonRouting | null
  reset_at?: number | null
  renew_at?: string
}

export function sortDevinAccounts<T extends SortableAccount>(
  accounts: readonly T[],
  sort: DevinSort
): T[] {
  const direction = sort.descending ? -1 : 1
  const ranked = accounts.map((account) => {
    let primary: number | null = null
    let weight = 0
    if (sort.field === 'priority' && account.devin_routing) {
      primary = account.devin_routing.priority ?? 0
      weight = account.devin_routing.weight ?? 1
    } else if (
      sort.field === 'reset' &&
      account.reset_at &&
      account.reset_at > 0
    ) {
      primary = account.reset_at
    } else if (sort.field === 'renewal' && account.renew_at) {
      const renewal = Date.parse(account.renew_at)
      if (Number.isFinite(renewal)) primary = renewal
    }
    return { account, primary, weight }
  })
  ranked.sort((a, b) => {
    if (a.primary === null && b.primary !== null) return 1
    if (a.primary !== null && b.primary === null) return -1
    if (a.primary !== null && b.primary !== null) {
      const order = a.primary - b.primary || a.weight - b.weight
      if (order) return direction * order
    }
    return a.account.email.localeCompare(b.account.email)
  })
  return ranked.map(({ account }) => account)
}
