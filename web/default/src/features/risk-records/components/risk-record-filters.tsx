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
import { Controller, useForm } from 'react-hook-form'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'
import {
  Field,
  FieldError,
  FieldGroup,
  FieldLabel,
} from '@/components/ui/field'
import { Input } from '@/components/ui/input'
import {
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'

import {
  commitRiskRecordFilters,
  createRiskRecordFilterFormSchema,
  EMPTY_RISK_RECORD_FILTER_DRAFT,
  getRiskRecordResultFilterLabel,
  getRiskRecordSourceFilterLabel,
  type RiskRecordFilterFormValues,
} from '../lib/risk-records'
import type { RiskRecordFilters } from '../types'

const ALL_RESULTS = 'all-results'
const ALL_SOURCES = 'all-sources'

const RESULT_OPTIONS = [
  { value: 'safe', label: 'Safe' },
  { value: 'unsafe', label: 'Unsafe' },
  { value: 'error', label: 'Error' },
  { value: 'not_reviewed', label: 'Not reviewed' },
] as const

const SOURCE_OPTIONS = [
  { value: 'provider', label: 'Provider source' },
  { value: 'cache', label: 'Cache source' },
  { value: 'inflight', label: 'In-flight source' },
  { value: 'local', label: 'Local source' },
] as const

type RiskRecordFiltersProps = {
  readonly disabled: boolean
  readonly onApply: (filters: RiskRecordFilters) => void
}

export function RiskRecordFiltersForm(props: RiskRecordFiltersProps) {
  const { t } = useTranslation()
  const form = useForm<RiskRecordFilterFormValues>({
    resolver: zodResolver(createRiskRecordFilterFormSchema(t)),
    defaultValues: EMPTY_RISK_RECORD_FILTER_DRAFT,
  })
  const startTime = form.watch('start_time')
  const errors = form.formState.errors

  function submitFilters(values: RiskRecordFilterFormValues) {
    props.onApply(commitRiskRecordFilters(values))
  }

  function clearFilters() {
    form.reset(EMPTY_RISK_RECORD_FILTER_DRAFT)
    props.onApply({})
  }

  return (
    <form
      className='bg-muted/40 mb-3 rounded-lg border p-3'
      onSubmit={form.handleSubmit(submitFilters)}
      noValidate
    >
      <FieldGroup className='grid gap-3 md:grid-cols-2 xl:grid-cols-4'>
        <Field data-invalid={Boolean(errors.start_time)}>
          <FieldLabel htmlFor='risk-record-start-time'>
            {t('Start Time')}
          </FieldLabel>
          <Input
            id='risk-record-start-time'
            type='datetime-local'
            aria-invalid={Boolean(errors.start_time)}
            {...form.register('start_time')}
          />
          <FieldError>{errors.start_time?.message}</FieldError>
        </Field>
        <Field data-invalid={Boolean(errors.end_time)}>
          <FieldLabel htmlFor='risk-record-end-time'>
            {t('End Time')}
          </FieldLabel>
          <Input
            id='risk-record-end-time'
            type='datetime-local'
            min={startTime || undefined}
            aria-invalid={Boolean(errors.end_time)}
            {...form.register('end_time')}
          />
          <FieldError>{errors.end_time?.message}</FieldError>
        </Field>
        <Field data-invalid={Boolean(errors.channel_id)}>
          <FieldLabel htmlFor='risk-record-channel-id'>
            {t('Channel ID')}
          </FieldLabel>
          <Input
            id='risk-record-channel-id'
            type='number'
            inputMode='numeric'
            min='1'
            step='1'
            aria-invalid={Boolean(errors.channel_id)}
            {...form.register('channel_id')}
          />
          <FieldError>{errors.channel_id?.message}</FieldError>
        </Field>
        <Field data-invalid={Boolean(errors.user_id)}>
          <FieldLabel htmlFor='risk-record-user-id'>{t('User ID')}</FieldLabel>
          <Input
            id='risk-record-user-id'
            type='number'
            inputMode='numeric'
            min='1'
            step='1'
            aria-invalid={Boolean(errors.user_id)}
            {...form.register('user_id')}
          />
          <FieldError>{errors.user_id?.message}</FieldError>
        </Field>
        <Field data-invalid={Boolean(errors.provider_id)}>
          <FieldLabel htmlFor='risk-record-provider-id'>
            {t('Provider ID')}
          </FieldLabel>
          <Input
            id='risk-record-provider-id'
            type='number'
            inputMode='numeric'
            min='0'
            step='1'
            aria-invalid={Boolean(errors.provider_id)}
            {...form.register('provider_id')}
          />
          <FieldError>{errors.provider_id?.message}</FieldError>
        </Field>
        <Controller
          control={form.control}
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
        <Controller
          control={form.control}
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
        <div className='flex flex-wrap items-end gap-2 md:col-span-2 xl:col-span-1'>
          <Button type='submit' size='sm' disabled={props.disabled}>
            {t('Run query')}
          </Button>
          <Button
            type='button'
            size='sm'
            variant='outline'
            disabled={props.disabled}
            onClick={clearFilters}
          >
            {t('Clear')}
          </Button>
        </div>
      </FieldGroup>
    </form>
  )
}
