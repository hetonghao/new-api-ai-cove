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
import { useTranslation } from 'react-i18next'

import { normalizeTierLabel } from '@/features/pricing/lib/billing-expr'
import { compileBillingExpression } from '@/features/pricing/lib/billing-expression/parser'

import type { UsageLog } from '../../data/schema'
import {
  billedKeysForFields,
  resolveBilledTokenCounts,
} from '../../lib/billed-tokens'
import { decodeBillingExprB64, getTieredBillingSummary } from '../../lib/format'
import {
  resolveRatioSettledPlan,
  type SettledUsageRow,
} from '../../lib/settled-ratio-prices'
import type { LogOtherData } from '../../types'
import { SettledPricePanel } from './settled-price-panel'

/**
 * Final unit prices of a settled log entry.
 *
 * Expression-billed requests rebuild them from the matched tier and the
 * multipliers the settlement recorded. Ratio-billed requests rebuild them from
 * the model ratios their log entry recorded; when that is not reproducible the
 * panel stays absent instead of showing guessed prices.
 */
export function SettledUnitPrices(props: {
  other: LogOtherData
  log: UsageLog
}) {
  const { t } = useTranslation()
  const other = props.other
  if (other.is_task) return null

  if (other.billing_mode !== 'tiered_expr') {
    const plan = resolveRatioSettledPlan({
      other,
      promptTokens: props.log.prompt_tokens,
      completionTokens: props.log.completion_tokens,
      withUsageCost: props.log.type === 2,
    })
    if (!plan) return null

    return (
      <SettledPricePanel
        log={props.log}
        unitLabel={t(plan.unitLabel)}
        baseHeader={t('Base unit price')}
        appliedNote={t('Applied: {{groupLabel}} {{group}}×', {
          groupLabel: t(plan.groupLabel),
          group: plan.groupRatio,
        })}
        formulaNote={t('Base unit price × group ratio = final unit price')}
        multipliers={[plan.groupRatio]}
        rows={plan.rows}
        usage={plan.usage}
      />
    )
  }

  const summary = getTieredBillingSummary(other)
  const compiled = compileBillingExpression(
    decodeBillingExprB64(other.expr_b64)
  )
  const hasUserRatio =
    other.user_group_ratio != null && other.user_group_ratio !== -1
  const groupRatio = hasUserRatio ? other.user_group_ratio : other.group_ratio
  const matchedRules = (other.request_rules ?? []).filter(
    (rule) => rule.matched === true
  )
  const conditionRatio = matchedRules.reduce(
    (ratio, rule) => ratio * rule.multiplier,
    1
  )
  const matchingTiers =
    summary?.tiers.filter(
      (tier) =>
        normalizeTierLabel(tier.label) ===
          normalizeTierLabel(other.matched_tier) &&
        (tier.billingUnit ?? 'token') === (other.billing_unit ?? 'token')
    ) ?? []
  const hasRecordedFixedPrice =
    other.billing_unit === 'request' &&
    typeof other.fixed_price === 'number' &&
    Number.isFinite(other.fixed_price) &&
    other.fixed_price >= 0
  const canCalculate =
    summary &&
    summary.priceEntries.length > 0 &&
    (hasRecordedFixedPrice ||
      (matchingTiers.length === 1 && matchingTiers[0] === summary.tier)) &&
    compiled.status === 'ready' &&
    (compiled.requestRules.length === 0 || other.request_rules != null) &&
    typeof groupRatio === 'number' &&
    Number.isFinite(groupRatio) &&
    groupRatio >= 0 &&
    matchedRules.every(
      (rule) => Number.isFinite(rule.multiplier) && rule.multiplier >= 0
    ) &&
    Number.isFinite(conditionRatio) &&
    summary.priceEntries.every((entry) =>
      Number.isFinite(entry.price * conditionRatio * groupRatio)
    )

  if (!canCalculate) {
    return (
      <section className='mt-4 space-y-2 border-t pt-4'>
        <h3 className='text-xs font-semibold'>{t('Settled unit prices')}</h3>
        <p className='text-muted-foreground text-xs'>
          {t(
            'Historical billing data is incomplete; final unit prices are unavailable.'
          )}
        </p>
      </section>
    )
  }

  const groupLabel = hasUserRatio ? t('User Exclusive Ratio') : t('Group Ratio')
  const rows = summary.priceEntries.map((entry) => ({
    id: entry.field,
    label: entry.shortLabel,
    base: entry.price,
    final: entry.price * conditionRatio * groupRatio,
  }))
  const unit = summary.priceEntries[0].unit
  const pricedKeys = billedKeysForFields(rows.map((row) => row.id))
  const counts =
    pricedKeys.length === rows.length && rows.length > 0
      ? resolveBilledTokenCounts({
          promptTokens: props.log.prompt_tokens,
          completionTokens: props.log.completion_tokens,
          other,
          pricedKeys,
        })
      : null
  const usageRows: SettledUsageRow[] | null = counts
    ? rows.map((row, index) => ({
        id: row.id,
        label: row.label,
        count: counts[pricedKeys[index]],
        price: row.final,
      }))
    : null

  return (
    <SettledPricePanel
      log={props.log}
      unitLabel={unit ? t(unit) : t('1M token')}
      baseHeader={t('Tier unit price')}
      appliedNote={t(
        'Applied: conditional multiplier {{condition}}× · {{groupLabel}} {{group}}×',
        {
          condition: conditionRatio,
          groupLabel,
          group: groupRatio,
        }
      )}
      formulaNote={t(
        'Tier unit price × conditional multiplier × group ratio = final unit price'
      )}
      multipliers={[conditionRatio, groupRatio]}
      rows={rows}
      usage={
        props.log.type === 2 && summary.tier.billingUnit !== 'request'
          ? usageRows
          : undefined
      }
    />
  )
}
