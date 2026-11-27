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
import { useRef, useState } from 'react'
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
  refreshDeviMonAccount,
  saveDeviMonChatGPTSession,
  setDeviMonAccountChannel,
} from '../api'

type Channel = 'cpa' | 'chatgpt'

// The operator copies this in a browser that is already signed in to ChatGPT.
const SESSION_URL = 'https://chatgpt.com/api/auth/session'

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
  const [sessionJson, setSessionJson] = useState('')
  const [confirmForget, setConfirmForget] = useState(false)
  const sessionField = useRef<HTMLTextAreaElement>(null)
  const accounts = useQuery({
    queryKey: deviMonAccountsKey,
    queryFn: ({ signal }) => getDeviMonChannelAccounts(signal),
    enabled: props.open,
    retry: false,
  })
  const stored = accounts.data?.find(
    (row) => row.email.toLowerCase() === props.email.toLowerCase()
  )
  const savedChannel: Channel =
    stored?.channel === 'chatgpt' ? 'chatgpt' : 'cpa'
  // The operator can preview the ChatGPT branch before a session exists; that
  // preview is what reveals the paste box, so it is local until saved.
  const [preview, setPreview] = useState<Channel | null>(null)
  const channel: Channel = preview ?? savedChannel

  const refresh = () => {
    void client.invalidateQueries({ queryKey: deviMonAccountsKey })
    void client.invalidateQueries({ queryKey: deviMonQueryKey })
  }

  const saveSession = useMutation({
    mutationFn: async (raw: string) => {
      // Fail before the request when the paste is obviously not JSON, so a
      // mis-copied clipboard does not look like a server error.
      try {
        JSON.parse(raw)
      } catch {
        throw new Error(t('That is not the session JSON'))
      }
      return saveDeviMonChatGPTSession(raw)
    },
    onSuccess: (saved) => {
      setSessionJson('')
      setPreview(null)
      refresh()
      toast.success(t('ChatGPT session saved'), { description: saved.email })
    },
    onError: (error) => {
      toast.error(t('Could not verify the ChatGPT session'), {
        description: error.message,
      })
    },
  })

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
          ? t('Subscription now comes from the ChatGPT session')
          : t('Subscription now comes from CPA')
      )
    },
    onError: (error) => {
      const message =
        getServerErrorStatus(error) === 400
          ? t('Save a ChatGPT session before selecting that channel')
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
      refresh()
      toast.success(t('ChatGPT session removed'), {
        description: props.email,
      })
    },
    onError: () =>
      toast.error(t('Could not remove the ChatGPT session'), {
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
            // Without a stored session there is nothing to switch to yet: reveal
            // the paste box and let the operator decide, rather than ignoring
            // the click or silently switching to a channel with no credential.
            if (next === 'chatgpt' && !stored?.signed_in) {
              setPreview('chatgpt')
              return
            }
            setPreview(null)
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
              aria-label={t('ChatGPT session')}
              className='mt-0.5'
            />
            <span>
              <span className='font-medium'>{t('ChatGPT session')}</span>
              <FieldDescription>
                {t(
                  'The gateway reads the subscription from a ChatGPT web session, without CPA.'
                )}
              </FieldDescription>
            </span>
          </label>
        </RadioGroup>

        {/* The paste box belongs to the ChatGPT branch only. Showing it while
            CPA is selected made the dialog look like both channels were active. */}
        {channel === 'chatgpt' && (
          <Field className='rounded-md border p-3'>
            <FieldLabel htmlFor='devi-mon-session'>
              {t('ChatGPT session JSON')}
            </FieldLabel>
            <FieldDescription>
              {t(
                'Open the link below in the browser that is signed in to this account, copy the whole page, and paste it here.'
              )}
            </FieldDescription>
            <Button
              role='link'
              variant='outline'
              size='sm'
              render={
                <a
                  href={SESSION_URL}
                  target='_blank'
                  rel='noreferrer noopener'
                />
              }
            >
              {t('Open the session page')}
            </Button>
            <textarea
              ref={sessionField}
              id='devi-mon-session'
              className='border-input bg-background focus-visible:border-ring focus-visible:ring-ring/50 min-h-24 w-full rounded-md border p-2 font-mono text-xs outline-none focus-visible:ring-3'
              placeholder='{"accessToken":"…","sessionToken":"…"}'
              value={sessionJson}
              onChange={(event) => setSessionJson(event.target.value)}
              spellCheck={false}
              autoComplete='off'
            />
            <div className='flex flex-wrap gap-2'>
              <Button
                type='button'
                size='sm'
                disabled={saveSession.isPending || sessionJson.trim() === ''}
                onClick={() => saveSession.mutate(sessionJson.trim())}
              >
                {saveSession.isPending
                  ? t('Verifying...')
                  : t('Save ChatGPT session')}
              </Button>
              {sessionJson !== '' && (
                <Button
                  type='button'
                  variant='ghost'
                  size='sm'
                  onClick={() => setSessionJson('')}
                >
                  {t('Clear')}
                </Button>
              )}
            </div>
          </Field>
        )}

        {stored?.signed_in && channel === 'chatgpt' ? (
          <div className='space-y-2 text-xs'>
            <p className='text-muted-foreground'>
              {t('A ChatGPT session is stored for this account.')}
            </p>
            <div className='flex flex-wrap gap-2'>
              <Button
                type='button'
                variant='ghost'
                size='sm'
                onClick={() => setConfirmForget(true)}
              >
                {t('Remove session')}
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
        title={t('Remove the ChatGPT session?')}
        desc={t(
          'The account returns to the CPA channel and the stored session is deleted. You can paste a new one later.'
        )}
        confirmText={t('Remove session')}
        destructive
        handleConfirm={() => forget.mutate()}
        isLoading={forget.isPending}
      />
    </>
  )
}
