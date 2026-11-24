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

const base = '/api/image-compression-cache'
export const MIB = 1024 * 1024
export const configKey = ['image-compression-cache', 'config'] as const
export const nodesKey = ['image-compression-cache', 'nodes'] as const
export const statsKey = ['image-compression-cache', 'stats'] as const

const count = z.number().int().nonnegative()
const configSchema = z.object({
  enabled: z.boolean(),
  max_bytes: count,
  ttl_seconds: count,
})
const entrySchema = z.object({
  id: z.string().min(1),
  user_id: count,
  original_mime: z.string(),
  compressed_mime: z.string(),
  original_bytes: count,
  compressed_bytes: count,
  hits: count,
  created_at: count,
  last_hit_at: count.nullish(),
  expires_at: count,
})
const statsSchema = configSchema.extend({
  node_name: z.string().min(1),
  instance_id: z.string().min(1),
  used_bytes: count,
  entry_count: count,
  hits: count,
  misses: count,
  evictions: count,
  expirations: count,
  removals: count,
  entries: z.array(entrySchema),
})
const nodesSchema = z.object({
  local_node_name: z.string().min(1),
  nodes: z.array(
    z.object({
      node_name: z.string().min(1),
      status: z.string(),
    })
  ),
})

export type CacheConfig = z.infer<typeof configSchema>
export type CacheEntry = z.infer<typeof entrySchema>
export type CacheStats = z.infer<typeof statsSchema>
export type ClearTarget = {
  readonly node_name: string
  readonly instance_id: string
  readonly ids: readonly string[]
}

function readData<T>(response: unknown, schema: z.ZodType<T>): T {
  return z
    .object({ success: z.literal(true), data: schema })
    .parse(requireServerSuccess(response)).data
}

export async function getCacheConfig(): Promise<CacheConfig> {
  const response = await api.get<unknown>(`${base}/config`)
  return readData(response.data, configSchema)
}

export async function saveCacheConfig(
  config: CacheConfig
): Promise<CacheConfig> {
  const response = await api.put<unknown>(`${base}/config`, config)
  return readData(response.data, configSchema)
}

export async function listCacheNodes(): Promise<z.infer<typeof nodesSchema>> {
  const response = await api.get<unknown>(`${base}/nodes`)
  return readData(response.data, nodesSchema)
}

export async function getCacheStats(
  params: {
    readonly node_name: string
    readonly page: number
    readonly page_size: number
  },
  signal: AbortSignal
): Promise<CacheStats> {
  const response = await api.get<unknown>(`${base}/stats`, {
    params,
    signal,
    disableDuplicate: true,
  })
  return readData(response.data, statsSchema)
}

export async function clearCacheEntries(target: ClearTarget): Promise<number> {
  const response = await api.post<unknown>(`${base}/clear`, target)
  return readData(response.data, z.object({ deleted: count })).deleted
}
