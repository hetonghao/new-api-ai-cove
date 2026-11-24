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

const deviMonAccountSchema = z.object({
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
  error: z.string(),
})

const deviMonDevinSchema = z.object({
  email: z.string(),
  disabled: z.boolean(),
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
})

export const deviMonSettingsSchema = z.object({
  interval_minutes: z.number().int().min(15).max(10080),
})

export type DeviMonSnapshot = z.infer<typeof deviMonSnapshotSchema>
export type DeviMonAccount = z.infer<typeof deviMonAccountSchema>
export type DeviMonState = z.infer<typeof deviMonStateSchema>
export type DeviMonSettings = z.infer<typeof deviMonSettingsSchema>
