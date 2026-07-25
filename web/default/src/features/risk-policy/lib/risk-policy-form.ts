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
import * as z from 'zod'

import {
  LOCAL_RISK_RULE_TYPES,
  RISK_ACTION_MODES,
  RISK_REVIEW_MODES,
  type LocalRiskRule,
  type LocalRiskRulePayload,
  type LocalRiskRuleTestPayload,
  type RiskPolicy,
  type RiskPolicyPayload,
} from '../types.ts'

type Translate = (key: string) => string

export function createRiskPolicyFormSchema(
  validatedProviderIds: readonly number[],
  t: Translate
) {
  return z
    .object({
      enabled: z.boolean(),
      provider_id: z.string(),
      review_mode: z.enum(RISK_REVIEW_MODES),
      action_mode: z.enum(RISK_ACTION_MODES),
    })
    .superRefine((values, context) => {
      if (!values.enabled) return

      const providerId = Number(values.provider_id)
      if (
        !Number.isInteger(providerId) ||
        !validatedProviderIds.includes(providerId)
      ) {
        context.addIssue({
          code: 'custom',
          path: ['provider_id'],
          message: t('Select a validated provider'),
        })
      }
    })
}

export type RiskPolicyFormValues = z.infer<
  ReturnType<typeof createRiskPolicyFormSchema>
>

export function riskPolicyToFormValues(
  policy: RiskPolicy
): RiskPolicyFormValues {
  return {
    enabled: policy.enabled,
    provider_id: policy.provider_id === null ? '' : String(policy.provider_id),
    review_mode: policy.review_mode,
    action_mode: policy.action_mode,
  }
}

export function riskPolicyFormValuesToPayload(
  values: RiskPolicyFormValues
): RiskPolicyPayload {
  return {
    provider_id: values.enabled ? Number(values.provider_id) : null,
    enabled_channels: values.enabled ? ['cpa-pro'] : [],
    review_mode: values.review_mode,
    action_mode: values.action_mode,
  }
}

export function createLocalRuleFormSchema(t: Translate) {
  return z.object({
    rule_type: z.enum(LOCAL_RISK_RULE_TYPES),
    pattern: z.string().trim().min(1, t('Rule pattern is required')),
    enabled: z.boolean(),
  })
}

export type LocalRiskRuleFormValues = z.infer<
  ReturnType<typeof createLocalRuleFormSchema>
>

export function localRuleToFormValues(
  rule: LocalRiskRule | null
): LocalRiskRuleFormValues {
  return {
    rule_type: rule?.rule_type ?? 'keyword',
    pattern: rule?.pattern ?? '',
    enabled: rule?.enabled ?? true,
  }
}

export function localRuleFormValuesToPayload(
  values: LocalRiskRuleFormValues
): LocalRiskRulePayload {
  return {
    rule_type: values.rule_type,
    pattern: values.pattern.trim(),
    enabled: values.enabled,
  }
}

export function createLocalRuleTestFormSchema(t: Translate) {
  return z.object({
    rule_type: z.enum(LOCAL_RISK_RULE_TYPES),
    pattern: z.string().trim().min(1, t('Rule pattern is required')),
    text: z
      .string()
      .refine((value) => value.trim().length > 0, t('Test text is required')),
  })
}

export type LocalRiskRuleTestFormValues = z.infer<
  ReturnType<typeof createLocalRuleTestFormSchema>
>

export function localRuleTestToFormValues(
  rule: LocalRiskRule | null
): LocalRiskRuleTestFormValues {
  return {
    rule_type: rule?.rule_type ?? 'keyword',
    pattern: rule?.pattern ?? '',
    text: '',
  }
}

export function localRuleTestFormValuesToPayload(
  values: LocalRiskRuleTestFormValues
): LocalRiskRuleTestPayload {
  return {
    rule_type: values.rule_type,
    pattern: values.pattern.trim(),
    text: values.text,
  }
}
