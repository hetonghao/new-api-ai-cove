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
import { Controller, type Control } from 'react-hook-form'
import { useTranslation } from 'react-i18next'

import { Field, FieldError, FieldLabel } from '@/components/ui/field'
import {
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import type { RiskProvider } from '@/features/risk-providers/types'

import type { RiskRecordFilterFormValues } from '../lib/risk-records'

const ALL_PROVIDERS = 'all-providers'
const NO_PROVIDER = 'no-provider'

type RiskRecordProviderFilterProps = {
  readonly control: Control<RiskRecordFilterFormValues>
  readonly providers: readonly RiskProvider[]
}

export function RiskRecordProviderFilter(props: RiskRecordProviderFilterProps) {
  const { t } = useTranslation()

  return (
    <Controller
      control={props.control}
      name='provider_id'
      render={({ field, fieldState }) => {
        const value =
          field.value === '0' ? NO_PROVIDER : field.value || ALL_PROVIDERS
        let label = t('All providers')
        if (value === NO_PROVIDER) {
          label = t('No provider')
        } else if (value !== ALL_PROVIDERS) {
          const provider = props.providers.find(
            (item) => String(item.id) === value
          )
          label = provider ? `${provider.name} (#${provider.id})` : value
        }
        return (
          <Field data-invalid={fieldState.invalid}>
            <FieldLabel htmlFor='risk-record-provider-id'>
              {t('Cloud review provider')}
            </FieldLabel>
            <Select
              value={value}
              onValueChange={(nextValue) => {
                if (nextValue === null) return
                if (nextValue === ALL_PROVIDERS) {
                  field.onChange('')
                  return
                }
                field.onChange(nextValue === NO_PROVIDER ? '0' : nextValue)
              }}
            >
              <SelectTrigger id='risk-record-provider-id' className='w-full'>
                <SelectValue>{label}</SelectValue>
              </SelectTrigger>
              <SelectContent alignItemWithTrigger={false}>
                <SelectGroup>
                  <SelectItem value={ALL_PROVIDERS}>
                    {t('All providers')}
                  </SelectItem>
                  <SelectItem value={NO_PROVIDER}>
                    {t('No provider')}
                  </SelectItem>
                  {props.providers.map((provider) => (
                    <SelectItem key={provider.id} value={String(provider.id)}>
                      {provider.name} (#{provider.id})
                    </SelectItem>
                  ))}
                </SelectGroup>
              </SelectContent>
            </Select>
            <FieldError>{fieldState.error?.message}</FieldError>
          </Field>
        )
      }}
    />
  )
}
