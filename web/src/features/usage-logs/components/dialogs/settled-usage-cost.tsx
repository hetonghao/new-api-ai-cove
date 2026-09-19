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
import {
  formatBillingCurrencyFromUSD,
  getCurrencyDisplay,
} from '@/lib/currency'

import type { UsageLog } from '../../data/schema'
import type { SettledUsageRow } from '../../lib/settled-ratio-prices'

const amountOptions = {
  digitsLarge: 8,
  digitsSmall: 8,
  abbreviate: false,
  minimumNonZero: 0.00000001,
}

/**
 * Usage behind a settled charge: the billed token counts resolved for the log,
 * paired with the final unit prices the settlement applied. Callers pass null
 * when those counts cannot be rebuilt from the log entry.
 */
export function SettledUsageCost(props: {
  log: UsageLog
  rows: SettledUsageRow[] | null
}) {
  const { t } = useTranslation()
  const recordedCost = props.log.quota / getCurrencyDisplay().config.quotaPerUnit
  const rows =
    props.rows === null
      ? []
      : props.rows.map((row) => {
          const subtotal = (row.price / 1_000_000) * row.count
          return {
            ...row,
            subtotal,
            formattedPrice: formatBillingCurrencyFromUSD(
              row.price,
              amountOptions
            ),
            formattedSubtotal: formatBillingCurrencyFromUSD(
              subtotal,
              amountOptions
            ),
          }
        })
  const complete = props.rows !== null && rows.length > 0
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
        <span className='text-muted-foreground text-[10px]'>
          {t('Unit')}: USD / {t('1M token')}
        </span>
      </div>
      {complete ? (
        <StaticDataTable
          className='rounded-none border-0 bg-transparent'
          tableProps={{ 'aria-label': t('Usage cost') }}
          tableClassName='table-fixed [&_td]:break-words [&_td]:whitespace-normal [&_td]:text-xs [&_th]:text-xs [&_th]:whitespace-normal'
          data={rows}
          getRowKey={(row) => row.id}
          columns={[
            {
              id: 'item',
              header: t('Billing item'),
              className: 'w-1/4',
              cell: (row) => t(row.label),
            },
            {
              id: 'usage',
              header: t('Token usage'),
              className: 'w-1/4 text-right',
              cellClassName: 'text-right font-mono tabular-nums',
              cell: (row) => row.count.toLocaleString(),
            },
            {
              id: 'price',
              header: t('Final unit price'),
              className: 'w-1/4 text-right',
              cellClassName: 'text-right font-mono tabular-nums',
              cell: (row) => row.formattedPrice,
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
            'Recorded billable tokens are incomplete; the cost formula is unavailable.'
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
                {t('Final unit price ÷ 1,000,000 × token usage = subtotal')}
              </p>
              {rows.map((row) => (
                <div
                  key={row.id}
                  className='flex flex-wrap justify-between gap-x-4 gap-y-1'
                >
                  <span>{t(row.label)}</span>
                  <span className='font-mono break-all tabular-nums'>
                    {row.formattedPrice} ÷ 1,000,000 ×{' '}
                    {row.count.toLocaleString()} = {row.formattedSubtotal}
                  </span>
                </div>
              ))}
              <p className='border-t pt-2 font-mono break-all tabular-nums'>
                {t('Token cost subtotal')}:{' '}
                {rows.map((row) => row.formattedSubtotal).join(' + ')} ={' '}
                {calculated}
              </p>
              <p className='text-muted-foreground'>
                {t(
                  'Uses recorded billable tokens and final unit prices; multipliers are not applied again.'
                )}
              </p>
              {diverges && (
                <p className='text-muted-foreground'>
                  {t(
                    'Token subtotals differ from the recorded charge. Rounding or additional charges may apply; the log charge is authoritative.'
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
