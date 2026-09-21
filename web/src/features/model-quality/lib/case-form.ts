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
  QualityCaseView,
  QualityCaseWrite,
  QualitySchedule,
} from '../types'

export type QualityCaseFormValues = {
  name: string
  description: string
  enabled: boolean
  output_type: 'svg' | 'text'
  mode: 'route' | 'channel'
  model: string
  token_id: number
  group: string
  channel_ids: number[]
  prompt: string
  instruction: string
  instruction_role: 'system' | 'developer'
  protocol: 'responses' | 'chat'
  max_output_tokens?: number
  reasoning_effort:
    | ''
    | 'none'
    | 'minimal'
    | 'low'
    | 'medium'
    | 'high'
    | 'xhigh'
    | 'max'
  temperature?: number
  top_p?: number
  samples_per_target: number
  timeout_seconds: number
  daily_limit?: number
  schedule_enabled: boolean
  schedule_kind: 'interval' | 'daily' | 'weekly'
  interval_minutes: number
  schedule_time: string
  weekdays: number[]
  timezone: string
}

const TIME_RE = /^([01]\d|2[0-3]):[0-5]\d$/

function isValidTimezone(value: string): boolean {
  try {
    Intl.DateTimeFormat(undefined, { timeZone: value })
    return true
  } catch {
    return false
  }
}

export function getQualityCaseFormSchema(t: TFunction) {
  return z
    .object({
      name: z
        .string()
        .trim()
        .min(1, t('Name is required'))
        .max(128, t('Name must be 128 characters or fewer')),
      description: z
        .string()
        .max(4096, t('Description must be 4096 characters or fewer')),
      enabled: z.boolean(),
      output_type: z.enum(['svg', 'text']),
      mode: z.enum(['route', 'channel']),
      model: z
        .string()
        .trim()
        .min(1, t('Model is required'))
        .max(256, t('Model must be 256 characters or fewer')),
      token_id: z
        .number()
        .int()
        .min(1, t('Select an approved execution token')),
      group: z.string(),
      channel_ids: z.array(z.number().int().positive()),
      prompt: z.string().trim().min(1, t('Prompt is required')),
      instruction: z.string(),
      instruction_role: z.enum(['system', 'developer']),
      protocol: z.enum(['responses', 'chat']),
      max_output_tokens: z
        .number()
        .int()
        .min(1, t('Max output tokens must be 1-32768'))
        .max(32768, t('Max output tokens must be 1-32768'))
        .optional(),
      reasoning_effort: z.enum([
        '',
        'none',
        'minimal',
        'low',
        'medium',
        'high',
        'xhigh',
        'max',
      ]),
      temperature: z
        .number()
        .min(0, t('Temperature must be 0-2'))
        .max(2, t('Temperature must be 0-2'))
        .optional(),
      top_p: z
        .number()
        .min(0, t('Top P must be 0-1'))
        .max(1, t('Top P must be 0-1'))
        .optional(),
      samples_per_target: z
        .number()
        .int()
        .min(1, t('Samples per target must be 1-10'))
        .max(10, t('Samples per target must be 1-10')),
      timeout_seconds: z
        .number()
        .int()
        .min(10, t('Timeout must be 10-600 seconds'))
        .max(600, t('Timeout must be 10-600 seconds')),
      daily_limit: z
        .number()
        .int()
        .min(1, t('Daily limit must be 1-10000'))
        .max(10000, t('Daily limit must be 1-10000'))
        .optional(),
      schedule_enabled: z.boolean(),
      schedule_kind: z.enum(['interval', 'daily', 'weekly']),
      interval_minutes: z
        .number()
        .int()
        .min(1, t('Interval must be 1-10080 minutes'))
        .max(10080, t('Interval must be 1-10080 minutes')),
      schedule_time: z
        .string()
        .regex(TIME_RE, t('Schedule time must be HH:mm')),
      weekdays: z.array(z.number().int().min(1).max(7)),
      timezone: z
        .string()
        .refine(isValidTimezone, t('Timezone must be a valid IANA name')),
    })
    .superRefine((values, context) => {
      if (values.prompt.length + values.instruction.length > 32768) {
        context.addIssue({
          code: 'custom',
          path: ['instruction'],
          message: t('Prompt and instruction must be 32768 characters or fewer'),
        })
      }
      if (values.instruction !== '' && !values.instruction_role) {
        context.addIssue({
          code: 'custom',
          path: ['instruction_role'],
          message: t('Select an instruction role'),
        })
      }
      if (values.mode === 'route' && values.channel_ids.length !== 0) {
        context.addIssue({
          code: 'custom',
          path: ['channel_ids'],
          message: t('Normal routing cannot pin channels'),
        })
      }
      if (values.mode === 'channel') {
        if (values.channel_ids.length === 0) {
          context.addIssue({
            code: 'custom',
            path: ['channel_ids'],
            message: t('Select at least one target channel'),
          })
        }
        if (new Set(values.channel_ids).size !== values.channel_ids.length) {
          context.addIssue({
            code: 'custom',
            path: ['channel_ids'],
            message: t('Duplicate target channels'),
          })
        }
        if (
          values.channel_ids.length * values.samples_per_target >
          100
        ) {
          context.addIssue({
            code: 'custom',
            path: ['channel_ids'],
            message: t('Total samples per run must be 100 or fewer'),
          })
        }
      }
      if (!values.schedule_enabled) return
      if (values.schedule_kind === 'weekly' && values.weekdays.length === 0) {
        context.addIssue({
          code: 'custom',
          path: ['weekdays'],
          message: t('Select at least one weekday'),
        })
      }
    })
}

