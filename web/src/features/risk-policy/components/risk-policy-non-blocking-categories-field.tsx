/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as published by
the Free Software Foundation, either version 3 of the License, or
(at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.
*/
import { Controller, useFormContext } from 'react-hook-form'
import { useTranslation } from 'react-i18next'

import { MultiSelect } from '@/components/multi-select'
import {
  Field,
  FieldDescription,
  FieldError,
  FieldLabel,
} from '@/components/ui/field'
import {
  getRiskRecordCategoryLabel,
  RISK_RECORD_CATEGORY_CODES,
} from '@/features/risk-records/lib/risk-records'

import type { RiskPolicyFormValues } from '../lib/risk-policy-form'

export function RiskPolicyNonBlockingCategoriesField() {
  const { t } = useTranslation()
  const form = useFormContext<RiskPolicyFormValues>()
  const enabled = form.watch('enabled')
  const errors = form.formState.errors
  const categoryOptions = RISK_RECORD_CATEGORY_CODES.map((category) => {
    const label = getRiskRecordCategoryLabel(category)
    return {
      label: `${category} · ${label ? t(label) : category}`,
      value: category.toLowerCase(),
    }
  })

  return (
    <Field
      className='lg:col-span-2'
      data-invalid={Boolean(errors.non_blocking_categories)}
    >
      <FieldLabel htmlFor='risk-policy-non-blocking-categories'>
        {t('Non-blocking unsafe categories')}
      </FieldLabel>
      <Controller
        control={form.control}
        name='non_blocking_categories'
        render={({ field }) => (
          <MultiSelect
            id='risk-policy-non-blocking-categories'
            options={categoryOptions}
            selected={field.value ?? []}
            onChange={field.onChange}
            allowCreate
            placeholder={t('Select non-blocking categories')}
            emptyText={t('No risk categories found')}
            disabled={!enabled}
            maxVisibleChips={3}
            aria-invalid={Boolean(errors.non_blocking_categories)}
            aria-describedby={
              errors.non_blocking_categories
                ? 'risk-policy-non-blocking-categories-error'
                : undefined
            }
          />
        )}
      />
      <FieldDescription className='text-pretty'>
        {t(
          'In block mode, an unsafe result is allowed only when every returned category is listed here. Empty or mixed categories remain blocked, and the unsafe evidence is retained.'
        )}
      </FieldDescription>
      <FieldError
        id='risk-policy-non-blocking-categories-error'
        errors={[errors.non_blocking_categories]}
      />
    </Field>
  )
}
