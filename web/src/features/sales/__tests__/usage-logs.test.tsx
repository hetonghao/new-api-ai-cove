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
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import {
  createMemoryHistory,
  createRootRoute,
  createRoute,
  createRouter,
  RouterProvider,
} from '@tanstack/react-router'
import { cleanup, render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, expect, it, vi } from 'vitest'

import { usageLogSchema } from '@/features/usage-logs/data/schema'
import { api } from '@/lib/api'
import { ROLE } from '@/lib/roles'
import { useAuthStore } from '@/stores/auth-store'

import { Sales } from '..'

afterEach(() => {
  cleanup()
  useAuthStore.setState(useAuthStore.getInitialState(), true)
  localStorage.clear()
})

it.each([
  { role: ROLE.SALES, empty: true },
  { role: ROLE.SALES, empty: false },
  { role: ROLE.SUPER_ADMIN, empty: false },
])(
  'opens invited-user logs without the general log export for role $role (empty=$empty)',
  async ({ role, empty }) => {
    useAuthStore.getState().auth.setUser({ id: 7, username: 'sales', role })
    const records = empty
      ? []
      : [
          usageLogSchema.parse({
            id: 1,
            user_id: 42,
            username: 'invitee-42',
            created_at: 1793491200,
            type: 2,
            content: '',
            model_name: 'gpt-4o',
          }),
        ]
    vi.spyOn(api, 'get').mockImplementation(async (url) => {
      let data: unknown = {}
      if (url === '/api/sales/users') {
        data = { items: [], total: 0 }
      } else if (url.startsWith('/api/sales/logs?')) {
        data = { items: records, total: records.length }
      } else if (
        [
          '/api/sales/data',
          '/api/sales/groups',
          '/api/group/',
          '/api/subscription/admin/plans',
        ].includes(url)
      ) {
        data = []
      } else if (url === '/api/subscription/self') {
        data = { subscriptions: [], all_subscriptions: [] }
      } else if (url === '/api/status') {
        data = { log_export_enabled: true }
      }
      return { data: { success: true, data } }
    })
    const root = createRootRoute()
    const auth = createRoute({
      getParentRoute: () => root,
      id: '_authenticated',
    })
    const sales = createRoute({
      getParentRoute: () => auth,
      path: '/sales/',
      component: Sales,
    })
    const router = createRouter({
      routeTree: root.addChildren([auth.addChildren([sales])]),
      history: createMemoryHistory({ initialEntries: ['/sales/'] }),
    })
    const client = new QueryClient({
      defaultOptions: { queries: { retry: false } },
    })
    const user = userEvent.setup()
    render(
      <QueryClientProvider client={client}>
        <RouterProvider router={router} />
      </QueryClientProvider>
    )

    await user.click(
      await screen.findByRole('tab', { name: 'Invited User Usage Logs' })
    )

    expect(
      await screen.findByText(empty ? 'No Logs Found' : 'invitee-42')
    ).toBeVisible()
    expect(
      screen.queryByRole('button', { name: 'Export' })
    ).not.toBeInTheDocument()
    expect(api.get).toHaveBeenCalledWith(
      expect.stringMatching(/^\/api\/sales\/logs\?/)
    )
    client.clear()
  }
)
