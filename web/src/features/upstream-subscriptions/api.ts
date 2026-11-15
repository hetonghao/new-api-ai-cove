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

import { api } from '@/lib/api'
import { authRequestOptions } from '@/lib/secure-verification'
import { requireServerSuccess } from '@/lib/server-error-message'

import {
  upstreamSubscriptionSchema,
  subscriptionLoginSchema,
  subscriptionLoginResultSchema,
  subscriptionChannelSchema,
  type SubscriptionFormValues,
} from './types'

const BASE_PATH = '/api/upstream-subscriptions'

function subscriptionData<T>(payload: unknown, schema: z.ZodType<T>): T {
  const response = z
    .object({ success: z.literal(true), data: z.unknown() })
    .parse(requireServerSuccess(payload))
  return schema.parse(response.data)
}

export async function listUpstreamSubscriptions(signal?: AbortSignal) {
  const response = await api.get<unknown>(BASE_PATH, {
    signal,
    disableDuplicate: true,
  })
  return subscriptionData(response.data, z.array(upstreamSubscriptionSchema))
}

export async function listSubscriptionChannels(signal?: AbortSignal) {
  const response = await api.get<unknown>(`${BASE_PATH}/channels`, {
    signal,
    disableDuplicate: true,
  })
  return subscriptionData(response.data, z.array(subscriptionChannelSchema))
}

export async function saveUpstreamSubscription(
  id: number | undefined,
  values: Partial<SubscriptionFormValues> &
    Pick<SubscriptionFormValues, 'platform'>
) {
  const response =
    id === undefined
      ? await api.post<unknown>(BASE_PATH, values)
      : await api.put<unknown>(`${BASE_PATH}/${id}`, values)
  return subscriptionData(response.data, upstreamSubscriptionSchema)
}

export async function refreshUpstreamSubscription(id: number) {
  const response = await api.post<unknown>(`${BASE_PATH}/${id}/refresh`)
  return subscriptionData(response.data, upstreamSubscriptionSchema)
}

export async function deleteUpstreamSubscription(id: number) {
  const response = await api.delete<unknown>(`${BASE_PATH}/${id}`)
  requireServerSuccess(response.data)
}

export async function startOpenDesignLogin(
  name: string,
  refreshIntervalMinutes: number,
  subscriptionID?: number,
  channelID = 0,
  account = '',
  autoRefreshDisabled = false
) {
  const response = await api.post<unknown>(`${BASE_PATH}/opendesign/login`, {
    name,
    subscription_id: subscriptionID ?? 0,
    refresh_interval_minutes: refreshIntervalMinutes,
    channel_id: channelID,
    account,
    auto_refresh_disabled: autoRefreshDisabled,
  })
  return subscriptionData(response.data, subscriptionLoginSchema)
}

export async function pollOpenDesignLogin(token: string, signal?: AbortSignal) {
  const response = await api.post<unknown>(
    `${BASE_PATH}/opendesign/login/poll`,
    { flow_token: token },
    { ...authRequestOptions, signal, singleUseAuthorization: true }
  )
  return subscriptionData(response.data, subscriptionLoginResultSchema)
}

export async function cancelOpenDesignLogin(token: string) {
  const response = await api.post<unknown>(
    `${BASE_PATH}/opendesign/login/cancel`,
    { flow_token: token },
    { ...authRequestOptions, singleUseAuthorization: true }
  )
  requireServerSuccess(response.data)
}
