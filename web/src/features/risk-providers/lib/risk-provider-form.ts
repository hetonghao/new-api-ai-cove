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

import type { Channel } from '@/features/channels/types'

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
  channel_id: null,
  model: '@cf/meta/llama-guard-3-8b',
  credential: '',
  timeout_ms: 800,
  failure_threshold: 5,
  cooldown_seconds: 30,
  priority: 0,
  daily_neurons_limit: 10000,
  daily_reset_time: '08:00',
}

export function getChannelModelOptions(
  channels: readonly Pick<Channel, 'id' | 'models'>[],
  channelId: number | null
): readonly string[] {
  const models = channels.find((channel) => channel.id === channelId)?.models
  if (!models) return []
  return [...new Set(models.split(',').map((model) => model.trim()))].filter(
    Boolean
  )
}

export function getRiskProviderFormSchema(
  t: TFunction,
  credentialRequired: boolean
) {
  return z
    .object({
      name: z.string().trim().min(1, t('Please enter a name')),
      provider_type: z.enum(['cloudflare', 'platform_internal']),
      account_id: z.string(),
      channel_id: z.number().int().positive().nullable(),
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
      priority: z.number().int().min(0, t('Priority must be zero or greater')),
      daily_neurons_limit: z
        .number()
        .int()
        .min(1, t('Daily Neurons limit must be greater than zero')),
      daily_reset_time: z.string().regex(/^([01]\d|2[0-3]):[0-5]\d$/, {
        message: t('Reset time must use HH:mm'),
      }),
    })
    .superRefine((values, context) => {
      if (
        values.provider_type === 'cloudflare' &&
        !/^[0-9a-fA-F]{32}$/.test(values.account_id.trim())
      ) {
        context.addIssue({
          code: 'custom',
          path: ['account_id'],
          message: t('Please enter a valid Cloudflare account ID'),
        })
      }
      if (
        values.provider_type === 'cloudflare' &&
        credentialRequired &&
        values.credential.trim().length === 0
      ) {
        context.addIssue({
          code: 'custom',
          path: ['credential'],
          message: t('Please enter a credential'),
        })
      }
      if (
        values.provider_type === 'platform_internal' &&
        values.channel_id === null
      ) {
        context.addIssue({
          code: 'custom',
          path: ['channel_id'],
          message: t('Please select a channel'),
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
    channel_id: provider.channel_id || null,
    model: provider.model,
    credential: '',
    timeout_ms: provider.timeout_ms,
    failure_threshold: provider.failure_threshold,
    cooldown_seconds: provider.cooldown_seconds,
    priority: provider.priority,
    daily_neurons_limit: provider.daily_neurons_limit,
    daily_reset_time: provider.daily_reset_time,
  }
}

export function formValuesToPayload(
  values: RiskProviderFormValues
): RiskProviderPayload {
  const commonPayload = {
    name: values.name.trim(),
    provider_type: values.provider_type,
    model: values.model.trim(),
    timeout_ms: values.timeout_ms,
    failure_threshold: values.failure_threshold,
    cooldown_seconds: values.cooldown_seconds,
    priority: values.priority,
    daily_neurons_limit: values.daily_neurons_limit,
    daily_reset_time: values.daily_reset_time,
  }
  if (values.provider_type === 'platform_internal') {
    return {
      ...commonPayload,
      channel_id: values.channel_id ?? undefined,
    }
  }
  const credential = values.credential.trim()
  const payload = {
    ...commonPayload,
    account_id: values.account_id.trim().toLowerCase(),
  }
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
