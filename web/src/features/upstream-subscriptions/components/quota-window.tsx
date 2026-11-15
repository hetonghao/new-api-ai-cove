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

import { Progress } from '@/components/ui/progress'
import { toIntlLocale } from '@/i18n/languages'
import { formatDateTimeStr, formatNumber } from '@/lib/format'
import { cn } from '@/lib/utils'

import type { SubscriptionQuotaWindow } from '../types'

type QuotaWindowProps = {
  readonly window: SubscriptionQuotaWindow
  readonly subscriptionName: string
}

export function QuotaWindow(props: QuotaWindowProps) {
  const { t, i18n } = useTranslation()
  const locale = toIntlLocale(i18n.resolvedLanguage || i18n.language)
  const window = props.window
  const percentage = window.used_percent
  const labels = {
    rolling: t('5-hour window'),
    weekly: t('Weekly window'),
    monthly: t('Monthly window'),
  }
  const label = labels[window.key]
  const percentText =
    percentage === null ? t('Unknown') : `${formatNumber(percentage, locale)}%`
  const exhausted = percentage !== null && percentage >= 100
  const restricted = window.status !== 'ok'
  const overLimit = percentage !== null && percentage > 100

  return (
    <div className='min-w-0 space-y-2.5'>
      <div className='flex flex-wrap items-baseline justify-between gap-1 text-sm'>
        <span className='font-medium'>{label}</span>
        <span className={cn('tabular-nums', exhausted && 'text-destructive')}>
          {t('{{percent}} used', { percent: percentText })}
        </span>
      </div>
      {percentage === null ? (
        <div className='bg-muted h-1 rounded-full' aria-hidden='true' />
      ) : (
        <Progress
          value={Math.min(percentage, 100)}
          max={100}
          aria-label={`${props.subscriptionName} · ${label}`}
          aria-valuetext={t('{{percent}} used', { percent: percentText })}
          className={cn(
            'gap-0 [&_[data-slot=progress-indicator]]:transition-none',
            percentage >= 80 &&
              '[&_[data-slot=progress-indicator]]:bg-amber-600 dark:[&_[data-slot=progress-indicator]]:bg-amber-400',
            exhausted &&
              '[&_[data-slot=progress-indicator]]:bg-destructive dark:[&_[data-slot=progress-indicator]]:bg-destructive'
          )}
        />
      )}
      {window.used_credits !== undefined &&
        window.limit_credits !== undefined && (
          <p className='text-muted-foreground text-xs leading-relaxed [overflow-wrap:anywhere] tabular-nums'>
            {t('Credits used / limit')}{' '}
            {BigInt(window.used_credits).toLocaleString(locale)} /{' '}
            {BigInt(window.limit_credits).toLocaleString(locale)}
            {window.remaining_credits !== undefined && (
              <span className='mt-0.5 block'>
                {t('Remaining credits: {{amount}}', {
                  amount: BigInt(window.remaining_credits).toLocaleString(
                    locale
                  ),
                })}
              </span>
            )}
          </p>
        )}
      {(exhausted || restricted) && (
        <p
          className={cn('text-xs font-medium', exhausted && 'text-destructive')}
        >
          {overLimit && t('Over limit')}
          {exhausted && !overLimit && t('Limit reached')}
          {!exhausted && restricted && t('Restricted by platform')}
        </p>
      )}
      {window.resets_at && (
        <p className='text-muted-foreground text-xs leading-relaxed'>
          {t('Resets at')}{' '}
          <time className='tabular-nums' dateTime={window.resets_at}>
            {formatDateTimeStr(new Date(window.resets_at))}
          </time>
        </p>
      )}
    </div>
  )
}
