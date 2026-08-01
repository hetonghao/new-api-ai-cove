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
import { ChevronDown, Loader2 } from 'lucide-react'
import { useEffect, useState } from 'react'
import { useForm } from 'react-hook-form'
import { useTranslation } from 'react-i18next'

import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import {
  Field,
  FieldError,
  FieldGroup,
  FieldLabel,
} from '@/components/ui/field'
import { Input } from '@/components/ui/input'
import { NativeSelect, NativeSelectOption } from '@/components/ui/native-select'
import type { RiskProvider } from '@/features/risk-providers/types'
import { CompactDateTimeRangePicker } from '@/features/usage-logs/components/compact-date-time-range-picker'

import { createDefaultRiskRecordFilterDraft } from '../lib/default-filter'
import {
  commitRiskRecordFilters,
  createRiskRecordFilterFormSchema,
  type RiskRecordFilterFormValues,
} from '../lib/risk-records'
import type { RiskRecordFilters } from '../types'
import { RiskRecordProviderFilter } from './risk-record-provider-filter'
import { RiskRecordSelectFilters } from './risk-record-select-filters'

type RiskRecordFiltersProps = {
  readonly disabled: boolean
  readonly initialValues: RiskRecordFilterFormValues
  readonly onApply: (filters: RiskRecordFilters) => void
  readonly providers: readonly RiskProvider[]
  readonly usernameOverride?: {
    readonly value: string
    readonly requestId: number
  }
}

export function RiskRecordFiltersForm(props: RiskRecordFiltersProps) {
  const { t } = useTranslation()
  const [advancedOpen, setAdvancedOpen] = useState(false)
  const form = useForm<RiskRecordFilterFormValues>({
    resolver: zodResolver(createRiskRecordFilterFormSchema(t)),
    defaultValues: props.initialValues,
  })
  const startTime = form.watch('start_time')
  const endTime = form.watch('end_time')
  const providerId = form.watch('provider_id')
  const channelId = form.watch('channel_id')
  const errors = form.formState.errors
  const advancedFilterCount = [providerId, channelId].filter(Boolean).length

  useEffect(() => {
    if (!props.usernameOverride) return
    form.setValue('username', props.usernameOverride.value, {
      shouldDirty: true,
      shouldValidate: true,
    })
  }, [form, props.usernameOverride])

  function submitFilters(values: RiskRecordFilterFormValues) {
    props.onApply(commitRiskRecordFilters(values))
  }

  function clearFilters() {
    const defaultValues = createDefaultRiskRecordFilterDraft()
    form.reset(defaultValues)
    props.onApply(commitRiskRecordFilters(defaultValues))
  }

  return (
    <form
      className='bg-card/50 rounded-lg border p-2.5 sm:p-3'
      onSubmit={form.handleSubmit(submitFilters)}
      noValidate
    >
      <input type='hidden' {...form.register('user_id')} />
      <div className='flex flex-col gap-2 sm:flex-row sm:items-start'>
        <FieldGroup className='grid min-w-0 flex-1 gap-3 md:grid-cols-2 xl:grid-cols-[max-content_repeat(4,minmax(9rem,1fr))]'>
          <Field
            className='w-fit max-w-full md:col-span-2 xl:col-span-1'
            data-invalid={Boolean(errors.start_time || errors.end_time)}
          >
            <FieldLabel>{t('Date Range')}</FieldLabel>
            <CompactDateTimeRangePicker
              className='w-auto max-w-full'
              start={startTime}
              end={endTime}
              onChange={(range) => {
                form.setValue('start_time', range.start, {
                  shouldDirty: true,
                  shouldValidate: true,
                })
                form.setValue('end_time', range.end, {
                  shouldDirty: true,
                  shouldValidate: true,
                })
              }}
            />
            <FieldError>
              {errors.start_time?.message || errors.end_time?.message}
            </FieldError>
          </Field>
          <RiskRecordSelectFilters control={form.control} />
          <Field data-invalid={Boolean(errors.username)}>
            <FieldLabel htmlFor='risk-record-username'>
              {t('Username')}
            </FieldLabel>
            <Input
              id='risk-record-username'
              aria-invalid={Boolean(errors.username)}
              {...form.register('username')}
            />
            <FieldError>{errors.username?.message}</FieldError>
          </Field>
          <Field data-invalid={Boolean(errors.provider_type)}>
            <FieldLabel htmlFor='risk-record-provider-type'>
              {t('Provider type')}
            </FieldLabel>
            <NativeSelect
              id='risk-record-provider-type'
              className='w-full'
              aria-invalid={Boolean(errors.provider_type)}
              {...form.register('provider_type')}
            >
              <NativeSelectOption value=''>
                {t('All provider types')}
              </NativeSelectOption>
              <NativeSelectOption value='cloudflare'>
                Cloudflare Workers AI
              </NativeSelectOption>
              <NativeSelectOption value='platform_internal'>
                {t('Platform internal model')}
              </NativeSelectOption>
            </NativeSelect>
            <FieldError>{errors.provider_type?.message}</FieldError>
          </Field>
        </FieldGroup>
        <Button
          type='button'
          variant='ghost'
          className='text-muted-foreground hover:text-foreground shrink-0 gap-1 self-end px-2 sm:mt-5'
          aria-expanded={advancedOpen}
          onClick={() => setAdvancedOpen((open) => !open)}
        >
          {advancedOpen ? t('Collapse') : t('Expand')}
          {advancedFilterCount > 0 && (
            <Badge className='ml-0.5 size-5 justify-center p-0 text-[10px]'>
              {advancedFilterCount}
            </Badge>
          )}
          <ChevronDown
            className={`size-3.5 transition-transform duration-200 ${advancedOpen ? 'rotate-180' : ''}`}
          />
        </Button>
      </div>
      {advancedOpen && (
        <FieldGroup className='mt-3 grid gap-3 md:grid-cols-2'>
          <RiskRecordProviderFilter
            control={form.control}
            providers={props.providers}
          />
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
        </FieldGroup>
      )}
      <div className='mt-3 flex flex-wrap justify-end gap-2'>
        <Button
          type='button'
          size='sm'
          variant='outline'
          disabled={props.disabled || !form.formState.isDirty}
          onClick={clearFilters}
        >
          {t('Reset')}
        </Button>
        <Button type='submit' size='sm' disabled={props.disabled}>
          {props.disabled && <Loader2 className='animate-spin' />}
          {t('Search')}
        </Button>
      </div>
    </form>
  )
}
