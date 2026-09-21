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
import { createFileRoute, redirect } from '@tanstack/react-router'
import z from 'zod'

import { ModelQuality } from '@/features/model-quality'
import { useAuthStore } from '@/stores/auth-store'

const modelQualitySearchSchema = z.object({
  tab: z.enum(['panel', 'cases', 'batches']).catch('panel'),
  case: z.number().int().positive().optional().catch(undefined),
  // -2 = all channels aggregated; >= 0 selects one channel
  channel: z.number().int().min(-2).optional().catch(undefined),
})

export const Route = createFileRoute('/_authenticated/model-quality/')({
  beforeLoad: () => {
    // Any authenticated user may enter; the page itself hides operator-only
    // tabs and renders an unavailable state when the public panel is off.
    const { auth } = useAuthStore.getState()
    if (!auth.user) {
      throw redirect({ to: '/403' })
    }
  },
  validateSearch: modelQualitySearchSchema,
  component: ModelQuality,
})
