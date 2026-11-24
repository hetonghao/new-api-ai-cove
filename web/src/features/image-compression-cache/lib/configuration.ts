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

import { getServerErrorSources } from '@/lib/server-error-message'

export const cacheFormSchema = z.object({
  enabled: z.boolean(),
  capacity: z.number().min(1).max(256),
  ttl: z.number().int().min(60).max(3600),
})

export type CacheForm = z.infer<typeof cacheFormSchema>

export function getCacheConfigErrorField(
  error: unknown
): 'capacity' | 'ttl' | 'root' {
  for (const source of getServerErrorSources(error)) {
    switch (source.message_key) {
      case 'Image compression cache capacity must be between 1 and 256 MiB':
        return 'capacity'
      case 'Image compression cache lifetime must be between 60 and 3600 seconds':
        return 'ttl'
    }
  }
  return 'root'
}
