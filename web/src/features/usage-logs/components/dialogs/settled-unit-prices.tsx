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
import { ChevronDown } from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { StaticDataTable } from '@/components/data-table'
import { Button } from '@/components/ui/button'
import {
  Collapsible,
  CollapsibleContent,
  CollapsibleTrigger,
} from '@/components/ui/collapsible'
import { normalizeTierLabel } from '@/features/pricing/lib/billing-expr'
import { compileBillingExpression } from '@/features/pricing/lib/billing-expression/parser'
import { formatBillingCurrencyFromUSD } from '@/lib/currency'

import type { UsageLog } from '../../data/schema'
import { decodeBillingExprB64, getTieredBillingSummary } from '../../lib/format'
import type { LogOtherData } from '../../types'
import { SettledUsageCost } from './settled-usage-cost'

export function SettledUnitPrices(props: {
  other: LogOtherData
  log: UsageLog
}) {
  const { t } = useTranslation()
  const other = props.other
  if (other.is_task) return null

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
  const priceOptions = {
    digitsLarge: 4,
    digitsSmall: 6,
    abbreviate: false,
    minimumNonZero: 0.000001,
  }
  const rows = summary.priceEntries.map((entry) => ({
    ...entry,
    base: formatBillingCurrencyFromUSD(entry.price, priceOptions),
    final: formatBillingCurrencyFromUSD(
      entry.price * conditionRatio * groupRatio,
      priceOptions
    ),
  }))
  const unit = summary.priceEntries[0].unit
  const unitLabel = unit ? t(unit) : t('1M token')

  return (
    <section className='mt-4 min-w-0 space-y-3 border-t pt-4'>
      <div className='flex flex-wrap items-baseline justify-between gap-2'>
        <h3 className='text-xs font-semibold'>{t('Settled unit prices')}</h3>
        <span className='text-muted-foreground text-[10px]'>
          {t('Unit')}: USD / {unitLabel}
        </span>
      </div>
      <p className='text-muted-foreground text-xs'>
        {t(
          'Applied: conditional multiplier {{condition}}× · {{groupLabel}} {{group}}×',
          {
            condition: conditionRatio,
            groupLabel,
            group: groupRatio,
          }
        )}
      </p>
      <StaticDataTable
        className='rounded-none border-0 bg-transparent'
        tableProps={{ 'aria-label': t('Settled unit prices') }}
        tableClassName='table-fixed [&_tbody_tr]:h-10 [&_td]:text-xs [&_th]:text-xs [&_td:last-child]:font-semibold'
        data={rows}
        getRowKey={(row) => row.field}
        columns={[
          {
            id: 'item',
            header: t('Billing item'),
            className: 'w-[40%]',
            cell: (row) => t(row.shortLabel),
          },
          {
            id: 'base',
            header: t('Tier unit price'),
            className: 'text-right text-muted-foreground',
            cellClassName: 'text-right font-mono text-muted-foreground',
            cell: (row) => row.base,
          },
          {
            id: 'final',
            header: t('Final unit price'),
            className:
              'bg-emerald-500/10 text-right text-emerald-700 dark:text-emerald-300',
            cellClassName:
              'bg-emerald-500/10 text-right font-mono text-emerald-700 dark:text-emerald-300',
            cell: (row) => row.final,
          },
        ]}
      />
      <Collapsible>
        <CollapsibleTrigger
          render={<Button variant='ghost' size='sm' />}
          className='group text-muted-foreground -ml-2 text-xs'
        >
          <ChevronDown
            className='size-3 transition-transform group-data-[panel-open]:rotate-180'
            aria-hidden='true'
          />
          {t('View calculation')}
        </CollapsibleTrigger>
        <CollapsibleContent>
          <div className='bg-muted/50 mt-2 space-y-2 rounded-md p-3 text-xs'>
            <p className='text-muted-foreground'>
              {t(
                'Tier unit price × conditional multiplier × group ratio = final unit price'
              )}
            </p>
            {rows.map((row) => (
              <div
                key={row.field}
                className='flex flex-wrap justify-between gap-x-4 gap-y-1'
              >
                <span>{t(row.shortLabel)}</span>
                <span className='font-mono break-all tabular-nums'>
                  {row.base} × {conditionRatio} × {groupRatio} = {row.final}
                </span>
              </div>
            ))}
          </div>
        </CollapsibleContent>
      </Collapsible>
      {props.log.type === 2 && summary.tier.billingUnit !== 'request' && (
        <SettledUsageCost
          log={props.log}
          other={other}
          prices={rows.map((row) => ({
            field: row.field,
            label: row.shortLabel,
            price: row.price * conditionRatio * groupRatio,
          }))}
        />
      )}
    </section>
  )
}
