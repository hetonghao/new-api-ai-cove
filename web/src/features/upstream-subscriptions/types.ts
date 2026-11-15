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

export const subscriptionChannelSchema = z.object({
  id: z.number().int().positive(),
  name: z.string(),
  priority: z.number().int(),
  weight: z.number().int().nonnegative(),
})

const quotaWindowSchema = z.object({
  key: z.enum(['rolling', 'weekly', 'monthly']),
  duration_seconds: z.number().int().nonnegative(),
  used_percent: z.number().finite().nonnegative().nullable(),
  status: z.string(),
  used_credits: z.string().regex(/^\d+$/).optional(),
  limit_credits: z.string().regex(/^\d+$/).optional(),
  remaining_credits: z.string().regex(/^\d+$/).optional(),
  resets_at: z.iso.datetime({ offset: true }).nullable(),
})

export const upstreamSubscriptionSchema = z.object({
  id: z.number().int().positive(),
  name: z.string(),
  channel_id: z.number().int().nonnegative().optional(),
  account: z.string().optional(),
  channel: subscriptionChannelSchema.nullable().optional(),
  platform: z.enum(['opencode', 'opendesign']),
  refresh_interval_minutes: z.number().int().min(1).max(1440).default(10),
  auto_refresh_disabled: z.boolean().default(false),
  last_success_at: z.number().int(),
  last_attempt_at: z.number().int(),
  next_refresh_at: z.number().int(),
  error_code: z.string(),
  stale: z.boolean(),
  quota: z
    .object({
      windows: z.array(quotaWindowSchema),
      workspace_id: z.string().optional(),
      workspace_name: z.string().optional(),
      tier: z.string().optional(),
      eligible: z.boolean().optional(),
      balance_usd: z.string().optional(),
      subscription_period_end: z.iso.datetime({ offset: true }).optional(),
    })
    .nullable(),
})

export const subscriptionLoginSchema = z.object({
  flow_token: z.string().min(1),
  activation_url: z.url().refine((value) => {
    const url = new URL(value)
    return (
      url.origin === 'https://open-design.ai' &&
      url.pathname === '/cloud/cli/activate'
    )
  }),
  user_code: z.string().min(1),
  expires_at: z.iso.datetime({ offset: true }),
  poll_interval: z.number().int().min(2).max(60),
})

export const subscriptionLoginResultSchema = z.discriminatedUnion('status', [
  z.object({ status: z.literal('pending') }),
  z.object({ status: z.literal('expired') }),
  z.object({
    status: z.literal('approved'),
    subscription: upstreamSubscriptionSchema,
  }),
])

export type UpstreamSubscription = z.infer<typeof upstreamSubscriptionSchema>
export type SubscriptionQuotaWindow = z.infer<typeof quotaWindowSchema>
export type SubscriptionLogin = z.infer<typeof subscriptionLoginSchema>
export type SubscriptionChannel = z.infer<typeof subscriptionChannelSchema>

export type SubscriptionFormValues = {
  readonly name: string
  readonly account: string
  readonly channel_id: number
  readonly platform: 'opencode' | 'opendesign'
  readonly credential: string
  readonly refresh_interval_minutes: number
  readonly auto_refresh_disabled: boolean
}
