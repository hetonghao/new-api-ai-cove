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

import {
  Field,
  FieldDescription,
  FieldGroup,
  FieldLabel,
} from '@/components/ui/field'
import { Input } from '@/components/ui/input'
import { NativeSelect, NativeSelectOption } from '@/components/ui/native-select'

import type { RiskProviderFormValues } from '../types'

type RiskProviderFormFieldsProps = {
  readonly values: RiskProviderFormValues
  readonly hasCredential: boolean
  readonly onChange: (values: RiskProviderFormValues) => void
}

export function RiskProviderFormFields(props: RiskProviderFormFieldsProps) {
  const { t } = useTranslation()

  return (
    <FieldGroup className='grid gap-4 sm:grid-cols-2'>
      <Field>
        <FieldLabel htmlFor='risk-provider-name'>{t('Name')}</FieldLabel>
        <Input
          id='risk-provider-name'
          value={props.values.name}
          onChange={(event) =>
            props.onChange({ ...props.values, name: event.target.value })
          }
          autoFocus
        />
      </Field>
      <Field>
        <FieldLabel htmlFor='risk-provider-type'>
          {t('Provider type')}
        </FieldLabel>
        <NativeSelect
          id='risk-provider-type'
          className='w-full'
          value={props.values.provider_type}
          disabled
        >
          <NativeSelectOption value='cloudflare'>
            Cloudflare Workers AI
          </NativeSelectOption>
        </NativeSelect>
      </Field>
      <Field className='sm:col-span-2'>
        <FieldLabel htmlFor='risk-provider-base-url'>
          {t('Connection URL')}
        </FieldLabel>
        <Input
          id='risk-provider-base-url'
          type='url'
          value={props.values.base_url}
          placeholder='https://api.cloudflare.com/client/v4/accounts/.../ai/run'
          onChange={(event) =>
            props.onChange({ ...props.values, base_url: event.target.value })
          }
        />
      </Field>
      <Field className='sm:col-span-2'>
        <FieldLabel htmlFor='risk-provider-model'>{t('Model')}</FieldLabel>
        <Input
          id='risk-provider-model'
          value={props.values.model}
          onChange={(event) =>
            props.onChange({ ...props.values, model: event.target.value })
          }
        />
      </Field>
      <Field className='sm:col-span-2'>
        <FieldLabel htmlFor='risk-provider-credential'>
          {t('Credential')}
        </FieldLabel>
        <Input
          id='risk-provider-credential'
          type='password'
          autoComplete='new-password'
          value={props.values.credential}
          placeholder={
            props.hasCredential
              ? t('Credential configured; leave blank to keep it')
              : t('Enter a provider credential')
          }
          onChange={(event) =>
            props.onChange({ ...props.values, credential: event.target.value })
          }
        />
        <FieldDescription>
          {props.hasCredential
            ? t('Enter a new credential only when you want to replace it.')
            : t('The credential is write-only after saving.')}
        </FieldDescription>
      </Field>
      <Field>
        <FieldLabel htmlFor='risk-provider-timeout'>
          {t('Review timeout (ms)')}
        </FieldLabel>
        <Input
          id='risk-provider-timeout'
          type='number'
          min={1}
          value={props.values.timeout_ms}
          onChange={(event) =>
            props.onChange({
              ...props.values,
              timeout_ms: event.target.valueAsNumber,
            })
          }
        />
      </Field>
      <Field>
        <FieldLabel htmlFor='risk-provider-failure-threshold'>
          {t('Failure threshold')}
        </FieldLabel>
        <Input
          id='risk-provider-failure-threshold'
          type='number'
          min={1}
          value={props.values.failure_threshold}
          onChange={(event) =>
            props.onChange({
              ...props.values,
              failure_threshold: event.target.valueAsNumber,
            })
          }
        />
      </Field>
      <Field>
        <FieldLabel htmlFor='risk-provider-cooldown'>
          {t('Circuit pause (seconds)')}
        </FieldLabel>
        <Input
          id='risk-provider-cooldown'
          type='number'
          min={1}
          value={props.values.cooldown_seconds}
          onChange={(event) =>
            props.onChange({
              ...props.values,
              cooldown_seconds: event.target.valueAsNumber,
            })
          }
        />
      </Field>
    </FieldGroup>
  )
}
