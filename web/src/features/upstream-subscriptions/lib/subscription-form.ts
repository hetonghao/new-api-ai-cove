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
  manual: boolean,
  bindingChannel: boolean
) {
  const intervalError = t('Refresh interval must be between 1 and 1440 minutes')
  return z
    .object({
      name: z.string().trim(),
      account: z
        .string()
        .trim()
        .refine(
          (value) => [...value].length <= 128,
          t('Subscription account must be at most 128 characters')
        ),
      channel_id: z.number().int().nonnegative(),
      platform: z.enum(['opencode', 'opendesign', 'commandcode']),
      credential: z.string().trim().max(4096),
      auto_refresh_disabled: z.boolean(),
      refresh_interval_minutes: z
        .number({ error: intervalError })
        .int(intervalError)
        .min(1, intervalError)
        .max(1440, intervalError),
    })
    .superRefine((values, context) => {
      if (bindingChannel && values.channel_id === 0) {
        context.addIssue({
          code: 'custom',
          path: ['channel_id'],
          message: t('Select a channel'),
        })
      }
      if (!bindingChannel && [...values.name].length > 128) {
        context.addIssue({
          code: 'custom',
          path: ['name'],
          message: t('Subscription name must be at most 128 characters'),
        })
      }
      const required = !editing && (values.platform !== 'opendesign' || manual)
      const prefix = values.platform === 'opencode' ? 'oc_sk_' : 'vela_ctrl_'
      const validCredential =
        values.platform === 'commandcode'
          ? /^[\x21-\x7e]+$/.test(values.credential)
          : values.credential.startsWith(prefix) &&
            values.credential.length > prefix.length &&
            !/\s/.test(values.credential)
      if (
        (required && !values.credential) ||
        (values.credential && !validCredential)
      ) {
        context.addIssue({
          code: 'custom',
          path: ['credential'],
          message:
            values.platform === 'commandcode'
              ? t('Use a valid CommandCode API key')
              : t(
                  'Use the platform subscription credential, not an inference key'
                ),
        })
      }
    })
}
