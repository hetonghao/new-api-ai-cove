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

import {
  getRiskRecordResultFilterLabel,
  getRiskRecordSourceFilterLabel,
  type RiskRecordFilterFormValues,
} from '../lib/risk-records'

const ALL_RESULTS = 'all-results'
const ALL_SOURCES = 'all-sources'

const RESULT_OPTIONS = [
  { value: 'safe', label: 'Safe' },
  { value: 'unsafe', label: 'Unsafe' },
  { value: 'error', label: 'Error' },
  { value: 'not_reviewed', label: 'Not reviewed' },
] as const

const SOURCE_OPTIONS = [
  { value: 'provider', label: 'Cloud review source' },
  { value: 'cache', label: 'Cache source' },
  { value: 'inflight', label: 'In-flight source' },
  { value: 'local', label: 'Local source' },
] as const

export function RiskRecordSelectFilters(props: {
  readonly control: Control<RiskRecordFilterFormValues>
}) {
  const { t } = useTranslation()

  return (
    <>
      <Controller
        control={props.control}
        name='source'
        render={({ field, fieldState }) => {
          const value = field.value || ALL_SOURCES
          const label = getRiskRecordSourceFilterLabel(value)
          return (
            <Field data-invalid={fieldState.invalid}>
              <FieldLabel htmlFor='risk-record-source'>
                {t('Source')}
              </FieldLabel>
              <Select
                value={value}
                onValueChange={(nextValue) => {
                  if (nextValue === null) return
                  field.onChange(nextValue === ALL_SOURCES ? '' : nextValue)
                }}
              >
                <SelectTrigger id='risk-record-source' className='w-full'>
                  <SelectValue>{t(label)}</SelectValue>
                </SelectTrigger>
                <SelectContent alignItemWithTrigger={false}>
                  <SelectGroup>
                    <SelectItem value={ALL_SOURCES}>
                      {t('All sources')}
                    </SelectItem>
                    {SOURCE_OPTIONS.map((option) => (
                      <SelectItem key={option.value} value={option.value}>
                        {t(option.label)}
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
      <Controller
        control={props.control}
        name='result'
        render={({ field, fieldState }) => {
          const value = field.value || ALL_RESULTS
          const label = getRiskRecordResultFilterLabel(value)
          return (
            <Field data-invalid={fieldState.invalid}>
              <FieldLabel htmlFor='risk-record-result'>
                {t('Result')}
              </FieldLabel>
              <Select
                value={value}
                onValueChange={(nextValue) => {
                  if (nextValue === null) return
                  field.onChange(nextValue === ALL_RESULTS ? '' : nextValue)
                }}
              >
                <SelectTrigger id='risk-record-result' className='w-full'>
                  <SelectValue>{t(label)}</SelectValue>
                </SelectTrigger>
                <SelectContent alignItemWithTrigger={false}>
                  <SelectGroup>
                    <SelectItem value={ALL_RESULTS}>
                      {t('All results')}
                    </SelectItem>
                    {RESULT_OPTIONS.map((option) => (
                      <SelectItem key={option.value} value={option.value}>
                        {t(option.label)}
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
    </>
  )
}
