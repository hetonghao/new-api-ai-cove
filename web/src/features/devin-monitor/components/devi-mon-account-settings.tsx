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

import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Settings } from 'lucide-react'
import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { ConfirmDialog } from '@/components/confirm-dialog'
import { Dialog } from '@/components/dialog'
import { Button } from '@/components/ui/button'
import { Field, FieldDescription, FieldLabel } from '@/components/ui/field'
import { RadioGroup, RadioGroupItem } from '@/components/ui/radio-group'
import { getServerErrorStatus } from '@/lib/server-error-message'

import {
  deviMonAccountsKey,
  deviMonQueryKey,
  forgetDeviMonChatGPTAccount,
  getDeviMonChannelAccounts,
  pollDeviMonChatGPTLogin,
  refreshDeviMonAccount,
  setDeviMonAccountChannel,
  startDeviMonChatGPTLogin,
} from '../api'
import type { DeviMonDeviceCode } from '../types'

type Channel = 'cpa' | 'chatgpt'

// The code is polled at the interval OpenAI asks for; the dialog stops when it
// is closed so a forgotten dialog cannot keep hitting the auth endpoint.
function useDeviMonDeviceCodeLogin(open: boolean, onSignedIn: () => void) {
  const { t } = useTranslation()
  const [deviceCode, setDeviceCode] = useState<DeviMonDeviceCode | null>(null)
  const start = useMutation({
    mutationFn: startDeviMonChatGPTLogin,
    onSuccess: setDeviceCode,
  })
  useEffect(() => {
    if (!open) setDeviceCode(null)
  }, [open])
  useEffect(() => {
    if (!open || !deviceCode) return
    const timer = window.setInterval(
      () => {
        void pollDeviMonChatGPTLogin(
          deviceCode.device_auth_id,
          deviceCode.user_code
        )
          .then((result) => {
            if (!result.done) return
            setDeviceCode(null)
            onSignedIn()
            toast.success(t('Signed in to ChatGPT'), {
              description: result.email,
            })
          })
          .catch(() => {
            // A failed poll is not fatal: the next tick tries again until the
            // code expires, which the operator can see in the dialog.
          })
      },
      Math.max(deviceCode.interval, 3) * 1000
    )
    return () => window.clearInterval(timer)
  }, [deviceCode, open, onSignedIn, t])
  return { deviceCode, start, reset: () => setDeviceCode(null) }
}

// One gear per account: the dialog behind it is where the reading channel is
// chosen, so the switch lives next to the data it changes.
export function AccountSettingsButton(props: {
  readonly email: string
  readonly onOpen: () => void
}) {
  const { t } = useTranslation()
  const label = t('Subscription channel for {{email}}', { email: props.email })
  return (
    <Button
      type='button'
      variant='ghost'
      size='icon-sm'
      className='text-muted-foreground hover:text-foreground pointer-coarse:size-11'
      aria-label={label}
      title={label}
      onClick={props.onOpen}
    >
      <Settings aria-hidden='true' className='size-3.5' />
    </Button>
  )
}

