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
import { Minus, Plus } from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { StaticDataTable } from '@/components/data-table'
import { Button } from '@/components/ui/button'
import {
  Collapsible,
  CollapsibleContent,
  CollapsibleTrigger,
} from '@/components/ui/collapsible'
import { toIntlLocale } from '@/i18n/languages'
import {
  formatBillingCurrencyFromUSD,
  getCurrencyDisplay,
} from '@/lib/currency'

import type { UsageLog } from '../../data/schema'
import {
  billedKeysForFields,
  resolveBilledTokenCounts,
} from '../../lib/billed-tokens'
import type { LogOtherData } from '../../types'

const amountOptions = {
  digitsLarge: 8,
  digitsSmall: 8,
  abbreviate: false,
  minimumNonZero: 0.00000001,
}

export function SettledUsageCost(props: {
  log: UsageLog
  other: LogOtherData
  multiplierLabel: string
  prices: Array<{
    field: string
    label: string
    price: number
    unit?: string
    quantityUnit?: string
    divisor?: number
  }>
}) {
  const { t, i18n } = useTranslation()
  const locale = toIntlLocale(i18n.resolvedLanguage || i18n.language)
  const quantityFormat = new Intl.NumberFormat(locale, {
    maximumFractionDigits: 20,
  })
  const isTask = props.other.is_task === true
  const fields = props.prices.map((entry) => entry.field)
  const pricedKeys = isTask ? fields : billedKeysForFields(fields)
  const counts = isTask
    ? props.other.usage_facts
    : resolveBilledTokenCounts({
        promptTokens: props.log.prompt_tokens,
        completionTokens: props.log.completion_tokens,
        other: props.other,
        pricedKeys,
      })
  const recordedCost =
    props.log.quota / getCurrencyDisplay().config.quotaPerUnit
  const quantities = props.prices.map((entry, index) =>
    isTask && entry.field === 'constant' ? 1 : counts?.[pricedKeys[index]]
  )
  const complete =
    pricedKeys.length === props.prices.length &&
    props.prices.length > 0 &&
    quantities.every(
      (count, index) =>
        typeof count === 'number' &&
        Number.isFinite(count) &&
        count >= 0 &&
        Number.isFinite(
          (props.prices[index].price /
            (props.prices[index].divisor ?? 1_000_000)) *
            count
        )
    )
  const rows = complete
    ? props.prices.map((entry, index) => {
        const count = Number(quantities[index])
        const subtotal = (entry.price / (entry.divisor ?? 1_000_000)) * count
        return {
          ...entry,
          count,
          subtotal,
          formattedPrice: formatBillingCurrencyFromUSD(
            entry.price,
            amountOptions
          ),
          formattedSubtotal: formatBillingCurrencyFromUSD(
            subtotal,
            amountOptions
          ),
        }
      })
    : []
  const tokenTotal = rows.reduce((sum, row) => sum + row.subtotal, 0)
  const calculated = formatBillingCurrencyFromUSD(tokenTotal, amountOptions)
  const recorded = formatBillingCurrencyFromUSD(recordedCost, amountOptions)
  // Quota is stored as a rounded integer, so an exact match is not expected;
  // only a real divergence (tool surcharges, add-ons) is worth calling out.
  const diverges =
    complete &&
    Math.abs(tokenTotal - recordedCost) >
      Math.max(0.000001, Math.abs(tokenTotal) * 0.01)

  return (
    <section
      aria-label={t('Usage cost')}
      className='min-w-0 space-y-3 border-t pt-4'
    >
      <div className='flex flex-wrap items-baseline justify-between gap-2'>
        <h3 className='text-xs font-semibold'>{t('Usage cost')}</h3>
        {!isTask && (
          <span className='text-muted-foreground text-[10px]'>
            {t('Unit')}: USD / {t('1M token')}
          </span>
        )}
      </div>
      {complete ? (
        <StaticDataTable
          className='rounded-none border-0 bg-transparent'
          tableProps={{ 'aria-label': t('Usage cost') }}
          tableClassName='table-fixed [&_td]:break-words [&_td]:whitespace-normal [&_td]:text-xs [&_th]:text-xs [&_th]:whitespace-normal'
          data={rows}
          getRowKey={(row) => row.field}
          columns={[
            {
              id: 'item',
              header: t('Billing item'),
              className: 'w-1/4',
              cell: (row) => t(row.label),
            },
            {
              id: 'usage',
              header: t(isTask ? 'Usage' : 'Token usage'),
              className: 'w-1/4 text-right',
              cellClassName: 'text-right font-mono tabular-nums',
              cell: (row) =>
                `${quantityFormat.format(row.count)}${isTask ? ` ${row.quantityUnit}` : ''}`,
            },
            {
              id: 'price',
              header: `${t('Final unit price')} ${props.multiplierLabel}`,
              className: 'w-1/4 text-right',
              cellClassName: 'text-right font-mono tabular-nums',
              cell: (row) =>
                isTask ? (
                  <span className='break-all'>
                    {row.formattedPrice}/{row.unit}
                  </span>
                ) : (
                  row.formattedPrice
                ),
            },
            {
              id: 'subtotal',
              header: t('Subtotal'),
              className: 'w-1/4 text-right',
              cellClassName:
                'text-right font-mono font-semibold tabular-nums text-emerald-700 dark:text-emerald-300',
              cell: (row) => row.formattedSubtotal,
            },
          ]}
        />
      ) : (
        <p className='text-muted-foreground text-xs'>
          {t(
            isTask
              ? 'Recorded billable usage is incomplete; the cost formula is unavailable.'
              : 'Recorded billable tokens are incomplete; the cost formula is unavailable.'
          )}
        </p>
      )}
      <div className='flex flex-wrap items-baseline justify-between gap-2 text-emerald-700 dark:text-emerald-300'>
        <span className='text-xs font-semibold'>{t('Total usage cost')}</span>
        <strong className='font-mono text-base tabular-nums'>{recorded}</strong>
      </div>
      {complete && (
        <Collapsible>
          <CollapsibleTrigger
            render={<Button variant='ghost' size='sm' />}
            className='group text-muted-foreground -ml-2 text-xs'
          >
            <Plus
              className='size-3 group-data-[panel-open]:hidden'
              aria-hidden='true'
            />
            <Minus
              className='hidden size-3 group-data-[panel-open]:block'
              aria-hidden='true'
            />
            {t('View usage cost formula')}
          </CollapsibleTrigger>
          <CollapsibleContent>
            <div className='bg-muted/50 mt-2 space-y-2 rounded-md p-3 text-xs'>
              <p className='text-muted-foreground'>
                {t(
                  isTask
                    ? 'Final unit price × usage = subtotal'
                    : 'Final unit price ÷ 1,000,000 × token usage = subtotal'
                )}
              </p>
              {rows.map((row) => (
                <div
                  key={row.field}
                  className='flex flex-wrap justify-between gap-x-4 gap-y-1'
                >
                  <span>{t(row.label)}</span>
                  <span className='font-mono break-all tabular-nums'>
                    {row.formattedPrice}
                    {row.divisor === 1 ? '' : ' ÷ 1,000,000'} ×{' '}
                    {quantityFormat.format(row.count)}
                    {isTask ? ` ${row.quantityUnit}` : ''} ={' '}
                    {row.formattedSubtotal}
                  </span>
                </div>
              ))}
              <p className='border-t pt-2 font-mono break-all tabular-nums'>
                {t(isTask ? 'Subtotal' : 'Token cost subtotal')}:{' '}
                {rows.map((row) => row.formattedSubtotal).join(' + ')} ={' '}
                {calculated}
              </p>
              <p className='text-muted-foreground'>
                {t(
                  isTask
                    ? 'Uses recorded billable usage and final unit prices; multipliers are not applied again.'
                    : 'Uses recorded billable tokens and final unit prices; multipliers are not applied again.'
                )}
              </p>
              {diverges && (
                <p className='text-muted-foreground'>
                  {t(
                    isTask
                      ? 'Usage subtotals differ from the recorded charge. Rounding or additional charges may apply; the log charge is authoritative.'
                      : 'Token subtotals differ from the recorded charge. Rounding or additional charges may apply; the log charge is authoritative.'
                  )}
                </p>
              )}
            </div>
          </CollapsibleContent>
        </Collapsible>
      )}
    </section>
  )
}
