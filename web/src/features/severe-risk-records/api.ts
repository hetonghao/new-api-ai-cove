import { api } from '@/lib/api'

import {
  severeRiskRecordDetailResponseSchema,
  severeRiskRecordResponseSchema,
  type SevereRiskRecordDetailResponse,
  type SevereRiskRecordResponse,
} from './types'

export class SevereRiskRecordResponseError extends Error {
  readonly name = 'SevereRiskRecordResponseError'
}

export async function listSevereRiskRecords(
  page: number,
  pageSize: number
): Promise<SevereRiskRecordResponse> {
  const response = await api.get<unknown>('/api/risk/severe-records', {
    params: { p: page, page_size: pageSize },
    skipErrorHandler: true,
  })
  const parsed = severeRiskRecordResponseSchema.safeParse(response.data)
  if (!parsed.success) {
    throw new SevereRiskRecordResponseError(
      'Invalid severe risk record response'
    )
  }
  return parsed.data
}

export async function getSevereRiskRecord(
  id: number
): Promise<SevereRiskRecordDetailResponse> {
  const response = await api.get<unknown>(`/api/risk/severe-records/${id}`, {
    skipErrorHandler: true,
  })
  const parsed = severeRiskRecordDetailResponseSchema.safeParse(response.data)
  if (!parsed.success) {
    throw new SevereRiskRecordResponseError(
      'Invalid severe risk record detail response'
    )
  }
  return parsed.data
}
