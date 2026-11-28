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
import { type RefObject, useContext, useEffect, useId } from 'react'
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
import { DevinOperationMode } from '../operation-mode'
import type {
  DeviMonAccount,
  DeviMonPolicyUpdate,
  DeviMonResetOffer,
  DeviMonState,
} from '../types'
import { DevinOperationGroup } from './devi-mon-operation-group'

export function DeviMonResetControls(props: {
  readonly account: DeviMonAccount
  readonly configured: boolean
  readonly offer: DeviMonResetOffer | null
  readonly returnFocus: RefObject<HTMLButtonElement | null>
  readonly onOfferChange: (offer: DeviMonResetOffer | null) => void
}) {
  const { t } = useTranslation()
  const operationMode = useContext(DevinOperationMode)
  const client = useQueryClient()
  const id = useId()
  const resumeLabel = t('Enable after quota resets')
  const { account, offer, onOfferChange } = props
  const useCard = account.exhaustion_action === 'use_card'
  const currentOffer =
    useCard && offer?.id === account.reset_offer?.id ? offer : null
  useEffect(() => {
    if (offer && !useCard) onOfferChange(null)
  }, [offer, useCard, onOfferChange])
  const choices = [
    {
      action: 'disable',
      label: t('Disable'),
      description: t('Disable when quota runs out'),
    },
    {
      action: 'use_card',
      label: t('Use card'),
      description: t('Auto reset when quota runs out'),
    },
    {
      action: 'silent',
      label: t('Silent'),
      description: t('Silent: take no action when quota runs out'),
    },
  ] as const
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
    mutationFn: (input: DeviMonPolicyUpdate) => {
      if (!account.reset_key) {
        throw new Error(t('Refresh this account before enabling auto reset'))
      }
      return setDeviMonAccountPolicy(account.email, account.reset_key, input)
    },
    onSuccess: async (_, policy) => {
      await client.cancelQueries(query)
      updateAccount((current) =>
        'exhaustion_action' in policy
          ? {
              ...current,
              exhaustion_action: policy.exhaustion_action,
              reset_offer:
                policy.exhaustion_action === 'use_card'
                  ? current.reset_offer
                  : null,
            }
          : { ...current, auto_enable_after_reset: policy.enable_after_reset }
      )
      if (
        'exhaustion_action' in policy &&
        policy.exhaustion_action !== 'use_card'
      ) {
        onOfferChange(null)
      }
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
              exhaustion_action: current.exhaustion_action,
              reset_offer:
                current.exhaustion_action === 'use_card'
                  ? updated.reset_offer
                  : null,
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
    <>
      <DevinOperationGroup
        label={`${t('Weekly allowance')} · ${account.email}`}
        className='flex flex-wrap items-center justify-end gap-x-2 gap-y-0.5 text-xs'
      >
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
            {choices.map(({ action, label, description }) => (
              <label
                key={action}
                title={description}
                className='has-checked:bg-background has-checked:text-foreground text-muted-foreground has-focus-visible:ring-ring relative inline-flex min-h-6 cursor-pointer items-center rounded-sm px-2 whitespace-nowrap has-checked:shadow-xs has-focus-visible:ring-2 has-disabled:cursor-default pointer-coarse:min-h-11'
              >
                <input
                  className='peer sr-only'
                  type='radio'
                  name={`${id}-mode`}
                  value={action}
                  checked={account.exhaustion_action === action}
                  aria-label={`${description} · ${account.email}`}
                  onChange={() =>
                    preference.mutate({ exhaustion_action: action })
                  }
                />
                {label}
              </label>
            ))}
          </div>
        </fieldset>
        <Label
          htmlFor={id}
          className='text-muted-foreground min-h-7 cursor-pointer text-xs font-normal whitespace-nowrap pointer-coarse:min-h-11'
        >
          <Checkbox
            id={id}
            checked={account.auto_enable_after_reset !== false}
            disabled={!operationMode || busy || account.devin_disabled === null}
            onCheckedChange={(enable) =>
              preference.mutate({ enable_after_reset: enable })
            }
          />
          <span>
            {resumeLabel} <span className='sr-only'>· {account.email}</span>
          </span>
        </Label>
        {useCard && account.reset_offer && (
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
      </DevinOperationGroup>
      <ConfirmDialog
        open={operationMode && currentOffer !== null}
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
        disabled={!currentOffer || preference.isPending}
        handleConfirm={() => {
          if (operationMode && currentOffer) {
            reset.mutate(currentOffer)
          }
        }}
      />
    </>
  )
}
