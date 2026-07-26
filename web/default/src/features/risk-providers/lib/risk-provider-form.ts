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
import type { TFunction } from 'i18next'
import { z } from 'zod'

import type {
  RiskProvider,
  RiskProviderFormValues,
  RiskProviderPayload,
} from '../types'

type RiskProviderServerFormError = {
  readonly name: 'root.server'
  readonly error: {
    readonly type: 'server'
    readonly message: string
  }
}

export const RISK_PROVIDER_DEFAULT_VALUES: RiskProviderFormValues = {
  name: '',
  provider_type: 'cloudflare',
  account_id: '',
  model: '@cf/meta/llama-guard-3-8b',
  credential: '',
  timeout_ms: 800,
  failure_threshold: 5,
  cooldown_seconds: 30,
}

export function getRiskProviderFormSchema(
  t: TFunction,
  credentialRequired: boolean
) {
  return z
    .object({
      name: z.string().trim().min(1, t('Please enter a name')),
      provider_type: z.literal('cloudflare'),
      account_id: z
        .string()
        .trim()
        .regex(
          /^[0-9a-fA-F]{32}$/,
          t('Please enter a valid Cloudflare account ID')
        ),
      model: z.string().trim().min(1, t('Please enter a model')),
      credential: z.string(),
      timeout_ms: z
        .number()
        .int()
        .min(1, t('Timeout must be greater than zero')),
      failure_threshold: z
        .number()
        .int()
        .min(1, t('Failure threshold must be greater than zero')),
      cooldown_seconds: z
        .number()
        .int()
        .min(1, t('Cooldown must be greater than zero')),
    })
    .superRefine((values, context) => {
      if (credentialRequired && values.credential.trim().length === 0) {
        context.addIssue({
          code: 'custom',
          path: ['credential'],
          message: t('Please enter a credential'),
        })
      }
    })
}

export function providerToFormValues(
  provider: RiskProvider
): RiskProviderFormValues {
  return {
    name: provider.name,
    provider_type: provider.provider_type,
    account_id: provider.account_id,
    model: provider.model,
    credential: '',
    timeout_ms: provider.timeout_ms,
    failure_threshold: provider.failure_threshold,
    cooldown_seconds: provider.cooldown_seconds,
  }
}

export function formValuesToPayload(
  values: RiskProviderFormValues
): RiskProviderPayload {
  const payload = {
    name: values.name.trim(),
    provider_type: values.provider_type,
    account_id: values.account_id.trim().toLowerCase(),
    model: values.model.trim(),
    timeout_ms: values.timeout_ms,
    failure_threshold: values.failure_threshold,
    cooldown_seconds: values.cooldown_seconds,
  }
  const credential = values.credential.trim()

  return credential ? { ...payload, credential } : payload
}

export function getRiskProviderServerFormError(
  message: string
): RiskProviderServerFormError {
  return {
    name: 'root.server',
    error: { type: 'server', message },
  }
}

export function canActivateProvider(provider: RiskProvider): boolean {
  return provider.validated_at !== null && !provider.active
}