export function defaultQualityCaseValues(input?: {
  token_id?: number
  group?: string
  timezone?: string
}): QualityCaseFormValues {
  const timezone =
    input?.timezone ||
    Intl.DateTimeFormat().resolvedOptions().timeZone ||
    'UTC'
  return {
    name: '',
    description: '',
    enabled: false,
    output_type: 'svg',
    mode: 'route',
    model: 'gpt-6-astra',
    token_id: input?.token_id ?? 0,
    group: input?.group ?? '',
    channel_ids: [],
    prompt:
      'Generate an SVG image of a pelican riding a bicycle by the seaside.',
    instruction: '',
    instruction_role: 'system',
    protocol: 'responses',
    max_output_tokens: undefined,
    reasoning_effort: '',
    temperature: undefined,
    top_p: undefined,
    samples_per_target: 1,
    timeout_seconds: 300,
    daily_limit: 50,
    schedule_enabled: false,
    schedule_kind: 'interval',
    interval_minutes: 60,
    schedule_time: '09:00',
    weekdays: [1, 2, 3, 4, 5],
    timezone,
  }
}

export function viewToFormValues(view: QualityCaseView): QualityCaseFormValues {
  const config = view.config
  const schedule = view.schedule
  return {
    name: view.name,
    description: view.description,
    enabled: view.enabled,
    output_type: config.output_type,
    mode: config.mode,
    model: config.model,
    token_id: config.token_id,
    group: config.group,
    channel_ids: [...(config.channel_ids ?? [])],
    prompt: config.prompt,
    instruction: config.instruction,
    instruction_role:
      config.instruction_role === 'developer' ? 'developer' : 'system',
    protocol: config.protocol,
    max_output_tokens:
      config.max_output_tokens > 0 ? config.max_output_tokens : undefined,
    reasoning_effort: config.reasoning_effort,
    temperature: config.temperature,
    top_p: config.top_p,
    samples_per_target: config.samples_per_target,
    timeout_seconds: config.timeout_seconds,
    daily_limit: config.daily_limit > 0 ? config.daily_limit : undefined,
    schedule_enabled: schedule.enabled,
    schedule_kind: schedule.kind,
    interval_minutes: schedule.interval_minutes,
    schedule_time: schedule.time,
    weekdays: [...(schedule.weekdays ?? [])],
    timezone: schedule.timezone,
  }
}

export function formValuesToWrite(
  values: QualityCaseFormValues,
  expectedEditVersion: number
): QualityCaseWrite {
  const schedule: QualitySchedule = {
    enabled: values.schedule_enabled,
    kind: values.schedule_kind,
    interval_minutes: values.interval_minutes,
    time: values.schedule_time,
    weekdays: [...values.weekdays].sort((a, b) => a - b),
    timezone: values.timezone,
  }
  return {
    name: values.name.trim(),
    description: values.description,
    enabled: values.enabled,
    expected_edit_version: expectedEditVersion,
    config: {
      model: values.model.trim(),
      output_type: values.output_type,
      mode: values.mode,
      protocol: values.protocol,
      token_id: values.token_id,
      group: values.group,
      channel_ids: values.mode === 'channel' ? values.channel_ids : [],
      prompt: values.prompt,
      instruction: values.instruction,
      instruction_role:
        values.instruction === '' ? '' : values.instruction_role,
      max_output_tokens: values.max_output_tokens ?? 0,
      reasoning_effort: values.reasoning_effort,
      temperature: values.temperature,
      top_p: values.top_p,
      samples_per_target: values.samples_per_target,
      timeout_seconds: values.timeout_seconds,
      daily_limit: values.daily_limit ?? 0,
    },
    schedule,
  }
}