export function DeviMonAccountSettings(props: {
  readonly email: string
  readonly open: boolean
  readonly onOpenChange: (open: boolean) => void
}) {
  const { t } = useTranslation()
  const client = useQueryClient()
  const accounts = useQuery({
    queryKey: deviMonAccountsKey,
    queryFn: ({ signal }) => getDeviMonChannelAccounts(signal),
    enabled: props.open,
    retry: false,
  })
  const stored = accounts.data?.find(
    (row) => row.email.toLowerCase() === props.email.toLowerCase()
  )
  const channel: Channel = stored?.channel === 'chatgpt' ? 'chatgpt' : 'cpa'
  const [confirmForget, setConfirmForget] = useState(false)

  const refresh = () => {
    void client.invalidateQueries({ queryKey: deviMonAccountsKey })
    void client.invalidateQueries({ queryKey: deviMonQueryKey })
  }
  const device = useDeviMonDeviceCodeLogin(props.open, refresh)

  const switchChannel = useMutation({
    mutationFn: async (next: Channel) => {
      await setDeviMonAccountChannel(props.email, next)
      // The stored snapshot still holds the old channel's reading. A failed
      // re-read must not turn a completed switch into an error.
      await refreshDeviMonAccount(props.email).catch(() => undefined)
    },
    onSuccess: (_data, next) => {
      refresh()
      toast.success(
        next === 'chatgpt'
          ? t('Subscription now comes from the ChatGPT sign-in')
          : t('Subscription now comes from CPA')
      )
    },
    onError: (error) => {
      const message =
        getServerErrorStatus(error) === 400
          ? t('Sign in to ChatGPT before selecting that channel')
          : t('Could not change the subscription channel')
      toast.error(message, { description: props.email })
    },
  })

  const forget = useMutation({
    mutationFn: async () => {
      await forgetDeviMonChatGPTAccount(props.email)
      await refreshDeviMonAccount(props.email).catch(() => undefined)
    },
    onSuccess: () => {
      setConfirmForget(false)
      device.reset()
      refresh()
      toast.success(t('ChatGPT sign-in removed'), {
        description: props.email,
      })
    },
    onError: () =>
      toast.error(t('Could not remove the ChatGPT sign-in'), {
        description: props.email,
      }),
  })

  return (
    <>
      <Dialog
        open={props.open}
        onOpenChange={props.onOpenChange}
        title={t('Subscription channel')}
        description={props.email}
        contentClassName='sm:max-w-lg'
        bodyClassName='space-y-5'
      >
        <RadioGroup
          value={channel}
          onValueChange={(value) => {
            const next = value as Channel
            if (next === channel) return
            if (next === 'chatgpt' && !stored?.signed_in) {
              device.start.mutate()
              return
            }
            switchChannel.mutate(next)
          }}
          aria-label={t('Subscription channel')}
        >
          <label className='flex items-start gap-2 text-sm'>
            <RadioGroupItem
              value='cpa'
              aria-label={t('CPA')}
              className='mt-0.5'
            />
            <span>
              <span className='font-medium'>{t('CPA')}</span>
              <FieldDescription>
                {t('Read the account from the Codex credentials inside CPA.')}
              </FieldDescription>
            </span>
          </label>
          <label className='flex items-start gap-2 text-sm'>
            <RadioGroupItem
              value='chatgpt'
              aria-label={t('ChatGPT sign-in')}
              className='mt-0.5'
            />
            <span>
              <span className='font-medium'>{t('ChatGPT sign-in')}</span>
              <FieldDescription>
                {t(
                  'The gateway signs in to ChatGPT itself and reads the subscription without CPA.'
                )}
              </FieldDescription>
            </span>
          </label>
        </RadioGroup>

        {device.start.isPending && (
          <p role='status' className='text-muted-foreground text-xs'>
            {t('Requesting a sign-in code...')}
          </p>
        )}

        {device.deviceCode && (
          <Field className='rounded-md border p-3'>
            <FieldLabel htmlFor='devi-mon-device-code'>
              {t('Enter this code at the link below')}
            </FieldLabel>
            <p
              id='devi-mon-device-code'
              className='font-mono text-lg tracking-wider tabular-nums'
            >
              {device.deviceCode.user_code}
            </p>
            <FieldDescription>
              <a
                className='underline'
                href={device.deviceCode.verification_url}
                target='_blank'
                rel='noreferrer'
              >
                {device.deviceCode.verification_url}
              </a>
            </FieldDescription>
            <FieldDescription>
              {t('Waiting for the code to be approved...')}
            </FieldDescription>
            <Button
              type='button'
              variant='ghost'
              size='sm'
              onClick={device.reset}
            >
              {t('Cancel sign-in')}
            </Button>
          </Field>
        )}

        {stored?.signed_in ? (
          <div className='space-y-2 text-xs'>
            <p className='text-muted-foreground'>
              {t('A ChatGPT sign-in is stored for this account.')}
            </p>
            <div className='flex flex-wrap gap-2'>
              <Button
                type='button'
                variant='outline'
                size='sm'
                disabled={device.start.isPending}
                onClick={() => device.start.mutate()}
              >
                {t('Sign in again')}
              </Button>
              <Button
                type='button'
                variant='ghost'
                size='sm'
                onClick={() => setConfirmForget(true)}
              >
                {t('Remove sign-in')}
              </Button>
            </div>
          </div>
        ) : null}

        {accounts.isError && (
          <p role='status' className='text-destructive text-xs'>
            {t('Could not load the account channels')}
          </p>
        )}
      </Dialog>

      <ConfirmDialog
        open={confirmForget}
        onOpenChange={setConfirmForget}
        title={t('Remove the ChatGPT sign-in?')}
        desc={t(
          'The account returns to the CPA channel and its tokens are deleted. You can sign in again later.'
        )}
        confirmText={t('Remove sign-in')}
        destructive
        handleConfirm={() => forget.mutate()}
        isLoading={forget.isPending}
      />
    </>
  )
}
