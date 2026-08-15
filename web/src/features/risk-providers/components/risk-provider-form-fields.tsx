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
import { ChevronDown } from 'lucide-react'
import { useState } from 'react'
import { useFormContext } from 'react-hook-form'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'
import {
  Collapsible,
  CollapsibleContent,
  CollapsibleTrigger,
} from '@/components/ui/collapsible'
import {
  Field,
  FieldDescription,
  FieldError,
  FieldGroup,
  FieldLabel,
} from '@/components/ui/field'
import { Input } from '@/components/ui/input'
import { NativeSelect, NativeSelectOption } from '@/components/ui/native-select'
import type { Channel } from '@/features/channels/types'

import type { RiskProviderFormValues } from '../types'
import { RiskProviderModelField } from './risk-provider-model-field'

type RiskProviderFormFieldsProps = {
  readonly hasCredential: boolean
  readonly channels: readonly Channel[]
  readonly channelsLoading: boolean
  readonly channelsError: boolean
}

export function RiskProviderFormFields(props: RiskProviderFormFieldsProps) {
  const { t } = useTranslation()
  const form = useFormContext<RiskProviderFormValues>()
  const errors = form.formState.errors
  const providerType = form.watch('provider_type')
  const providerTypeField = form.register('provider_type')
  const [advancedOpen, setAdvancedOpen] = useState(false)

  return (
    <FieldGroup className='grid gap-4 sm:grid-cols-2'>
      <Field data-invalid={Boolean(errors.name)}>
        <FieldLabel htmlFor='risk-provider-name'>{t('Name')}</FieldLabel>
        <Input
          id='risk-provider-name'
          aria-invalid={Boolean(errors.name)}
          autoFocus
          {...form.register('name')}
        />
        <FieldError errors={[errors.name]} />
      </Field>
      <Field data-invalid={Boolean(errors.provider_type)}>
        <FieldLabel htmlFor='risk-provider-type'>
          {t('Provider type')}
        </FieldLabel>
        <NativeSelect
          id='risk-provider-type'
          className='w-full'
          aria-invalid={Boolean(errors.provider_type)}
          {...providerTypeField}
          onChange={(event) => {
            providerTypeField.onChange(event)
            const nextType = event.target.value
            form.setValue('account_id', '', { shouldDirty: true })
            form.setValue('channel_id', null, { shouldDirty: true })
            if (nextType === 'openai') {
              form.setValue('model', 'omni-moderation-latest', {
                shouldDirty: true,
              })
              form.setValue('base_url', 'https://api.openai.com/v1', {
                shouldDirty: true,
              })
            } else {
              form.setValue('base_url', '', { shouldDirty: true })
              form.setValue(
                'model',
                nextType === 'cloudflare' ? '@cf/meta/llama-guard-3-8b' : '',
                { shouldDirty: true }
              )
            }
          }}
        >
          <NativeSelectOption value='cloudflare'>
            Cloudflare Workers AI
          </NativeSelectOption>
          <NativeSelectOption value='openai'>
            OpenAI Moderation
          </NativeSelectOption>
          <NativeSelectOption value='platform_internal'>
            {t('Platform internal model')}
          </NativeSelectOption>
        </NativeSelect>
        <FieldError errors={[errors.provider_type]} />
      </Field>
      {providerType === 'cloudflare' && (
        <Field
          className='sm:col-span-2'
          data-invalid={Boolean(errors.account_id)}
        >
          <FieldLabel htmlFor='risk-provider-account-id'>
            {t('Account ID')}
          </FieldLabel>
          <Input
            id='risk-provider-account-id'
            autoComplete='off'
            aria-invalid={Boolean(errors.account_id)}
            placeholder='0123456789abcdef0123456789abcdef'
            {...form.register('account_id')}
          />
          <FieldError errors={[errors.account_id]} />
        </Field>
      )}
      {providerType === 'openai' && (
        <Field
          className='sm:col-span-2'
          data-invalid={Boolean(errors.base_url)}
        >
          <FieldLabel htmlFor='risk-provider-base-url'>
            {t('Base URL')}
          </FieldLabel>
          <Input
            id='risk-provider-base-url'
            type='url'
            autoComplete='url'
            aria-invalid={Boolean(errors.base_url)}
            placeholder='https://api.openai.com/v1'
            {...form.register('base_url')}
          />
          <FieldError errors={[errors.base_url]} />
        </Field>
      )}
      {providerType === 'platform_internal' && (
        <Field
          className='sm:col-span-2'
          data-invalid={Boolean(errors.channel_id)}
        >
          <FieldLabel htmlFor='risk-provider-channel'>
            {t('Platform channel')}
          </FieldLabel>
          <NativeSelect
            id='risk-provider-channel'
            className='w-full'
            disabled={props.channelsLoading || props.channels.length === 0}
            aria-invalid={Boolean(errors.channel_id)}
            {...form.register('channel_id', {
              setValueAs: (value) => (value === '' ? null : Number(value)),
            })}
          >
            <NativeSelectOption value=''>
              {props.channelsLoading
                ? t('Loading channels...')
                : t('Select a channel')}
            </NativeSelectOption>
            {props.channels.map((channel) => (
              <NativeSelectOption key={channel.id} value={channel.id}>
                #{channel.id} · {channel.name}
              </NativeSelectOption>
            ))}
          </NativeSelect>
          <FieldDescription>
            {props.channelsError
              ? t('Failed to load channels')
              : t('The review request is fixed to this enabled channel.')}
          </FieldDescription>
          <FieldError errors={[errors.channel_id]} />
        </Field>
      )}
      <RiskProviderModelField channels={props.channels} />
      {providerType !== 'platform_internal' ? (
        <Field
          className='sm:col-span-2'
          data-invalid={Boolean(errors.credential)}
        >
          <FieldLabel htmlFor='risk-provider-credential'>
            {providerType === 'openai' ? t('API Key') : t('Credential')}
          </FieldLabel>
          <Input
            id='risk-provider-credential'
            type='password'
            autoComplete='new-password'
            aria-invalid={Boolean(errors.credential)}
            placeholder={
              props.hasCredential
                ? t('Credential configured; leave blank to keep it')
                : t('Enter a provider credential')
            }
            {...form.register('credential')}
          />
          <FieldDescription>
            {props.hasCredential
              ? t('Enter a new credential only when you want to replace it.')
              : t('The credential is write-only after saving.')}
          </FieldDescription>
          <FieldError errors={[errors.credential]} />
        </Field>
      ) : (
        <Field className='sm:col-span-2'>
          <FieldDescription>
            {t(
              'The system creates a hidden root token limited to loopback access and this model.'
            )}
          </FieldDescription>
        </Field>
      )}
      <Collapsible
        open={advancedOpen}
        onOpenChange={setAdvancedOpen}
        className='rounded-lg border sm:col-span-2'
      >
        <CollapsibleTrigger
          render={
            <Button
              type='button'
              variant='ghost'
              className='h-auto w-full justify-between rounded-lg px-3 py-2.5 text-sm'
            />
          }
        >
          <span>
            <span className='block text-left font-medium'>{t('Advanced')}</span>
            <span className='text-muted-foreground block text-left text-xs font-normal'>
              {t(
                providerType === 'cloudflare'
                  ? 'Priority, quota, reset time, and circuit breaker settings.'
                  : 'Priority and circuit breaker settings.'
              )}
            </span>
          </span>
          <ChevronDown
            className={advancedOpen ? 'size-4 rotate-180' : 'size-4'}
          />
        </CollapsibleTrigger>
        <CollapsibleContent className='border-t p-3'>
          <FieldGroup className='grid gap-4 sm:grid-cols-2'>
            <Field data-invalid={Boolean(errors.priority)}>
              <FieldLabel htmlFor='risk-provider-priority'>
                {t('Priority')}
              </FieldLabel>
              <Input
                id='risk-provider-priority'
                type='number'
                min={0}
                aria-invalid={Boolean(errors.priority)}
                {...form.register('priority', { valueAsNumber: true })}
              />
              <FieldDescription>
                {t(
                  'Higher values are selected first, matching channel priority.'
                )}
              </FieldDescription>
              <FieldError errors={[errors.priority]} />
            </Field>
            {providerType === 'cloudflare' ? (
              <>
                <Field data-invalid={Boolean(errors.daily_neurons_limit)}>
                  <FieldLabel htmlFor='risk-provider-daily-neurons-limit'>
                    {t('Daily Neurons limit')}
                  </FieldLabel>
                  <Input
                    id='risk-provider-daily-neurons-limit'
                    type='number'
                    min={1}
                    aria-invalid={Boolean(errors.daily_neurons_limit)}
                    {...form.register('daily_neurons_limit', {
                      valueAsNumber: true,
                    })}
                  />
                  <FieldDescription>
                    {t('Cloudflare only; default is 10,000 Neurons per day.')}
                  </FieldDescription>
                  <FieldError errors={[errors.daily_neurons_limit]} />
                </Field>
                <Field data-invalid={Boolean(errors.daily_reset_time)}>
                  <FieldLabel htmlFor='risk-provider-daily-reset-time'>
                    {t('Daily reset time (UTC+8)')}
                  </FieldLabel>
                  <Input
                    id='risk-provider-daily-reset-time'
                    type='time'
                    step={60}
                    aria-invalid={Boolean(errors.daily_reset_time)}
                    {...form.register('daily_reset_time')}
                  />
                  <FieldDescription>
                    {t(
                      'Default 08:00 UTC+8 equals 00:00 UTC; re-enable occurs about 5 minutes later.'
                    )}
                  </FieldDescription>
                  <FieldError errors={[errors.daily_reset_time]} />
                </Field>
              </>
            ) : null}
            <Field data-invalid={Boolean(errors.timeout_ms)}>
              <FieldLabel htmlFor='risk-provider-timeout'>
                {t('Review timeout (ms)')}
              </FieldLabel>
              <Input
                id='risk-provider-timeout'
                type='number'
                min={1}
                aria-invalid={Boolean(errors.timeout_ms)}
                {...form.register('timeout_ms', { valueAsNumber: true })}
              />
              <FieldError errors={[errors.timeout_ms]} />
            </Field>
            <Field data-invalid={Boolean(errors.failure_threshold)}>
              <FieldLabel htmlFor='risk-provider-failure-threshold'>
                {t('Failure threshold')}
              </FieldLabel>
              <Input
                id='risk-provider-failure-threshold'
                type='number'
                min={1}
                aria-invalid={Boolean(errors.failure_threshold)}
                {...form.register('failure_threshold', {
                  valueAsNumber: true,
                })}
              />
              <FieldError errors={[errors.failure_threshold]} />
            </Field>
            <Field data-invalid={Boolean(errors.cooldown_seconds)}>
              <FieldLabel htmlFor='risk-provider-cooldown'>
                {t('Circuit pause (seconds)')}
              </FieldLabel>
              <Input
                id='risk-provider-cooldown'
                type='number'
                min={1}
                aria-invalid={Boolean(errors.cooldown_seconds)}
                {...form.register('cooldown_seconds', { valueAsNumber: true })}
              />
              <FieldError errors={[errors.cooldown_seconds]} />
            </Field>
          </FieldGroup>
        </CollapsibleContent>
      </Collapsible>
    </FieldGroup>
  )
}
