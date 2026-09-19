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
import { formatBillingCurrencyFromUSD } from '@/lib/currency'

import type { UsageLog } from '../../data/schema'
import type {
  SettledPriceRow,
  SettledUsageRow,
} from '../../lib/settled-ratio-prices'
import { SettledUsageCost } from './settled-usage-cost'

const priceOptions = {
  digitsLarge: 4,
  digitsSmall: 6,
  abbreviate: false,
  minimumNonZero: 0.000001,
}

/**
 * Settled unit prices of one log entry: the unit price recorded for each
 * billing line, the multipliers settlement applied to it, and (when the request
 * bills by usage) the usage that produced the charge.
 *
 * Expression billing passes the tier price plus its conditional and group
 * multipliers; ratio billing passes the model ratio price plus the group ratio.
 * `usage` stays undefined for charges that do not multiply by usage at all.
 */
export function SettledPricePanel(props: {
  log: UsageLog
  unitLabel: string
  baseHeader: string
  appliedNote: string
  formulaNote: string
  multipliers: number[]
  rows: SettledPriceRow[]
  usage: SettledUsageRow[] | null | undefined
}) {
  const { t } = useTranslation()
  const rows = props.rows.map((row) => ({
    ...row,
    formattedBase: formatBillingCurrencyFromUSD(row.base, priceOptions),
    formattedFinal: formatBillingCurrencyFromUSD(row.final, priceOptions),
  }))
  const multiplierText = props.multipliers.join(' × ')

  return (
    <section className='mt-4 min-w-0 space-y-3 border-t pt-4'>
      <div className='flex flex-wrap items-baseline justify-between gap-2'>
        <h3 className='text-xs font-semibold'>{t('Settled unit prices')}</h3>
        <span className='text-muted-foreground text-[10px]'>
          {t('Unit')}: USD / {props.unitLabel}
        </span>
      </div>
      <p className='text-muted-foreground text-xs'>{props.appliedNote}</p>
      <StaticDataTable
        className='rounded-none border-0 bg-transparent'
        tableProps={{ 'aria-label': t('Settled unit prices') }}
        tableClassName='table-fixed [&_tbody_tr]:h-10 [&_td]:text-xs [&_th]:text-xs [&_td:last-child]:font-semibold'
        data={rows}
        getRowKey={(row) => row.id}
        columns={[
          {
            id: 'item',
            header: t('Billing item'),
            className: 'w-[40%]',
            cell: (row) => t(row.label),
          },
          {
            id: 'base',
            header: props.baseHeader,
            className: 'text-right text-muted-foreground',
            cellClassName: 'text-right font-mono text-muted-foreground',
            cell: (row) => row.formattedBase,
          },
          {
            id: 'final',
            header: t('Final unit price'),
            className:
              'bg-emerald-500/10 text-right text-emerald-700 dark:text-emerald-300',
            cellClassName:
              'bg-emerald-500/10 text-right font-mono text-emerald-700 dark:text-emerald-300',
            cell: (row) => row.formattedFinal,
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
            <p className='text-muted-foreground'>{props.formulaNote}</p>
            {rows.map((row) => (
              <div
                key={row.id}
                className='flex flex-wrap justify-between gap-x-4 gap-y-1'
              >
                <span>{t(row.label)}</span>
                <span className='font-mono break-all tabular-nums'>
                  {row.formattedBase} × {multiplierText} = {row.formattedFinal}
                </span>
              </div>
            ))}
          </div>
        </CollapsibleContent>
      </Collapsible>
      {props.usage !== undefined && (
        <SettledUsageCost log={props.log} rows={props.usage} />
      )}
    </section>
  )
}
