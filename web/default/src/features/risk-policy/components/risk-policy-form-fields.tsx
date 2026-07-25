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
import { useTranslation } from 'react-i18next'

import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import {
  Field,
  FieldContent,
  FieldDescription,
  FieldGroup,
  FieldLabel,
  FieldTitle,
} from '@/components/ui/field'
import { NativeSelect, NativeSelectOption } from '@/components/ui/native-select'
import { Switch } from '@/components/ui/switch'
import type { RiskProvider } from '@/features/risk-providers/types'

import type { RiskPolicyFormValues } from '../lib/risk-policy-form'

type RiskPolicyFormFieldsProps = {
  readonly values: RiskPolicyFormValues
  readonly validatedProviders: readonly RiskProvider[]
  readonly onChange: (values: RiskPolicyFormValues) => void
}

export function RiskPolicyFormFields(props: RiskPolicyFormFieldsProps) {
  const { t } = useTranslation()

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
          className='rounded-lg border p-3 lg:col-span-2'
        >
          <FieldContent>
            <FieldTitle>{t('Enable CPA Pro risk control')}</FieldTitle>
            <FieldDescription>
              {t(
                'CPA Pro is the only risk channel in the initial release. Local matches trigger cloud review and never reject by themselves.'
              )}
            </FieldDescription>
          </FieldContent>
          <Switch
            checked={props.values.enabled}
            onCheckedChange={(enabled) =>
              props.onChange({ ...props.values, enabled })
            }
            aria-label={t('Enable CPA Pro risk control')}
          />
        </Field>
        <Field>
          <FieldLabel htmlFor='risk-policy-provider'>
            {t('Active cloud review provider')}
          </FieldLabel>
          <NativeSelect
            id='risk-policy-provider'
            className='w-full'
            value={props.values.provider_id}
            disabled={
              !props.values.enabled || props.validatedProviders.length === 0
            }
            onChange={(event) =>
              props.onChange({
                ...props.values,
                provider_id: event.target.value,
              })
            }
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
        </Field>
        <Field>
          <FieldLabel htmlFor='risk-policy-review-mode'>
            {t('Cloud review scope')}
          </FieldLabel>
          <NativeSelect
            id='risk-policy-review-mode'
            className='w-full'
            value={props.values.review_mode}
            onChange={(event) =>
              props.onChange({
                ...props.values,
                review_mode:
                  event.target.value === 'full' ? 'full' : 'selective',
              })
            }
          >
            <NativeSelectOption value='selective'>
              {t('Selective cloud review')}
            </NativeSelectOption>
            <NativeSelectOption value='full'>
              {t('Full cloud review')}
            </NativeSelectOption>
          </NativeSelect>
          <FieldDescription>
            {props.values.review_mode === 'selective'
              ? t('Only local rule matches are sent to cloud review.')
              : t('Every new user message is sent to cloud review.')}
          </FieldDescription>
        </Field>
        <Field>
          <FieldLabel htmlFor='risk-policy-action-mode'>
            {t('Decision action')}
          </FieldLabel>
          <NativeSelect
            id='risk-policy-action-mode'
            className='w-full'
            value={props.values.action_mode}
            onChange={(event) =>
              props.onChange({
                ...props.values,
                action_mode:
                  event.target.value === 'block' ? 'block' : 'observe',
              })
            }
          >
            <NativeSelectOption value='observe'>
              {t('Observe only')}
            </NativeSelectOption>
            <NativeSelectOption value='block'>
              {t('Block unsafe requests')}
            </NativeSelectOption>
          </NativeSelect>
          <FieldDescription>
            {props.values.action_mode === 'observe'
              ? t(
                  'Review runs asynchronously without adding first-token latency.'
                )
              : t(
                  'Confirmed unsafe content is rejected before upstream dispatch.'
                )}
          </FieldDescription>
        </Field>
      </FieldGroup>
    </>
  )
}
