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

export const riskRecordResultSchema = z.string().min(1)
export const riskContentSaveScopeSchema = z.enum(['all', 'unsafe', 'none'])
export const riskRecordRetentionDaysSchema = z.number().int().min(1).max(180)

export const riskRecordChunkSchema = z
  .object({
    index: z.number().int().nonnegative(),
    result: riskRecordResultSchema,
    categories: z.array(z.string()).readonly(),
    latency_ms: z.number().int().nonnegative(),
    prompt_tokens: z.number().int().nonnegative(),
    completion_tokens: z.number().int().nonnegative(),
    total_tokens: z.number().int().nonnegative(),
    neurons: z.number().int().nonnegative(),
  })
  .readonly()

export const riskRecordSchema = z
  .object({
    id: z.number().int().positive(),
    request_id: z.string().min(1),
    channel_id: z.number().int().nonnegative(),
    channel_name: z.string().default(''),
    user_id: z.number().int().positive(),
    username: z.string().default(''),
    token_id: z.number().int().nonnegative(),
    token_name: z.string().default(''),
    model: z.string(),
    path: z.string(),
    preview: z.string(),
    content_hash: z.string(),
    rule_ids: z.array(z.number().int().positive()).readonly(),
    provider_id: z.number().int().nonnegative(),
    provider_name: z.string(),
    result: riskRecordResultSchema,
    source: z.string().default(''),
    provider_called: z.boolean().default(false),
    cache_hit: z.boolean().default(false),
    blocked: z.boolean().default(false),
    categories: z.array(z.string()).readonly(),
    latency_ms: z.number().int().nonnegative(),
    prompt_tokens: z.number().int().nonnegative(),
    completion_tokens: z.number().int().nonnegative(),
    total_tokens: z.number().int().nonnegative(),
    neurons: z.number().int().nonnegative(),
    chunks: z.preprocess(
      (value) => value ?? [],
      z.array(riskRecordChunkSchema).readonly()
    ),
    error_code: z.string(),
    observed_at: z.string().min(1),
  })
  .readonly()

export const riskRecordPageSchema = z
  .object({
    items: z.array(riskRecordSchema).readonly(),
    total: z.number().int().nonnegative(),
    page: z.number().int().positive(),
    page_size: z.number().int().positive().max(100),
  })
  .readonly()

export const riskRecordResponseSchema = z
  .object({
    success: z.boolean(),
    message: z.string().optional(),
    data: riskRecordPageSchema.optional(),
  })
  .readonly()

export const riskRecordGovernanceSchema = z
  .object({
    save_scope: z.enum(['all', 'suspicious', 'unsafe']),
    content_save_scope: riskContentSaveScopeSchema,
    retention_days: riskRecordRetentionDaysSchema,
  })
  .readonly()

export const riskRecordGovernanceResponseSchema = z
  .object({
    success: z.boolean(),
    message: z.string().optional(),
    data: riskRecordGovernanceSchema.optional(),
  })
  .readonly()

export type RiskRecordResult = z.infer<typeof riskRecordResultSchema>
export type RiskRecordChunk = z.infer<typeof riskRecordChunkSchema>
export type RiskRecord = z.infer<typeof riskRecordSchema>
export type RiskRecordPage = z.infer<typeof riskRecordPageSchema>
export type RiskRecordResponse = z.infer<typeof riskRecordResponseSchema>
export type RiskContentSaveScope = z.infer<typeof riskContentSaveScopeSchema>
export type RiskRecordGovernance = z.infer<typeof riskRecordGovernanceSchema>
export type RiskRecordGovernanceResponse = z.infer<
  typeof riskRecordGovernanceResponseSchema
>

export type RiskRecordFilterDraft = {
  readonly start_time?: Date
  readonly end_time?: Date
  readonly channel_id: string
  readonly username: string
  readonly provider_id: string
  readonly result: string
  readonly source: string
}

export type RiskRecordFilters = {
  readonly start_timestamp?: number
  readonly end_timestamp?: number
  readonly channel_id?: number
  readonly username?: string
  readonly provider_id?: number
  readonly result?: string
  readonly source?: string
}
