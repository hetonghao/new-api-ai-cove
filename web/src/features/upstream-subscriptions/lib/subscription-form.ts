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

import type { TFunction } from 'i18next'
import { z } from 'zod'

export function subscriptionFormSchema(
  t: TFunction,
  editing: boolean,
  manual: boolean
) {
  const intervalError = t('Refresh interval must be between 1 and 1440 minutes')
  return z
    .object({
      name: z
        .string()
        .trim()
        .max(128, t('Subscription name must be at most 128 characters')),
      platform: z.enum(['opencode', 'opendesign']),
      credential: z.string().trim().max(4096),
      refresh_interval_minutes: z
        .number({ error: intervalError })
        .int(intervalError)
        .min(1, intervalError)
        .max(1440, intervalError),
    })
    .superRefine((values, context) => {
      const required = !editing && (values.platform === 'opencode' || manual)
      const prefix = values.platform === 'opencode' ? 'oc_sk_' : 'vela_ctrl_'
      if (
        (required && !values.credential) ||
        (values.credential &&
          (!values.credential.startsWith(prefix) ||
            values.credential.length <= prefix.length ||
            /\s/.test(values.credential)))
      ) {
        context.addIssue({
          code: 'custom',
          path: ['credential'],
          message: t(
            'Use the platform subscription credential, not an inference key'
          ),
        })
      }
    })
}
