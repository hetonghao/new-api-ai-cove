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
import { useEffect, useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { NumericSpinnerInput } from '@/features/channels/components/numeric-spinner-input'
import { createChannelFieldUpdateScheduler } from '@/features/channels/lib/channel-field-update'
import { handleServerError } from '@/lib/handle-server-error'

import { deviMonInventoryKey, setDeviMonRouting } from '../api'
import type { DeviMonRouting, DevinInventory } from '../types'

function RoutingField({
  email,
  field,
  value,
  configured,
}: {
  email: string
  field: keyof DeviMonRouting
  value: number | null
  configured: boolean
}) {
  const { t } = useTranslation()
  const client = useQueryClient()
  const [draft, setDraft] = useState<number | null>(null)
  const label = field === 'priority' ? t('Priority') : t('Weight')
  const fallback = field === 'priority' ? 0 : 1
  const { mutate, isPending } = useMutation({
    mutationFn: (next: number) => setDeviMonRouting(email, field, next),
    scope: { id: `devin-routing:${email}` },
    retry: false,
    onSuccess: async (_, next) => {
      await client.cancelQueries({ queryKey: deviMonInventoryKey, exact: true })
      client.setQueryData<DevinInventory>(deviMonInventoryKey, (data) => {
        const key = email.trim().toLowerCase()
        const row = data?.[key]
        if (!data || !row?.devin_routing) return data
        return {
          ...data,
          [key]: {
            ...row,
            devin_routing: { ...row.devin_routing, [field]: next },
          },
        }
      })
    },
    onError: (err) =>
      handleServerError(err, t('Could not update Devin routing')),
    onSettled: async () => {
      await client.invalidateQueries({
        queryKey: deviMonInventoryKey,
        exact: true,
      })
      setDraft(null)
    },
  })
  const scheduler = useMemo(
    () => createChannelFieldUpdateScheduler(mutate),
    [mutate]
  )
  useEffect(() => () => scheduler.flush(), [scheduler])
  return (
    <span className='text-muted-foreground inline-flex flex-col items-center gap-x-0.5 text-xs tabular-nums @sm/accounts:flex-row'>
      <span>{label}</span>
      <NumericSpinnerInput
        value={draft ?? value ?? fallback}
        compact
        min={field === 'priority' ? -2147483648 : 0}
        max={field === 'priority' ? 2147483647 : 1_000_000}
        ariaLabel={`${label} · ${email}`}
        disabled={!configured || isPending}
        onCommit={scheduler.flush}
        onChange={(next) => {
          setDraft(next)
          scheduler.schedule(next)
        }}
        className='pointer-coarse:[&_button]:text-muted-foreground pointer-coarse:[&_button]:min-h-11 pointer-coarse:[&>div]:h-11'
      />
    </span>
  )
}

export function DevinRoutingControls({
  routing,
  email,
  configured,
}: {
  readonly routing?: DeviMonRouting | null
  readonly email: string
  readonly configured: boolean
}) {
  const { t } = useTranslation()
  if (!routing) {
    return (
      <span className='text-muted-foreground text-xs'>
        {t('Priority')} / {t('Weight')}: {t('Not fetched yet')}
      </span>
    )
  }
  return (
    <span className='inline-flex items-center gap-x-1.5'>
      {(['priority', 'weight'] as const).map((field) => (
        <RoutingField
          key={field}
          email={email}
          field={field}
          value={routing[field]}
          configured={configured}
        />
      ))}
    </span>
  )
}
