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

import { useMutation, useQueryClient } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { Switch } from '@/components/ui/switch'

import { deviMonQueryKey, setDeviMonDevinDisabled } from '../api'
import type { DeviMonState } from '../types'

// Rows render the last collection, so the switch must both repaint immediately
// and store a fresh reading: without the reading the next poll restores the
// state CPA had when the collection ran, which looks like a dead switch.
function withDeviMonDisabled(
  state: DeviMonState | undefined,
  email: string,
  disabled: boolean
): DeviMonState | undefined {
  const snapshot = state?.snapshot
  if (!snapshot) return state
  const sameRow = (row: { email: string }) =>
    row.email.toLowerCase() === email.toLowerCase()
  return {
    ...state,
    snapshot: {
      ...snapshot,
      accounts: snapshot.accounts.map((row) =>
        sameRow(row) ? { ...row, devin_disabled: disabled } : row
      ),
      unpaired_devin: snapshot.unpaired_devin.map((row) =>
        sameRow(row) ? { ...row, disabled } : row
      ),
    },
  }
}

// Mirrors cpa-pro-1's enable/disable switch. It flips the Devin credential in
// CPA, which is the same state the row's badge shows; the reading channel is
// unrelated, so every row keeps this switch.
export function DevinAccountToggle(props: {
  readonly email: string
  readonly disabled: boolean | null
  readonly configured: boolean
}) {
  const { t } = useTranslation()
  const client = useQueryClient()
  // Checked means the Devin credential is enabled in CPA.
  const enabled = props.disabled === false
  const label = enabled
    ? t('Disable {{email}}', { email: props.email })
    : t('Enable {{email}}', { email: props.email })
  const toggle = useMutation({
    mutationFn: () => setDeviMonDevinDisabled(props.email, enabled),
    onMutate: async () => {
      await client.cancelQueries({ queryKey: deviMonQueryKey, exact: true })
      client.setQueryData<DeviMonState>(deviMonQueryKey, (state) =>
        withDeviMonDisabled(state, props.email, enabled)
      )
    },
    onSuccess: async () => {
      await client.invalidateQueries(
        { queryKey: deviMonQueryKey, exact: true },
        { throwOnError: true }
      )
      toast.success(
        enabled ? t('Devin account disabled') : t('Devin account enabled'),
        {
          description: props.email,
        }
      )
    },
    onError: async () => {
      // Repaint from the server so the switch never keeps a state CPA refused.
      await client.invalidateQueries({
        queryKey: deviMonQueryKey,
        exact: true,
      })
      toast.error(t('Could not change the Devin status'), {
        description: props.email,
      })
    },
  })
  return (
    <Switch
      className='self-center'
      checked={enabled}
      aria-label={label}
      title={label}
      aria-busy={toggle.isPending}
      disabled={
        !props.configured || toggle.isPending || props.disabled === null
      }
      onCheckedChange={() => toggle.mutate()}
    />
  )
}
