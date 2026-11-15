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
import { useMutation } from '@tanstack/react-query'
import { isAxiosError } from 'axios'
import { useEffect, useId, useState } from 'react'
import { useForm } from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { Dialog } from '@/components/dialog'
import { PasswordInput } from '@/components/password-input'
import { Button } from '@/components/ui/button'
import {
  Field,
  FieldDescription,
  FieldError,
  FieldLabel,
} from '@/components/ui/field'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { RadioGroup, RadioGroupItem } from '@/components/ui/radio-group'
import { getServerErrorMessage } from '@/lib/server-error-message'

import {
  cancelOpenDesignLogin,
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
  const [challenge, setChallenge] = useState<SubscriptionLogin | null>(null)
  const form = useForm<SubscriptionFormValues>({
    resolver: zodResolver(
      subscriptionFormSchema(t, props.subscription !== null, manual)
    ),
    defaultValues: {
      name: '',
      platform: 'opencode',
      credential: '',
      refresh_interval_minutes: 10,
    },
  })
  const platform = form.watch('platform')
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
      platform: props.subscription?.platform ?? 'opencode',
      credential: '',
      refresh_interval_minutes:
        props.subscription?.refresh_interval_minutes ?? 10,
    })
    setManual(false)
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
        props.subscription?.id
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
        className='space-y-5'
      >
        <fieldset
          className='space-y-2'
          disabled={locked || props.subscription !== null}
        >
          <legend className='mb-2 text-sm font-medium'>{t('Platform')}</legend>
          <RadioGroup
            value={platform}
            className='grid-cols-2'
            disabled={locked || props.subscription !== null}
            onValueChange={(value) => {
              if (value !== 'opencode' && value !== 'opendesign') return
              form.setValue('platform', value)
              form.setValue('credential', '')
              form.clearErrors()
              setManual(false)
            }}
            aria-label={t('Platform')}
          >
            <div className='flex items-center gap-2 py-1'>
              <RadioGroupItem id={`${id}-opencode`} value='opencode' />
              <Label htmlFor={`${id}-opencode`}>OpenCode Go</Label>
            </div>
            <div className='flex items-center gap-2 py-1'>
              <RadioGroupItem id={`${id}-opendesign`} value='opendesign' />
              <Label htmlFor={`${id}-opendesign`}>OpenDesign Go</Label>
            </div>
          </RadioGroup>
        </fieldset>
        <Field>
          <FieldLabel htmlFor={`${id}-name`}>{t('Name (optional)')}</FieldLabel>
          <Input
            id={`${id}-name`}
            maxLength={128}
            disabled={locked}
            placeholder={t('For example, team account')}
            aria-invalid={!!form.formState.errors.name}
            aria-describedby={`${id}-name-error`}
            {...form.register('name')}
          />
          <FieldError
            id={`${id}-name-error`}
            errors={[form.formState.errors.name]}
          />
        </Field>
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
