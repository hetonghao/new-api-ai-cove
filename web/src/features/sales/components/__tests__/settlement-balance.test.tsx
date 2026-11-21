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
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { useState } from 'react'
import { expect, it, vi } from 'vitest'

import { api } from '@/lib/api'

import { SettlementDialog } from '../commission-settlements'

const salesperson = {
  sales_user_id: 42,
  username: 'seller',
  commission_ratio: 10,
  total_revenue: 100,
  settled_commission_amount: 0,
  settled_commission_revenue: 0,
  pending_commission_revenue: 100,
  pending_commission_amount: 10,
  total_commission_amount: 10,
  last_settlement_created_at: 0,
}

it('credits the submitted settlement only when opted in, and clears the choice when reopened', async () => {
  vi.spyOn(api, 'get').mockResolvedValue({
    data: { success: true, data: { items: [] } },
  })
  const onSubmit = vi.fn()
  const user = userEvent.setup()
  const client = new QueryClient()
  const checkboxName = 'Credit this settlement to the account balance'

  function Harness() {
    const [open, setOpen] = useState(true)
    return (
      <>
        <button type='button' onClick={() => setOpen(true)}>
          Reopen
        </button>
        <SettlementDialog
          row={open ? salesperson : null}
          open={open}
          mode='settle'
          isSubmitting={false}
          onOpenChange={setOpen}
          onSubmit={(payload) => {
            onSubmit(payload)
            setOpen(false)
          }}
        />
      </>
    )
  }

  const view = render(
    <QueryClientProvider client={client}>
      <Harness />
    </QueryClientProvider>
  )
  try {
    const checkbox = await screen.findByRole('checkbox', { name: checkboxName })
    expect(checkbox).not.toBeChecked()
    await user.click(screen.getByText(checkboxName))
    expect(checkbox).toBeChecked()
    await user.click(screen.getByRole('button', { name: 'Create Settlement' }))
    expect(onSubmit).toHaveBeenLastCalledWith(
      expect.objectContaining({ salesUserId: 42, creditToBalance: true })
    )

    await user.click(await screen.findByRole('button', { name: 'Reopen' }))
    expect(
      await screen.findByRole('checkbox', { name: checkboxName })
    ).not.toBeChecked()
    await user.click(screen.getByRole('button', { name: 'Create Settlement' }))
    expect(onSubmit).toHaveBeenLastCalledWith(
      expect.objectContaining({ salesUserId: 42, creditToBalance: false })
    )
  } finally {
    view.unmount()
    client.clear()
  }
})
