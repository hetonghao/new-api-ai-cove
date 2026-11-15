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

import { zodResolver } from '@hookform/resolvers/zod'
import { useMutation, useQuery } from '@tanstack/react-query'
import { isAxiosError } from 'axios'
import { useEffect, useId, useState } from 'react'
import { useForm } from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { Dialog } from '@/components/dialog'
import { PasswordInput } from '@/components/password-input'
import { Button } from '@/components/ui/button'
import { Combobox } from '@/components/ui/combobox'
import {
  Field,
  FieldDescription,
  FieldError,
  FieldLabel,
} from '@/components/ui/field'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { RadioGroup, RadioGroupItem } from '@/components/ui/radio-group'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { getServerErrorMessage } from '@/lib/server-error-message'

import {
  cancelOpenDesignLogin,
  listSubscriptionChannels,
  saveUpstreamSubscription,
  startOpenDesignLogin,
} from '../api'
import { subscriptionFormSchema } from '../lib/subscription-form'
import type {
  SubscriptionFormValues,
  SubscriptionLogin,
  UpstreamSubscription,
} from '../types'
import { SubscriptionLoginPanel } from './subscription-login-panel'
import { SubscriptionPlatform } from './subscription-platform'

type SubscriptionFormDialogProps = {
  readonly open: boolean
  readonly subscription: UpstreamSubscription | null
  readonly onOpenChange: (open: boolean) => void
  readonly onSaved: () => void
}

