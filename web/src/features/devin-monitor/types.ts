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

import { z } from 'zod'

const deviMonCardSchema = z.object({
  expires_at: z.string(),
})

const deviMonRoutingSchema = z.object({
  priority: z.number().int().nullable(),
  weight: z.number().int().nonnegative().nullable(),
})

const deviMonResetOfferSchema = z.object({
  id: z.uuid(),
  expires_at: z.string(),
})

export const deviMonAccountSchema = z.object({
  email: z.string(),
  used_percent: z.number().nullable(),
  reset_at: z.number().int().nullable(),
  plan_type: z.string(),
  credit_balance: z.string(),
  unlimited: z.boolean(),
  cards: z.array(deviMonCardSchema),
  renew_at: z.string(),
  renew_source: z.string(),
  devin_disabled: z.boolean().nullable(),
  devin_routing: deviMonRoutingSchema.nullish(),
  channel: z.enum(['cpa', 'chatgpt']).default('cpa'),
  error: z.string(),
  observed_at_us: z.number().int().default(0),
  reset_key: z.string().optional(),
  auto_reset: z.boolean().default(false),
  reset_offer: deviMonResetOfferSchema.nullish(),
  reset_outcome: z.enum(['applied', 'rejected', 'unverified']).optional(),
})

const deviMonDevinSchema = z.object({
  email: z.string(),
  disabled: z.boolean(),
  devin_routing: deviMonRoutingSchema.nullish(),
})

export const deviMonSnapshotSchema = z.object({
  collected_at: z.number().int(),
  duration_ms: z.number().int(),
  interval_minutes: z.number().int(),
  accounts: z.array(deviMonAccountSchema),
  unpaired_devin: z.array(deviMonDevinSchema),
  errors: z.array(z.string()),
})

export const deviMonStateSchema = z.object({
  snapshot: deviMonSnapshotSchema.nullable(),
  collecting: z.boolean(),
  next_run_at: z.number().int(),
  configured: z.boolean(),
  last_error: z.string().optional(),
  interval_minutes: z.number().int().positive().optional(),
})

export const deviMonSettingsSchema = z.object({
  interval_minutes: z.number().int().min(15).max(10080),
  chatgpt_proxy_url: z.string().default(''),
})

// One credential the monitor can read through either channel.
export const deviMonChannelAccountSchema = z.object({
  email: z.string(),
  account_id: z.string(),
  channel: z.enum(['cpa', 'chatgpt']),
  signed_in: z.boolean(),
  expires_at: z.number().int(),
})

export type DeviMonChannelAccount = z.infer<typeof deviMonChannelAccountSchema>

export type DeviMonRouting = z.infer<typeof deviMonRoutingSchema>
export type DeviMonSnapshot = z.infer<typeof deviMonSnapshotSchema>
export type DeviMonResetOffer = z.infer<typeof deviMonResetOfferSchema>
export type DeviMonAccount = z.infer<typeof deviMonAccountSchema>
export type DeviMonState = z.infer<typeof deviMonStateSchema>
export type DeviMonSettings = z.infer<typeof deviMonSettingsSchema>
