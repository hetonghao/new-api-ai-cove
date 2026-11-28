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

import { Plus } from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'
import { Popover, PopoverContent, PopoverTitle } from '@/components/ui/popover'
import dayjs from '@/lib/dayjs'
import { cn } from '@/lib/utils'

import type { DeviMonAccount } from '../types'
import { DevinDetailTrigger } from './devi-mon-operation-group'

type Cards = DeviMonAccount['cards']

function CardDates({
  cards,
  column = false,
}: {
  readonly cards: Cards
  readonly column?: boolean
}) {
  const { t } = useTranslation()
  const counts = new Map<string, number>()
  for (const { expires_at } of cards) {
    counts.set(expires_at, (counts.get(expires_at) ?? 0) + 1)
  }
  return (
    <ul
      className={cn(
        'flex min-w-0 flex-wrap items-baseline gap-x-2 gap-y-0.5 tabular-nums',
        column && 'flex-col items-start'
      )}
    >
      {Array.from(counts, ([expires, count]) => {
        const date = dayjs(expires)
        return (
          <li className='whitespace-nowrap' key={expires}>
            {date.isValid() ? (
              <time
                dateTime={expires}
                title={date.format('YYYY-MM-DD HH:mm:ss')}
              >
                {date.format('MM-DD HH:mm')}
              </time>
            ) : (
              t('Not provided')
            )}
            {count > 1 && (
              <span className='text-muted-foreground'> ×{count}</span>
            )}
          </li>
        )
      })}
    </ul>
  )
}

export function DeviMonResetCards({
  cards,
  email,
  hasError,
}: {
  readonly cards: Cards
  readonly email: string
  readonly hasError: boolean
}) {
  const { t } = useTranslation()
  if (!cards.length) return hasError ? t('Not provided') : t('None')
  const extra = cards.slice(2)
  const moreLabel = `${t('Reset cards')} · ${t('More')} (${extra.length}) · ${email}`
  return (
    <div className='flex min-w-0 flex-wrap items-center gap-x-1.5 gap-y-0.5'>
      <span className='shrink-0'>×{cards.length}</span>
      <CardDates cards={cards.slice(0, 2)} />
      {extra.length > 0 && (
        <Popover>
          <DevinDetailTrigger
            openOnHover
            delay={100}
            closeDelay={120}
            render={
              <Button
                type='button'
                variant='outline'
                size='icon-xs'
                className='text-muted-foreground relative size-5 shrink-0 pointer-coarse:after:absolute pointer-coarse:after:-inset-y-2'
                aria-label={moreLabel}
              />
            }
          >
            <Plus className='size-3' aria-hidden='true' />
          </DevinDetailTrigger>
          <PopoverContent
            align='end'
            side='top'
            collisionPadding={8}
            initialFocus={false}
            aria-label={moreLabel}
            className='max-h-48 w-max max-w-[calc(100vw-1rem)] gap-1.5 overflow-auto text-xs'
          >
            <PopoverTitle className='text-muted-foreground text-xs'>
              {t('Expires at')} · ×{extra.length}
            </PopoverTitle>
            <CardDates cards={extra} column />
          </PopoverContent>
        </Popover>
      )}
    </div>
  )
}
