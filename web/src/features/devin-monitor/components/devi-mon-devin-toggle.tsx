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
import { Power } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { Button } from '@/components/ui/button'

import { deviMonQueryKey, setDeviMonDevinDisabled } from '../api'

// Mirrors cpa-pro-1's enable/disable toggle. It flips the Devin credential in
// CPA, which is the same state the row's badge shows; the reading channel is
// unrelated, so every row keeps this switch.
export function DevinAccountToggle(props: {
  readonly email: string
  readonly disabled: boolean | null
  readonly configured: boolean
}) {
  const { t } = useTranslation()
  const client = useQueryClient()
  const next = !(props.disabled ?? false)
  const toggle = useMutation({
    mutationFn: () => setDeviMonDevinDisabled(props.email, next),
    onSuccess: async () => {
      await client.invalidateQueries(
        { queryKey: deviMonQueryKey, exact: true },
        { throwOnError: true }
      )
      toast.success(
        next ? t('Devin account disabled') : t('Devin account enabled'),
        {
          description: props.email,
        }
      )
    },
    onError: () => {
      toast.error(t('Could not change the Devin status'), {
        description: props.email,
      })
    },
  })
  const label = next
    ? t('Disable {{email}}', { email: props.email })
    : t('Enable {{email}}', { email: props.email })
  return (
    <Button
      type='button'
      variant='ghost'
      size='icon-sm'
      className='text-muted-foreground hover:text-foreground pointer-coarse:size-11'
      aria-label={label}
      title={label}
      aria-busy={toggle.isPending}
      disabled={
        !props.configured || toggle.isPending || props.disabled === null
      }
      onClick={() => toggle.mutate()}
    >
      <Power
        aria-hidden='true'
        className={props.disabled ? 'text-destructive size-3.5' : 'size-3.5'}
      />
    </Button>
  )
}
