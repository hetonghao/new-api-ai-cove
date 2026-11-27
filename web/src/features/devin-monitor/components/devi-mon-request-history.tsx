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

import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from '@/components/ui/popover'
import { toIntlLocale } from '@/i18n/languages'
import dayjs from '@/lib/dayjs'
import { formatNumber } from '@/lib/format'
import { cn } from '@/lib/utils'

import type { DevinRequests } from '../types'

// Match the CPA management center's ProviderStatusBar: continuous RGB stops
// at 0%, 50%, 100%, and an uncolored block for a window with no requests.
const stops = [
  [239, 68, 68],
  [250, 204, 21],
  [34, 197, 94],
] as const
const lampSlots = Array.from({ length: 20 }, (_, i) => `recent-slot-${i}`)
function bucketColor(rate: number) {
  const i = rate < 0.5 ? 0 : 1,
    fraction = rate * 2 - i
  return `rgb(${stops[i].map((v, c) => Math.round(v + (stops[i + 1][c] - v) * fraction)).join(', ')})`
}

export function DevinRequestHistory({
  email,
  history,
}: {
  readonly email: string
  readonly history?: DevinRequests | null
}) {
  const { t, i18n } = useTranslation()
  const [tabStop, setTabStop] = useState(19)
  const locale = toIntlLocale(i18n.resolvedLanguage || i18n.language)
  if (!history) {
    return (
      <p className='text-muted-foreground text-xs'>
        {t('Request history')}: {t('Not fetched yet')}
      </p>
    )
  }
  const recent = history.recent.slice(-20)
  const blocks = lampSlots.map((slot, i) => {
    const { success, failed } = recent[i - 20 + recent.length] ?? {
      success: 0,
      failed: 0,
    }
    const total = success + failed
    const start = dayjs.unix(history.as_of - (20 - i) * 600)
    return {
      slot,
      success,
      failed,
      rate: total ? success / total : null,
      time: `${start.format('HH:mm')} – ${start.add(10, 'minute').format('HH:mm')}`,
    }
  })
  const wins = recent.reduce((sum, b) => sum + b.success, 0)
  const total = recent.reduce((sum, b) => sum + b.success + b.failed, 0)
  const rate = total ? (wins * 100) / total : null
  let rateClass = 'text-muted-foreground'
  if (rate !== null) {
    if (rate >= 90) rateClass = 'text-success'
    else if (rate >= 50) rateClass = 'text-warning'
    else rateClass = 'text-destructive'
  }
  const groupLabel = `${t('Request history')} · ${email}`
  return (
    <div
      role='group'
      aria-label={groupLabel}
      className='min-w-0 space-y-0.5 text-xs tabular-nums'
    >
      <div className='flex flex-wrap items-baseline justify-between gap-x-2'>
        <span className='font-medium'>{t('Request history')}</span>
        <span className='inline-flex flex-wrap gap-x-2'>
          <span
            title={t('Success')}
            className={
              history.success > 0 ? 'text-success' : 'text-muted-foreground'
            }
          >
            {t('Success')}{' '}
            <b className='font-semibold'>
              {formatNumber(history.success, locale)}
            </b>
          </span>
          <span
            title={t('Failed')}
            className={
              history.failed > 0 ? 'text-destructive' : 'text-muted-foreground'
            }
          >
            {t('Failed')}{' '}
            <b className='font-semibold'>
              {formatNumber(history.failed, locale)}
            </b>
          </span>
        </span>
      </div>
      <Popover>
        {({ payload }) => {
          const selected =
            typeof payload === 'number' ? blocks[payload] : undefined
          return (
            <>
              <div className='flex items-center gap-2'>
                <div className='flex min-w-0 flex-1'>
                  {blocks.map((b, i) => (
                    <PopoverTrigger
                      key={b.slot}
                      payload={i}
                      openOnHover
                      delay={0}
                      closeDelay={0}
                      type='button'
                      tabIndex={i === tabStop ? 0 : -1}
                      onFocus={() => setTabStop(i)}
                      onKeyDown={(e) => {
                        let next: number
                        switch (e.key) {
                          case 'ArrowRight':
                            next = Math.min(19, i + 1)
                            break
                          case 'ArrowLeft':
                            next = Math.max(0, i - 1)
                            break
                          case 'Home':
                            next = 0
                            break
                          case 'End':
                            next = 19
                            break
                          default:
                            return
                        }
                        e.preventDefault()
                        const target =
                          e.currentTarget.parentElement?.children[next]
                        if (target instanceof HTMLButtonElement) {
                          target.focus()
                          if (next !== i) target.click()
                        }
                      }}
                      aria-label={`${b.time} · ${b.rate === null ? t('No requests') : `${t('Success')} ${b.success} · ${t('Failed')} ${b.failed}`}`}
                      className='group/lamp focus-visible:outline-ring flex h-7 min-w-0 flex-1 items-center justify-center rounded-xs px-px focus-visible:outline-2 pointer-coarse:h-11'
                    >
                      <span
                        className='bg-muted-foreground/20 h-3 w-full rounded-[2px] group-hover/lamp:h-4 group-data-popup-open/lamp:h-4'
                        style={
                          b.rate === null
                            ? undefined
                            : { backgroundColor: bucketColor(b.rate) }
                        }
                      />
                    </PopoverTrigger>
                  ))}
                </div>
                <span
                  title={t('Success rate')}
                  className={cn('min-w-10 text-end font-semibold', rateClass)}
                >
                  {rate === null
                    ? '--'
                    : `${formatNumber(Number(rate.toFixed(1)), locale)}%`}
                </span>
              </div>
              {selected && (
                <PopoverContent
                  side='top'
                  initialFocus={false}
                  finalFocus={false}
                  aria-label={selected.time}
                  className='w-auto gap-1 text-xs tabular-nums'
                >
                  <strong className='font-medium'>{selected.time}</strong>
                  {selected.rate === null ? (
                    <span className='text-muted-foreground'>
                      {t('No requests')}
                    </span>
                  ) : (
                    <span className='flex items-center gap-2'>
                      <span className='text-success'>
                        {t('Success')} {formatNumber(selected.success, locale)}
                      </span>
                      <span className='text-destructive'>
                        {t('Failed')} {formatNumber(selected.failed, locale)}
                      </span>
                      <span>({(selected.rate * 100).toFixed(1)}%)</span>
                    </span>
                  )}
                </PopoverContent>
              )}
            </>
          )
        }}
      </Popover>
    </div>
  )
}
