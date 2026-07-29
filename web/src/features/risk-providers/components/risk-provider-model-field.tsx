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
import { useEffect, useMemo } from 'react'
import { useFormContext } from 'react-hook-form'
import { useTranslation } from 'react-i18next'

import { Field, FieldError, FieldLabel } from '@/components/ui/field'
import { Input } from '@/components/ui/input'
import { NativeSelect, NativeSelectOption } from '@/components/ui/native-select'
import type { Channel } from '@/features/channels/types'

import { getChannelModelOptions } from '../lib/risk-provider-form'
import type { RiskProviderFormValues } from '../types'

type RiskProviderModelFieldProps = {
  readonly channels: readonly Channel[]
}

export function RiskProviderModelField(props: RiskProviderModelFieldProps) {
  const { t } = useTranslation()
  const form = useFormContext<RiskProviderFormValues>()
  const modelError = form.formState.errors.model
  const providerType = form.watch('provider_type')
  const channelId = form.watch('channel_id')
  const selectedModel = form.watch('model')
  const channelAvailable = props.channels.some(
    (channel) => channel.id === channelId
  )
  const channelModels = useMemo(
    () => getChannelModelOptions(props.channels, channelId),
    [props.channels, channelId]
  )
  let modelPlaceholder = t('Select a model')
  if (channelId === null) {
    modelPlaceholder = t('Select a channel')
  } else if (channelModels.length === 0) {
    modelPlaceholder = t('No models available')
  }

  useEffect(() => {
    if (
      providerType === 'platform_internal' &&
      channelId !== null &&
      channelAvailable &&
      selectedModel.length > 0 &&
      !channelModels.includes(selectedModel)
    ) {
      form.setValue('model', '', { shouldDirty: true })
    }
  }, [
    channelAvailable,
    channelId,
    channelModels,
    form,
    providerType,
    selectedModel,
  ])

  return (
    <Field className='sm:col-span-2' data-invalid={Boolean(modelError)}>
      <FieldLabel htmlFor='risk-provider-model'>{t('Model')}</FieldLabel>
      {providerType === 'platform_internal' ? (
        <NativeSelect
          id='risk-provider-model'
          className='w-full'
          disabled={channelId === null || channelModels.length === 0}
          aria-invalid={Boolean(modelError)}
          {...form.register('model')}
        >
          <NativeSelectOption value=''>{modelPlaceholder}</NativeSelectOption>
          {channelModels.map((model) => (
            <NativeSelectOption key={model} value={model}>
              {model}
            </NativeSelectOption>
          ))}
        </NativeSelect>
      ) : (
        <Input
          id='risk-provider-model'
          aria-invalid={Boolean(modelError)}
          {...form.register('model')}
        />
      )}
      <FieldError errors={[modelError]} />
    </Field>
  )
}
