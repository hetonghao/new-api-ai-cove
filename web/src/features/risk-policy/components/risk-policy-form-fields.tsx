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
  FieldDescription,
  FieldError,
  FieldGroup,
  FieldLabel,
} from '@/components/ui/field'
import { NativeSelect, NativeSelectOption } from '@/components/ui/native-select'
import type { Channel } from '@/features/channels/types'
import type { RiskProvider } from '@/features/risk-providers/types'
import type { User } from '@/features/users/types'

import {
  mergeRiskPolicyModelOptions,
  type RiskPolicyFormValues,
} from '../lib/risk-policy-form'
import { RiskPolicyActivationField } from './risk-policy-activation-field'

type RiskPolicyFormFieldsProps = {
  readonly validatedProviders: readonly RiskProvider[]
  readonly channels: readonly Channel[]
  readonly users: readonly User[]
  readonly models: readonly string[]
}

export function RiskPolicyFormFields(props: RiskPolicyFormFieldsProps) {
  const { t } = useTranslation()
  const form = useFormContext<RiskPolicyFormValues>()
  const enabled = form.watch('enabled')
  const reviewMode = form.watch('review_mode')
  const actionMode = form.watch('action_mode')
  const excludedModels = form.watch('excluded_models')
  const errors = form.formState.errors
  const channelOptions = props.channels.map((channel) => ({
    label: `#${channel.id} · ${channel.name}`,
    value: String(channel.id),
  }))
  const userOptions = props.users.map((user) => ({
    label: `#${user.id} · ${user.username}`,
    value: String(user.id),
  }))
  const providerOptions = props.validatedProviders.map((provider) => ({
    label: `#${provider.id} · ${provider.name}`,
    value: String(provider.id),
  }))
  const modelOptions = mergeRiskPolicyModelOptions(
    props.models,
    excludedModels
  ).map((model) => ({ label: model, value: model }))

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
        <RiskPolicyActivationField />
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
        <Field data-invalid={Boolean(errors.provider_ids)}>
          <FieldLabel htmlFor='risk-policy-provider'>
            {t('Cloud review provider pool')}
          </FieldLabel>
          <Controller
            control={form.control}
            name='provider_ids'
            render={({ field }) => (
              <MultiSelect
                id='risk-policy-provider'
                options={providerOptions}
                selected={field.value.map(String)}
                onChange={(values) => field.onChange(values.map(Number))}
                placeholder={t('Select a validated provider')}
                emptyText={t('No validated provider available')}
                disabled={!enabled || providerOptions.length === 0}
                maxVisibleChips={3}
                aria-invalid={Boolean(errors.provider_ids)}
                aria-describedby={
                  errors.provider_ids ? 'risk-policy-provider-error' : undefined
                }
              />
            )}
          />
          <FieldDescription>
            {t('Selection order is preserved for the cloud review pool.')}
          </FieldDescription>
          <FieldError
            id='risk-policy-provider-error'
            errors={[errors.provider_ids]}
          />
        </Field>
        <Field data-invalid={Boolean(errors.excluded_user_ids)}>
          <FieldLabel htmlFor='risk-policy-excluded-users'>
            {t('Users excluded from AI risk control')}
          </FieldLabel>
          <Controller
            control={form.control}
            name='excluded_user_ids'
            render={({ field }) => (
              <MultiSelect
                id='risk-policy-excluded-users'
                options={userOptions}
                selected={field.value.map(String)}
                onChange={(values) => field.onChange(values.map(Number))}
                placeholder={t('Select users...')}
                emptyText={t('No users found')}
                disabled={!enabled || props.users.length === 0}
                maxVisibleChips={3}
              />
            )}
          />
          <FieldDescription className='text-pretty'>
            {t(
              'Selected users skip local screening, cloud review, and risk records. The existing sensitive-word check is unchanged.'
            )}
          </FieldDescription>
          <FieldError errors={[errors.excluded_user_ids]} />
        </Field>
        <Field data-invalid={Boolean(errors.excluded_models)}>
          <FieldLabel htmlFor='risk-policy-excluded-models'>
            {t('Models excluded from AI risk control')}
          </FieldLabel>
          <Controller
            control={form.control}
            name='excluded_models'
            render={({ field }) => (
              <MultiSelect
                id='risk-policy-excluded-models'
                options={modelOptions}
                selected={field.value}
                onChange={field.onChange}
                placeholder={t('Select models...')}
                emptyText={t('No models found')}
                disabled={!enabled || modelOptions.length === 0}
                maxVisibleChips={3}
              />
            )}
          />
          <FieldDescription>
            {t(
              'Selected original model names skip local screening, cloud review, and risk records. The existing sensitive-word check is unchanged.'
            )}
          </FieldDescription>
          <FieldError errors={[errors.excluded_models]} />
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
