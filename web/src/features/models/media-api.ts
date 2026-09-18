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
import { api } from '@/lib/api'

import type {
  GetMediaModelsPolicyResponse,
  MediaPolicy,
  UpdateMediaModelsPolicyResponse,
} from './lib/media-policy'

export const mediaModelsPolicyQueryKey = ['media-model-policy'] as const

export async function getMediaModelsPolicy(): Promise<GetMediaModelsPolicyResponse> {
  const res = await api.get('/api/option/media_models')
  return res.data
}

export async function updateMediaModelsPolicy(
  expectedVersion: string,
  policy: MediaPolicy
): Promise<UpdateMediaModelsPolicyResponse> {
  const res = await api.put('/api/option/media_models', {
    expected_version: expectedVersion,
    policy,
  })
  return res.data
}
