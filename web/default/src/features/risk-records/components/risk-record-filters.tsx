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
import { useState, type FormEvent } from 'react'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'
import { Field, FieldGroup, FieldLabel } from '@/components/ui/field'
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
  EMPTY_RISK_RECORD_FILTER_DRAFT,
  getRiskRecordResultFilterLabel,
  getRiskRecordSourceFilterLabel,
} from '../lib/risk-records'
import type { RiskRecordFilterDraft, RiskRecordFilters } from '../types'

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
  const [draft, setDraft] = useState<RiskRecordFilterDraft>(
    EMPTY_RISK_RECORD_FILTER_DRAFT
  )
  const resultValue = draft.result || ALL_RESULTS
  const sourceValue = draft.source || ALL_SOURCES
  const resultLabel = getRiskRecordResultFilterLabel(resultValue)
  const sourceLabel = getRiskRecordSourceFilterLabel(sourceValue)

  function submitFilters(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    props.onApply(commitRiskRecordFilters(draft))
  }

  function clearFilters() {
    setDraft(EMPTY_RISK_RECORD_FILTER_DRAFT)
    props.onApply({})
  }

  return (
    <form
      className='bg-muted/40 mb-3 rounded-lg border p-3'
      onSubmit={submitFilters}
    >
      <FieldGroup className='grid gap-3 md:grid-cols-2 xl:grid-cols-4'>
        <Field>
          <FieldLabel htmlFor='risk-record-start-time'>
            {t('Start Time')}
          </FieldLabel>
          <Input
            id='risk-record-start-time'
            type='datetime-local'
            value={draft.start_time}
            onChange={(event) =>
              setDraft({ ...draft, start_time: event.target.value })
            }
          />
        </Field>
        <Field>
          <FieldLabel htmlFor='risk-record-end-time'>
            {t('End Time')}
          </FieldLabel>
          <Input
            id='risk-record-end-time'
            type='datetime-local'
            value={draft.end_time}
            min={draft.start_time || undefined}
            onChange={(event) =>
              setDraft({ ...draft, end_time: event.target.value })
            }
          />
        </Field>
        <Field>
          <FieldLabel htmlFor='risk-record-channel-id'>
            {t('Channel ID')}
          </FieldLabel>
          <Input
            id='risk-record-channel-id'
            type='number'
            inputMode='numeric'
            min='1'
            step='1'
            value={draft.channel_id}
            onChange={(event) =>
              setDraft({ ...draft, channel_id: event.target.value })
            }
          />
        </Field>
        <Field>
          <FieldLabel htmlFor='risk-record-user-id'>{t('User ID')}</FieldLabel>
          <Input
            id='risk-record-user-id'
            type='number'
            inputMode='numeric'
            min='1'
            step='1'
            value={draft.user_id}
            onChange={(event) =>
              setDraft({ ...draft, user_id: event.target.value })
            }
          />
        </Field>
        <Field>
          <FieldLabel htmlFor='risk-record-provider-id'>
            {t('Provider ID')}
          </FieldLabel>
          <Input
            id='risk-record-provider-id'
            type='number'
            inputMode='numeric'
            min='0'
            step='1'
            value={draft.provider_id}
            onChange={(event) =>
              setDraft({ ...draft, provider_id: event.target.value })
            }
          />
        </Field>
        <Field>
          <FieldLabel htmlFor='risk-record-result'>{t('Result')}</FieldLabel>
          <Select
            value={resultValue}
            onValueChange={(value) =>
              value !== null &&
              setDraft({
                ...draft,
                result: value === ALL_RESULTS ? '' : value,
              })
            }
          >
            <SelectTrigger id='risk-record-result' className='w-full'>
              <SelectValue>{t(resultLabel)}</SelectValue>
            </SelectTrigger>
            <SelectContent alignItemWithTrigger={false}>
              <SelectGroup>
                <SelectItem value={ALL_RESULTS}>{t('All results')}</SelectItem>
                {RESULT_OPTIONS.map((option) => (
                  <SelectItem key={option.value} value={option.value}>
                    {t(option.label)}
                  </SelectItem>
                ))}
              </SelectGroup>
            </SelectContent>
          </Select>
        </Field>
        <Field>
          <FieldLabel htmlFor='risk-record-source'>{t('Source')}</FieldLabel>
          <Select
            value={sourceValue}
            onValueChange={(value) =>
              value !== null &&
              setDraft({
                ...draft,
                source: value === ALL_SOURCES ? '' : value,
              })
            }
          >
            <SelectTrigger id='risk-record-source' className='w-full'>
              <SelectValue>{t(sourceLabel)}</SelectValue>
            </SelectTrigger>
            <SelectContent alignItemWithTrigger={false}>
              <SelectGroup>
                <SelectItem value={ALL_SOURCES}>{t('All sources')}</SelectItem>
                {SOURCE_OPTIONS.map((option) => (
                  <SelectItem key={option.value} value={option.value}>
                    {t(option.label)}
                  </SelectItem>
                ))}
              </SelectGroup>
            </SelectContent>
          </Select>
        </Field>
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
