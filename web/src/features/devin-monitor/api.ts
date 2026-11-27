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
import { requireServerSuccess } from '@/lib/server-error-message'

import {
  deviMonAccountSchema,
  deviMonChannelAccountSchema,
  deviMonDeviceCodeSchema,
  deviMonSettingsSchema,
  deviMonStateSchema,
  type DeviMonSettings,
} from './types'

const BASE_PATH = '/api/devin-monitor'

function deviMonData<T>(payload: unknown, schema: z.ZodType<T>): T {
  const response = z
    .object({ success: z.literal(true), data: z.unknown() })
    .parse(requireServerSuccess(payload))
  return schema.parse(response.data)
}

export const deviMonQueryKey = ['devin-monitor'] as const
export const deviMonSettingsKey = ['devin-monitor', 'config'] as const
export const deviMonAccountsKey = ['devin-monitor', 'accounts'] as const

export async function getDeviMonState(signal?: AbortSignal) {
  const response = await api.get<unknown>(BASE_PATH, {
    signal,
    disableDuplicate: true,
  })
  return deviMonData(response.data, deviMonStateSchema)
}

export async function collectDeviMon() {
  const response = await api.post<unknown>(`${BASE_PATH}/collect`)
  return deviMonData(response.data, deviMonStateSchema)
}

export async function refreshDeviMonAccount(email: string) {
  const response = await api.post<unknown>(
    `${BASE_PATH}/accounts/refresh`,
    { email },
    { skipErrorHandler: true, skipBusinessError: true }
  )
  deviMonData(response.data, z.null())
}

export async function setDeviMonAccountAutoReset(
  email: string,
  resetKey: string,
  enabled: boolean
): Promise<void> {
  const response = await api.put<unknown>(
    `${BASE_PATH}/accounts/auto-reset`,
    { email, reset_key: resetKey, enabled },
    { skipErrorHandler: true, skipBusinessError: true }
  )
  deviMonData(response.data, z.null())
}

export async function confirmDeviMonAccountReset(
  email: string,
  offerId: string
) {
  const response = await api.post<unknown>(
    `${BASE_PATH}/accounts/reset`,
    { email, offer_id: offerId },
    { skipErrorHandler: true, skipBusinessError: true }
  )
  return deviMonData(response.data, deviMonAccountSchema)
}

export async function getDeviMonChannelAccounts(signal?: AbortSignal) {
  const response = await api.get<unknown>(`${BASE_PATH}/accounts`, {
    signal,
    disableDuplicate: true,
  })
  return deviMonData(response.data, z.array(deviMonChannelAccountSchema))
}

export async function setDeviMonAccountChannel(
  email: string,
  channel: 'cpa' | 'chatgpt'
) {
  const response = await api.put<unknown>(
    `${BASE_PATH}/accounts/channel`,
    { email, channel },
    { skipErrorHandler: true, skipBusinessError: true }
  )
  deviMonData(response.data, z.null())
}

export async function startDeviMonChatGPTLogin() {
  const response = await api.post<unknown>(
    `${BASE_PATH}/accounts/chatgpt/login`
  )
  return deviMonData(response.data, deviMonDeviceCodeSchema)
}

export async function pollDeviMonChatGPTLogin(
  deviceAuthId: string,
  userCode: string
) {
  const response = await api.post<unknown>(
    `${BASE_PATH}/accounts/chatgpt/login/poll`,
    { device_auth_id: deviceAuthId, user_code: userCode },
    { skipErrorHandler: true, skipBusinessError: true }
  )
  return deviMonData(
    response.data,
    z.object({ done: z.boolean(), email: z.string() })
  )
}

export async function forgetDeviMonChatGPTAccount(email: string) {
  const response = await api.delete<unknown>(`${BASE_PATH}/accounts/chatgpt`, {
    data: { email },
    skipErrorHandler: true,
    skipBusinessError: true,
  })
  deviMonData(response.data, z.null())
}

export async function getDeviMonSettings(signal?: AbortSignal) {
  const response = await api.get<unknown>(`${BASE_PATH}/config`, {
    signal,
    disableDuplicate: true,
  })
  return deviMonData(response.data, deviMonSettingsSchema)
}

export async function saveDeviMonSettings(settings: DeviMonSettings) {
  const response = await api.put<unknown>(`${BASE_PATH}/config`, settings)
  return deviMonData(response.data, deviMonSettingsSchema)
}