export function SubscriptionFormDialog(props: SubscriptionFormDialogProps) {
  const { t } = useTranslation()
  const id = useId()
  const [manual, setManual] = useState(false)
  const [bindingChannel, setBindingChannel] = useState(false)
  const [challenge, setChallenge] = useState<SubscriptionLogin | null>(null)
  const form = useForm<SubscriptionFormValues>({
    resolver: zodResolver(
      subscriptionFormSchema(
        t,
        props.subscription !== null,
        manual,
        bindingChannel
      )
    ),
    defaultValues: {
      name: '',
      account: '',
      channel_id: 0,
      platform: 'opencode',
      credential: '',
      refresh_interval_minutes: 10,
    },
  })
  const platform = form.watch('platform')
  const channelID = form.watch('channel_id')
  const channels = useQuery({
    queryKey: ['upstream-subscription-channels'],
    queryFn: ({ signal }) => listSubscriptionChannels(signal),
    enabled: props.open,
    retry: false,
    meta: { errorToast: false },
  })
  const selectedChannel = channels.data?.find(
    (channel) => channel.id === channelID
  )
  const channelOptions = (channels.data ?? []).map((channel) => ({
    value: String(channel.id),
    label: `${channel.name} · #${channel.id}`,
  }))
  if (channelID > 0 && !selectedChannel) {
    channelOptions.unshift({
      value: String(channelID),
      label: props.subscription?.name ?? `#${channelID}`,
    })
  }
  const cancelLogin = useMutation({
    mutationFn: cancelOpenDesignLogin,
    meta: { errorToast: false },
    onError: (error) => {
      if (!isAxiosError(error) || error.response?.status !== 410) {
        toast.error(
          t('Could not cancel login immediately. It will expire automatically.')
        )
      }
    },
  })

  useEffect(() => {
    if (!props.open) return
    form.reset({
      name: props.subscription?.name ?? '',
      account: props.subscription?.account ?? '',
      channel_id: props.subscription?.channel_id ?? 0,
      platform: props.subscription?.platform ?? 'opencode',
      credential: '',
      refresh_interval_minutes:
        props.subscription?.refresh_interval_minutes ?? 10,
    })
    setManual(false)
    setBindingChannel(!!props.subscription?.channel_id)
    setChallenge(null)
  }, [props.open, props.subscription, form])

  function completeLogin() {
    setChallenge(null)
    form.reset()
    props.onSaved()
    props.onOpenChange(false)
    toast.success(t('Subscription connected'))
  }

  function cancelChallenge() {
    if (challenge) cancelLogin.mutate(challenge.flow_token)
    setChallenge(null)
  }

  function handleOpenChange(open: boolean) {
    if (form.formState.isSubmitting) return
    if (!open) {
      cancelChallenge()
      form.reset()
    }
    props.onOpenChange(open)
  }

  async function beginLogin(values: SubscriptionFormValues) {
    setChallenge(
      await startOpenDesignLogin(
        values.name,
        values.refresh_interval_minutes,
        props.subscription?.id,
        values.channel_id,
        values.account
      )
    )
  }

  async function submit(values: SubscriptionFormValues) {
    try {
      if (values.platform === 'opendesign' && !manual && !props.subscription) {
        await beginLogin(values)
        return
      }
      await saveUpstreamSubscription(props.subscription?.id, values)
      form.reset()
      props.onSaved()
      props.onOpenChange(false)
      toast.success(t('Subscription saved'))
    } catch (error) {
      form.setError('root', { message: getServerErrorMessage(error) })
    }
  }

  async function reconnect(values: SubscriptionFormValues) {
    try {
      await beginLogin(values)
    } catch (error) {
      form.setError('root', { message: getServerErrorMessage(error) })
    }
  }

  const busy = form.formState.isSubmitting
  const locked = busy || challenge !== null
  const useKey = platform === 'opencode' || manual
  let submitLabel = t('Create subscription')
  if (props.subscription) {
    submitLabel = t('Save changes')
  } else if (platform === 'opendesign' && !manual) {
    submitLabel = t('Sign in to OpenDesign')
  }

  return (
    <Dialog
      open={props.open}
      onOpenChange={handleOpenChange}
      title={
        props.subscription ? t('Edit subscription') : t('Create subscription')
      }
      description={t(
        'Connect a platform account to view its subscription allowance.'
      )}
      contentClassName='sm:max-w-lg'
      footer={
        <>
          <Button
            type='button'
            variant='outline'
            onClick={() => handleOpenChange(false)}
            disabled={busy}
          >
            {t('Cancel')}
          </Button>
          {!challenge && (
            <Button type='submit' form={id} disabled={busy}>
              {busy ? t('Saving...') : submitLabel}
            </Button>
          )}
        </>
      }
    >
      <form
        id={id}
        noValidate
        onSubmit={form.handleSubmit(submit)}
        className='space-y-4'
      >
        <Field>
          <FieldLabel htmlFor={`${id}-platform`}>{t('Platform')}</FieldLabel>
          <Select
            name='platform'
            value={platform}
            disabled={locked || props.subscription !== null}
            onValueChange={(value) => {
              if (value !== 'opencode' && value !== 'opendesign') return
              form.setValue('platform', value)
              form.setValue('credential', '')
              form.clearErrors()
              setManual(false)
            }}
            items={[
              {
                value: 'opencode',
                label: <SubscriptionPlatform platform='opencode' />,
              },
              {
                value: 'opendesign',
                label: <SubscriptionPlatform platform='opendesign' />,
              },
            ]}
          >
            <SelectTrigger id={`${id}-platform`} className='w-full'>
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value='opencode'>
                <SubscriptionPlatform platform='opencode' />
              </SelectItem>
              <SelectItem value='opendesign'>
                <SubscriptionPlatform platform='opendesign' />
              </SelectItem>
            </SelectContent>
          </Select>
        </Field>
        <Field>
          <FieldLabel htmlFor={`${id}-account`}>
            {t('Subscription account (optional)')}
          </FieldLabel>
          <Input
            id={`${id}-account`}
            maxLength={128}
            disabled={locked}
            aria-invalid={!!form.formState.errors.account}
            aria-describedby={`${id}-account-error`}
            {...form.register('account')}
          />
          <FieldError
            id={`${id}-account-error`}
            errors={[form.formState.errors.account]}
          />
        </Field>
        <fieldset disabled={locked} className='space-y-2'>
          <legend className='mb-2 text-sm font-medium'>
            {t('Subscription name')}
          </legend>
          <RadioGroup
            value={bindingChannel ? 'channel' : 'independent'}
            disabled={locked}
            className='grid-cols-2'
            aria-label={t('Name source')}
            onValueChange={(value) => {
              if (value === 'independent') {
                form.setValue(
                  'name',
                  selectedChannel?.name ??
                    props.subscription?.name ??
                    form.getValues('name')
                )
                form.setValue('channel_id', 0)
                setBindingChannel(false)
              } else if (value === 'channel') {
                setBindingChannel(true)
              }
              form.clearErrors('root')
            }}
          >
            <div className='flex items-center gap-2 py-1'>
              <RadioGroupItem id={`${id}-independent`} value='independent' />
              <Label htmlFor={`${id}-independent`}>
                {t('Independent name')}
              </Label>
            </div>
            <div className='flex items-center gap-2 py-1'>
              <RadioGroupItem id={`${id}-channel`} value='channel' />
              <Label htmlFor={`${id}-channel`}>{t('Bind to channel')}</Label>
            </div>
          </RadioGroup>
        </fieldset>
        {bindingChannel && (
          <Field>
            <FieldLabel htmlFor={`${id}-channel-select`}>
              {t('Channel')}
            </FieldLabel>
            <Combobox
              id={`${id}-channel-select`}
              name='channel_id'
              options={channelOptions}
              value={String(channelID)}
              onValueChange={(value) => {
                if (value) form.setValue('channel_id', Number(value))
              }}
              placeholder={t('Select a channel')}
              emptyText={t('No channels found')}
              disabled={locked || channels.isPending || !!channels.error}
              aria-invalid={!!form.formState.errors.channel_id}
              aria-describedby={`${id}-channel-help ${id}-channel-error`}
            />
            <FieldDescription id={`${id}-channel-help`}>
              {t('The name follows the channel. Rename it in Channels.')}
            </FieldDescription>
            <FieldError
              id={`${id}-channel-error`}
              errors={[form.formState.errors.channel_id]}
            />
            {!channels.isPending &&
              !channels.error &&
              channelID > 0 &&
              !selectedChannel && (
                <FieldError>
                  {t(
                    'Bound channel was deleted. Select another channel or use an independent name.'
                  )}
                </FieldError>
              )}
          </Field>
        )}
        {bindingChannel && channels.error && (
          <Field>
            <FieldError>{t('Failed to load channels')}</FieldError>
            <Button
              type='button'
              variant='outline'
              size='sm'
              onClick={() => void channels.refetch()}
            >
              {t('Retry')}
            </Button>
          </Field>
        )}
        {!bindingChannel && (
          <Field>
            <FieldLabel htmlFor={`${id}-name`}>
              {t('Name (optional)')}
            </FieldLabel>
            <Input
              {...form.register('name')}
              id={`${id}-name`}
              maxLength={128}
              disabled={locked}
              placeholder={t('For example, team account')}
              aria-invalid={!!form.formState.errors.name}
              aria-describedby={`${id}-name-error`}
            />
            <FieldError
              id={`${id}-name-error`}
              errors={[form.formState.errors.name]}
            />
          </Field>
        )}
        <Field>
          <FieldLabel htmlFor={`${id}-interval`}>
            {t('Refresh interval (minutes)')}
          </FieldLabel>
          <Input
            id={`${id}-interval`}
            type='number'
            inputMode='numeric'
            min={1}
            max={1440}
            step={1}
            required
            disabled={locked}
            aria-invalid={!!form.formState.errors.refresh_interval_minutes}
            aria-describedby={`${id}-interval-help ${id}-interval-error`}
            {...form.register('refresh_interval_minutes', {
              valueAsNumber: true,
            })}
          />
          <FieldDescription id={`${id}-interval-help`}>
            {t('1–1440 minutes; defaults to 10 minutes.')}
          </FieldDescription>
          <FieldError
            id={`${id}-interval-error`}
            errors={[form.formState.errors.refresh_interval_minutes]}
          />
        </Field>
        {platform === 'opendesign' && !manual && !challenge && (
          <div className='space-y-3'>
            <p className='text-muted-foreground text-sm leading-relaxed'>
              {t(
                'Sign in on the official OpenDesign page and approve access. The subscription will be connected automatically.'
              )}
            </p>
            {props.subscription && (
              <Button
                type='button'
                variant='outline'
                disabled={busy}
                onClick={() => void form.handleSubmit(reconnect)()}
              >
                {t('Sign in to OpenDesign again')}
              </Button>
            )}
          </div>
        )}
        {useKey && (
          <Field>
            <FieldLabel htmlFor={`${id}-credential`}>
              {platform === 'opencode' ? 'API Key' : 'controlKey'}
              {props.subscription
                ? ` · ${t('Leave blank to keep current credential')}`
                : ' *'}
            </FieldLabel>
            <PasswordInput
              id={`${id}-credential`}
              disabled={locked}
              required={!props.subscription}
              maxLength={4096}
              autoComplete='new-password'
              autoCapitalize='none'
              autoCorrect='off'
              spellCheck={false}
              placeholder={platform === 'opencode' ? 'oc_sk_…' : 'vela_ctrl_…'}
              aria-invalid={!!form.formState.errors.credential}
              aria-describedby={`${id}-credential-help ${id}-credential-error`}
              {...form.register('credential')}
            />
            <FieldDescription id={`${id}-credential-help`}>
              {platform === 'opencode'
                ? t(
                    'Use an API key from the OpenCode console with an active Go subscription.'
                  )
                : t(
                    'Use the controlKey from your OpenDesign login profile, not a sk-amr inference key.'
                  )}
            </FieldDescription>
            <FieldError
              id={`${id}-credential-error`}
              errors={[form.formState.errors.credential]}
            />
          </Field>
        )}
        {challenge && (
          <SubscriptionLoginPanel
            challenge={challenge}
            onApproved={completeLogin}
            onRestart={cancelChallenge}
          />
        )}
        {platform === 'opendesign' && !challenge && (
          <Button
            type='button'
            variant='link'
            className='h-auto px-0 text-xs'
            disabled={busy}
            aria-expanded={manual}
            aria-controls={manual ? `${id}-credential` : undefined}
            onClick={() => {
              setManual(!manual)
              form.setValue('credential', '')
              form.clearErrors()
            }}
          >
            {manual
              ? t('Use OpenDesign sign-in instead')
              : t('Enter a controlKey manually')}
          </Button>
        )}
        <FieldError errors={[form.formState.errors.root]} />
      </form>
    </Dialog>
  )
}
