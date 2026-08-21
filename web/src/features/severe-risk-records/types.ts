import { z } from 'zod'

export const severeRiskActionStatusSchema = z.enum([
  'pending',
  'success',
  'failed',
  'disabled',
])

export const severeRiskRecordSchema = z.object({
  id: z.number().int().positive(),
  request_id: z.string(),
  channel_id: z.number().int().positive(),
  channel_name: z.string(),
  user_id: z.number().int().positive(),
  username: z.string(),
  token_id: z.number().int().positive(),
  token_name: z.string(),
  model: z.string(),
  path: z.string(),
  error_code: z.string(),
  error_detail: z.string(),
  context_hash: z.string(),
  channel_scope: z.enum(['all', 'key']),
  channel_key_fingerprint: z.string().optional(),
  user_action_status: severeRiskActionStatusSchema,
  channel_action_status: severeRiskActionStatusSchema,
  triggered_at: z.string(),
})

export const severeRiskRecordPageSchema = z.object({
  items: z.array(severeRiskRecordSchema),
  total: z.number().int().nonnegative(),
  page: z.number().int().positive(),
  page_size: z.number().int().positive(),
})

export const severeRiskRecordResponseSchema = z.object({
  success: z.boolean(),
  message: z.string().optional(),
  data: severeRiskRecordPageSchema.optional(),
})

export const severeRiskRecordDetailResponseSchema = z.object({
  success: z.boolean(),
  message: z.string().optional(),
  data: z
    .object({
      record: severeRiskRecordSchema,
      context: z.string(),
    })
    .optional(),
})

export type SevereRiskRecord = z.infer<typeof severeRiskRecordSchema>
export type SevereRiskActionStatus = z.infer<
  typeof severeRiskActionStatusSchema
>
export type SevereRiskRecordResponse = z.infer<
  typeof severeRiskRecordResponseSchema
>
export type SevereRiskRecordDetailResponse = z.infer<
  typeof severeRiskRecordDetailResponseSchema
>
