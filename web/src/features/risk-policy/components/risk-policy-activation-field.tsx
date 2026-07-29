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

import {
  Field,
  FieldContent,
  FieldDescription,
  FieldTitle,
} from '@/components/ui/field'
import { Switch } from '@/components/ui/switch'

import type { RiskPolicyFormValues } from '../lib/risk-policy-form'

export function RiskPolicyActivationField() {
  const { t } = useTranslation()
  const form = useFormContext<RiskPolicyFormValues>()

  return (
    <Field
      orientation='horizontal'
      className='relative rounded-lg border p-3 lg:col-span-2'
    >
      <FieldContent>
        <FieldTitle className='pr-10'>{t('Enable AI risk control')}</FieldTitle>
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
  )
}
