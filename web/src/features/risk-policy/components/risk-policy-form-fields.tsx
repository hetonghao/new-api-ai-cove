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
import { Controller, useFormContext } from 'react-hook-form'
import { useTranslation } from 'react-i18next'

import { MultiSelect } from '@/components/multi-select'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import {
  Field,
  FieldContent,
  FieldDescription,
  FieldError,
  FieldGroup,
  FieldLabel,
  FieldTitle,
} from '@/components/ui/field'
import { NativeSelect, NativeSelectOption } from '@/components/ui/native-select'
import { Switch } from '@/components/ui/switch'
import type { Channel } from '@/features/channels/types'
import type { RiskProvider } from '@/features/risk-providers/types'

import type { RiskPolicyFormValues } from '../lib/risk-policy-form'

type RiskPolicyFormFieldsProps = {
  readonly validatedProviders: readonly RiskProvider[]
  readonly channels: readonly Channel[]
}

export function RiskPolicyFormFields(props: RiskPolicyFormFieldsProps) {
  const { t } = useTranslation()
  const form = useFormContext<RiskPolicyFormValues>()
  const enabled = form.watch('enabled')
  const reviewMode = form.watch('review_mode')
  const actionMode = form.watch('action_mode')
  const errors = form.formState.errors
  const channelOptions = props.channels.map((channel) => ({
    label: `#${channel.id} · ${channel.name}`,
    value: String(channel.id),
  }))

  return (
    <>
      {props.validatedProviders.length === 0 ? (
        <Alert>
          <AlertTitle>{t('No validated provider available')}</AlertTitle>
          <AlertDescription>
            {t(
              'Save and verify a cloud review provider before enabling AI risk control.'
            )}
          </AlertDescription>
        </Alert>
      ) : null}
      <FieldGroup className='grid gap-5 lg:grid-cols-2'>
        <Field
          orientation='horizontal'
          className='relative rounded-lg border p-3 lg:col-span-2'
        >
          <FieldContent>
            <FieldTitle className='pr-10'>
              {t('Enable AI risk control')}
            </FieldTitle>
            <FieldDescription>
              {t(
                'Only selected channels run local risk screening or cloud review.'
              )}
            </FieldDescription>
          </FieldContent>
          <Controller
            control={form.control}
            name='enabled'
            render={({ field }) => (
              <Switch
                className='absolute top-3 right-3'
                checked={field.value}
                onCheckedChange={field.onChange}
                aria-label={t('Enable AI risk control')}
              />
            )}
          />
        </Field>
        <Field data-invalid={Boolean(errors.enabled_channels)}>
          <FieldLabel htmlFor='risk-policy-channels'>
            {t('Risk channels')}
          </FieldLabel>
          <Controller
            control={form.control}
            name='enabled_channels'
            render={({ field }) => (
              <MultiSelect
                id='risk-policy-channels'
                options={channelOptions}
                selected={field.value.map(String)}
                onChange={(values) => field.onChange(values.map(Number))}
                placeholder={t('Select items...')}
                emptyText={t('No channels found')}
                disabled={!enabled || props.channels.length === 0}
              />
            )}
          />
          <FieldDescription>
            {t(
              'Only selected channels run local risk screening or cloud review.'
            )}
          </FieldDescription>
          <FieldError errors={[errors.enabled_channels]} />
        </Field>
        <Field data-invalid={Boolean(errors.provider_id)}>
          <FieldLabel htmlFor='risk-policy-provider'>
            {t('Active cloud review provider')}
          </FieldLabel>
          <NativeSelect
            id='risk-policy-provider'
            className='w-full'
            disabled={!enabled || props.validatedProviders.length === 0}
            aria-invalid={Boolean(errors.provider_id)}
            {...form.register('provider_id')}
          >
            <NativeSelectOption value=''>
              {t('Select a validated provider')}
            </NativeSelectOption>
            {props.validatedProviders.map((provider) => (
              <NativeSelectOption key={provider.id} value={provider.id}>
                {provider.name}
              </NativeSelectOption>
            ))}
          </NativeSelect>
          <FieldDescription>
            {t('Saving the policy makes this validated provider active.')}
          </FieldDescription>
          <FieldError errors={[errors.provider_id]} />
        </Field>
        <Field data-invalid={Boolean(errors.review_mode)}>
          <FieldLabel htmlFor='risk-policy-review-mode'>
            {t('Cloud review scope')}
          </FieldLabel>
          <NativeSelect
            id='risk-policy-review-mode'
            className='w-full'
            aria-invalid={Boolean(errors.review_mode)}
            {...form.register('review_mode')}
          >
            <NativeSelectOption value='selective'>
              {t('Selective cloud review')}
            </NativeSelectOption>
            <NativeSelectOption value='full'>
              {t('Full cloud review')}
            </NativeSelectOption>
          </NativeSelect>
          <FieldDescription>
            {reviewMode === 'selective'
              ? t('Only local rule matches are sent to cloud review.')
              : t('Every new user message is sent to cloud review.')}
          </FieldDescription>
          <FieldError errors={[errors.review_mode]} />
        </Field>
        <Field data-invalid={Boolean(errors.action_mode)}>
          <FieldLabel htmlFor='risk-policy-action-mode'>
            {t('Decision action')}
          </FieldLabel>
          <NativeSelect
            id='risk-policy-action-mode'
            className='w-full'
            aria-invalid={Boolean(errors.action_mode)}
            {...form.register('action_mode')}
          >
            <NativeSelectOption value='observe'>
              {t('Observe only')}
            </NativeSelectOption>
            <NativeSelectOption value='block'>
              {t('Block unsafe requests')}
            </NativeSelectOption>
          </NativeSelect>
          <FieldDescription>
            {actionMode === 'observe'
              ? t(
                  'Review runs asynchronously without adding first-token latency.'
                )
              : t(
                  'Confirmed unsafe content is rejected before upstream dispatch.'
                )}
          </FieldDescription>
          <FieldError errors={[errors.action_mode]} />
        </Field>
      </FieldGroup>
    </>
  )
}
