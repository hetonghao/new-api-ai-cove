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
  setDeviMonAccountPolicy,
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
  const resumeLabel = t('Enable after quota resets')
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
    mutationFn: (
      input: { enabled: boolean } | { enable_after_reset: boolean }
    ) => {
      if (!account.reset_key) {
        throw new Error(t('Refresh this account before enabling auto reset'))
      }
      return setDeviMonAccountPolicy(account.email, account.reset_key, input)
    },
    onSuccess: async (_, policy) => {
      await client.cancelQueries(query)
      updateAccount((current) =>
        'enabled' in policy
          ? {
              ...current,
              auto_reset: policy.enabled,
              reset_offer: policy.enabled ? current.reset_offer : null,
            }
          : { ...current, auto_enable_after_reset: policy.enable_after_reset }
      )
      if ('enabled' in policy && !policy.enabled) onOfferChange(null)
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
          : {
              ...updated,
              auto_reset: current.auto_reset,
              auto_enable_after_reset: current.auto_enable_after_reset,
            }
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
  const busy =
    !props.configured ||
    !account.reset_key ||
    preference.isPending ||
    reset.isPending
  return (
    <div className='flex max-w-full flex-wrap items-center justify-end gap-x-2 gap-y-0.5 text-xs'>
      <fieldset
        disabled={busy}
        title={
          account.reset_key
            ? undefined
            : t('Refresh this account before enabling auto reset')
        }
        className='flex min-w-0 flex-wrap items-center gap-x-1.5 gap-y-1'
        aria-label={`${t('When quota runs out')} · ${account.email}`}
      >
        <legend className='sr-only'>{t('When quota runs out')}</legend>
        <span className='text-muted-foreground' aria-hidden='true'>
          {t('When quota runs out')}
        </span>
        <div className='bg-muted inline-flex rounded-md p-0.5'>
          {([false, true] as const).map((useCard) => (
            <label
              key={String(useCard)}
              className='has-checked:bg-background has-checked:text-foreground text-muted-foreground has-focus-visible:ring-ring relative inline-flex min-h-6 cursor-pointer items-center rounded-sm px-2 has-checked:shadow-xs has-focus-visible:ring-2 has-disabled:cursor-default pointer-coarse:min-h-11'
            >
              <input
                className='peer sr-only'
                type='radio'
                name={`${id}-mode`}
                checked={account.auto_reset === useCard}
                aria-label={`${useCard ? t('Auto reset when quota runs out') : t('Disable when quota runs out')} · ${account.email}`}
                onChange={() => preference.mutate({ enabled: useCard })}
              />
              {useCard ? t('Use card') : t('Disable')}
            </label>
          ))}
        </div>
      </fieldset>
      <Label
        htmlFor={id}
        className='text-muted-foreground min-h-7 cursor-pointer text-xs font-normal pointer-coarse:min-h-11'
      >
        <Checkbox
          id={id}
          checked={account.auto_enable_after_reset !== false}
          disabled={busy || account.devin_disabled === null}
          aria-label={`${resumeLabel} · ${account.email}`}
          onCheckedChange={(enable) =>
            preference.mutate({ enable_after_reset: enable })
          }
        />
        {resumeLabel}
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
