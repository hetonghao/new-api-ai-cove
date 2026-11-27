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

import type { ReactNode } from 'react'
import { useTranslation } from 'react-i18next'

import { Progress } from '@/components/ui/progress'
import { toIntlLocale } from '@/i18n/languages'
import dayjs from '@/lib/dayjs'
import { formatNumber } from '@/lib/format'
import { cn } from '@/lib/utils'

import type { SubscriptionQuotaWindow } from '../types'

type QuotaWindowProps = {
  readonly window: SubscriptionQuotaWindow
  readonly label?: string
  readonly subscriptionName: string
  readonly footer?: ReactNode
  readonly now?: number
}

export function QuotaWindow(props: QuotaWindowProps) {
  const { t, i18n } = useTranslation()
  const locale = toIntlLocale(i18n.resolvedLanguage || i18n.language)
  const window = props.window
  const percentage = window.used_percent
  const labels = {
    rolling: t('5-hour window'),
    weekly: t('7-day window'),
    monthly: t('30-day window'),
  }
  const label = props.label ?? labels[window.key]
  const remaining = percentage === null ? null : Math.max(0, 100 - percentage)
  let percentText =
    remaining === null ? t('Unknown') : `${formatNumber(remaining, locale)}%`
  if (remaining !== null && remaining > 0 && remaining < 0.01) {
    percentText = `<${formatNumber(0.01, locale)}%`
  }
  const exhausted = percentage !== null && percentage >= 100
  const restricted = window.status !== 'ok'
  let severity: 'success' | 'info' | 'warning' | 'destructive' = 'success'
  if (percentage !== null && percentage >= 98) {
    severity = 'destructive'
  } else if (percentage !== null && percentage >= 90) {
    severity = 'warning'
  } else if (percentage !== null && percentage >= 50) {
    severity = 'info'
  }
  const barTones = {
    success: '[&_[data-slot=progress-indicator]]:bg-success',
    info: '[&_[data-slot=progress-indicator]]:bg-info',
    warning: '[&_[data-slot=progress-indicator]]:bg-warning',
    destructive: '[&_[data-slot=progress-indicator]]:bg-destructive',
  }
  let valueText = t('{{percent}} remaining', { percent: percentText })
  if (exhausted) valueText = t('Window quota exhausted')
  else if (percentage === null) valueText = t('Quota unknown')
  let resetText = ''
  if (window.resets_at) {
    const milliseconds =
      Date.parse(window.resets_at) - (props.now ?? Date.now())
    const minutes = Math.ceil(milliseconds / 60_000)
    const days = Math.floor(minutes / 1440)
    const hours = Math.floor((minutes % 1440) / 60)
    const remainderMinutes = minutes % 60
    if (milliseconds <= 0) {
      resetText = t('Reset due')
    } else if (milliseconds < 60_000) {
      resetText = t('Resets in less than 1 minute')
    } else if (days > 0) {
      resetText = t(
        'Resets in {{days}} days {{hours}} hours {{minutes}} minutes',
        { days, hours, minutes: remainderMinutes }
      )
    } else if (hours > 0) {
      resetText = t('Resets in {{hours}} hours {{minutes}} minutes', {
        hours,
        minutes: remainderMinutes,
      })
    } else {
      resetText = t('Resets in {{minutes}} minutes', { minutes })
    }
  }

  return (
    <div className='min-w-0 space-y-1.5'>
      <div className='flex flex-wrap items-baseline justify-between gap-1 text-sm'>
        <span className='font-medium'>{label}</span>
        <span
          className='tabular-nums'
          style={
            percentage === null
              ? undefined
              : {
                  color: `color-mix(in oklab, var(--${severity}) 70%, var(--foreground))`,
                }
          }
        >
          {valueText}
        </span>
      </div>
      {percentage === null ? (
        <div className='bg-muted h-1 rounded-full' aria-hidden='true' />
      ) : (
        <Progress
          value={remaining}
          max={100}
          aria-label={`${props.subscriptionName} · ${label}`}
          aria-valuetext={valueText}
          className={cn(
            'gap-0 [&_[data-slot=progress-indicator]]:transition-none',
            barTones[severity],
            exhausted && '[&_[data-slot=progress-track]]:bg-destructive/15'
          )}
        />
      )}
      {!exhausted && restricted && (
        <p className='text-muted-foreground text-xs'>
          {t('Restricted by platform')}
        </p>
      )}
      {(window.resets_at || props.footer) && (
        <div className='flex flex-wrap items-center justify-between gap-x-2 gap-y-1'>
          {window.resets_at && (
            <p className='text-muted-foreground text-xs leading-relaxed'>
              <time
                className='tabular-nums'
                dateTime={window.resets_at}
                title={dayjs(window.resets_at).format('YYYY-MM-DD HH:mm:ss')}
              >
                {resetText}
              </time>
            </p>
          )}
          {props.footer}
        </div>
      )}
    </div>
  )
}
