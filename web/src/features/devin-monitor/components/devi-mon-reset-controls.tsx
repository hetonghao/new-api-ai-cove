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
import { type RefObject, useId } from 'react'
import { useTranslation } from 'react-i18next'

import { ConfirmDialog } from '@/components/confirm-dialog'
import { Button } from '@/components/ui/button'
import { Checkbox } from '@/components/ui/checkbox'
import { Label } from '@/components/ui/label'
import dayjs from '@/lib/dayjs'
import { handleServerError } from '@/lib/handle-server-error'

import {
  confirmDeviMonAccountReset,
  deviMonQueryKey,
  setDeviMonAccountAutoReset,
} from '../api'
import type { DeviMonAccount, DeviMonResetOffer, DeviMonState } from '../types'

export function DeviMonResetControls(props: {
  readonly account: DeviMonAccount
  readonly configured: boolean
  readonly offer: DeviMonResetOffer | null
  readonly returnFocus: RefObject<HTMLButtonElement | null>
  readonly onOfferChange: (offer: DeviMonResetOffer | null) => void
}) {
  const { t } = useTranslation()
  const client = useQueryClient()
  const id = useId()
  const label = t('Auto reset when quota runs out')
  const { account, offer, onOfferChange } = props
  const query = { queryKey: deviMonQueryKey, exact: true }
  const updateAccount = (
    update: (current: DeviMonAccount) => DeviMonAccount
  ) => {
    client.setQueryData<DeviMonState>(deviMonQueryKey, (state) => {
      if (!state?.snapshot) return state
      return {
        ...state,
        snapshot: {
          ...state.snapshot,
          accounts: state.snapshot.accounts.map((row) =>
            row.email === account.email && row.reset_key === account.reset_key
              ? update(row)
              : row
          ),
        },
      }
    })
  }
  const preference = useMutation({
    mutationFn: (input: { readonly key: string; readonly enabled: boolean }) =>
      setDeviMonAccountAutoReset(account.email, input.key, input.enabled),
    onSuccess: async (_, { enabled }) => {
      await client.cancelQueries(query)
      updateAccount((current) => ({
        ...current,
        auto_reset: enabled,
        reset_offer: enabled ? current.reset_offer : null,
      }))
      if (!enabled) onOfferChange(null)
      await client.invalidateQueries(query)
    },
    onError: (error) => {
      handleServerError(error, t('Could not save monitor configuration'))
      void client.invalidateQueries(query)
    },
  })
  const reset = useMutation({
    mutationFn: (selected: DeviMonResetOffer) =>
      confirmDeviMonAccountReset(account.email, selected.id),
    retry: false,
    onSuccess: async (updated) => {
      await client.cancelQueries(query)
      updateAccount((current) =>
        updated.observed_at_us < current.observed_at_us
          ? current
          : { ...updated, auto_reset: current.auto_reset }
      )
      onOfferChange(null)
      await client.invalidateQueries(query)
    },
    onError: (error) => {
      onOfferChange(null)
      handleServerError(
        error,
        t(
          'Could not confirm the reset. Refresh the account to check its usage.'
        )
      )
      void client.invalidateQueries(query)
    },
  })
  const resultText = {
    applied: t('Reset card used. Usage updated.'),
    rejected: t('Reset card was not used.'),
    unverified: t('Reset result unconfirmed. Refresh to check usage.'),
  }
  return (
    <div className='flex flex-wrap items-center gap-x-3'>
      <Label
        htmlFor={id}
        className='text-muted-foreground min-h-8 cursor-pointer py-1 text-xs font-normal pointer-coarse:min-h-11'
        title={
          account.reset_key
            ? undefined
            : t('Refresh this account before enabling auto reset')
        }
      >
        <Checkbox
          id={id}
          aria-labelledby={`${id}-label`}
          checked={account.auto_reset}
          disabled={
            !props.configured ||
            !account.reset_key ||
            preference.isPending ||
            reset.isPending
          }
          onCheckedChange={(enabled) => {
            if (account.reset_key) {
              preference.mutate({ key: account.reset_key, enabled })
            }
          }}
        />
        <span aria-hidden='true'>{label}</span>
        <span id={`${id}-label`} className='sr-only'>
          {`${label} · ${account.email}`}
        </span>
      </Label>
      {account.auto_reset && account.reset_offer && (
        <Button
          type='button'
          size='xs'
          variant='outline'
          className='pointer-coarse:min-h-11'
          disabled={
            !props.configured || preference.isPending || reset.isPending
          }
          onClick={() => onOfferChange(account.reset_offer ?? null)}
        >
          {t('Confirm card use')}
        </Button>
      )}
      {account.reset_outcome && (
        <span role='status' className='text-muted-foreground text-xs'>
          {resultText[account.reset_outcome]}
        </span>
      )}
      <ConfirmDialog
        open={offer !== null}
        onOpenChange={(open) => {
          if (!open && !reset.isPending) onOfferChange(null)
        }}
        title={t('Use one reset card?')}
        desc={t(
          'The weekly quota for {{email}} is exhausted. This will use the card that expires on {{date}} and refresh usage.',
          {
            email: account.email,
            date: offer ? dayjs(offer.expires_at).format('MM-DD HH:mm') : '',
          }
        )}
        confirmText={t('Use reset card')}
        destructive
        isLoading={reset.isPending}
        finalFocus={props.returnFocus}
        disabled={
          !offer ||
          !account.auto_reset ||
          account.reset_offer?.id !== offer.id ||
          preference.isPending
        }
        handleConfirm={() => {
          if (offer) {
            reset.mutate(offer)
          }
        }}
      />
    </div>
  )
}
