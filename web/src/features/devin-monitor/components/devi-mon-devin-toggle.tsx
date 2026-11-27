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

import DevinIcon from '@lobehub/icons/es/Devin/components/Mono'
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { CircleAlert, CircleCheck } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { Button } from '@/components/ui/button'
import { cn } from '@/lib/utils'

import {
  deviMonInventoryKey,
  deviMonQueryKey,
  setDeviMonDevinDisabled,
} from '../api'
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
      await client.invalidateQueries({
        queryKey: deviMonInventoryKey,
        exact: true,
      })
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
  if (props.disabled === null) {
    return (
      <span className='text-muted-foreground text-xs'>
        {t('No Devin account')}
      </span>
    )
  }
  const status = enabled ? t('Devin enabled') : t('Devin disabled')
  const Mark = enabled ? CircleCheck : CircleAlert
  return (
    <Button
      type='button'
      variant='ghost'
      size='sm'
      role='switch'
      aria-checked={enabled}
      aria-label={label}
      title={`${status} · ${label}`}
      aria-busy={toggle.isPending}
      disabled={!props.configured || toggle.isPending}
      onClick={() => toggle.mutate()}
      className={cn(
        'gap-1 rounded-md px-1.5 pointer-coarse:min-h-11',
        enabled
          ? 'bg-success/8 hover:bg-success/15'
          : 'bg-destructive/8 hover:bg-destructive/15'
      )}
      style={{
        color: `color-mix(in oklab, var(--${enabled ? 'success' : 'destructive'}) 70%, var(--foreground))`,
      }}
    >
      <DevinIcon aria-hidden='true' className='size-4' />
      <Mark aria-hidden='true' className='size-3.5' />
      <span className='sr-only'>{status}</span>
    </Button>
  )
}
