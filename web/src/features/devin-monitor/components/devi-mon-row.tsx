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
import dayjs from '@/lib/dayjs'
import { formatNumber } from '@/lib/format'
import { cn } from '@/lib/utils'

import type { DeviMonAccount } from '../types'

// Mirrors the Coding Plan quota window so both tabs read the same: the bar is
// the REMAINING allowance and severity follows the used share.
type Severity = 'success' | 'info' | 'warning' | 'destructive' | 'neutral'

function severityOf(used: number | null): Severity {
  if (used === null) return 'neutral'
  if (used >= 98) return 'destructive'
  if (used >= 90) return 'warning'
  if (used >= 50) return 'info'
  return 'success'
}

const BAR_TONES: Record<Severity, string> = {
  success: '[&_[data-slot=progress-indicator]]:bg-success',
  info: '[&_[data-slot=progress-indicator]]:bg-info',
  warning: '[&_[data-slot=progress-indicator]]:bg-warning',
  destructive: '[&_[data-slot=progress-indicator]]:bg-destructive',
  neutral: '[&_[data-slot=progress-indicator]]:bg-muted-foreground',
}

const SEVERITY_TOKENS: Record<Severity, string> = {
  success: 'var(--success)',
  info: 'var(--info)',
  warning: 'var(--warning)',
  destructive: 'var(--destructive)',
  neutral: 'var(--muted-foreground)',
}

type Relative = (target: Date, now: number) => string

type DeviMonRowProps = {
  readonly account: DeviMonAccount
  readonly now: number
}

export function DeviMonRow(props: DeviMonRowProps) {
  const { t, i18n } = useTranslation()
  const locale = toIntlLocale(i18n.resolvedLanguage || i18n.language)
  const account = props.account
  const now = props.now
  const used = account.used_percent
  const remaining = used === null ? null : Math.max(0, 100 - used)
  const severity = severityOf(used)
  const unavailable = account.error !== ''

  const relative: Relative = (target, current) => {
    const milliseconds = target.getTime() - current
    if (Number.isNaN(milliseconds)) return ''
    if (milliseconds <= 0) return t('Overdue')
    const minutes = Math.floor(milliseconds / 60_000)
    const days = Math.floor(minutes / 1440)
    const hours = Math.floor((minutes % 1440) / 60)
    if (days > 0) return t('{{days}}d{{hours}}h left', { days, hours })
    if (hours > 0) return t('{{hours}}h left', { hours })
    return t('{{minutes}}m left', { minutes: Math.max(1, minutes) })
  }

  let devinLabel = t('No Devin account')
  let devinTone = 'text-muted-foreground'
  if (account.devin_disabled !== null) {
    if (account.devin_disabled) {
      devinLabel = t('Devin disabled')
      devinTone = 'text-destructive'
    } else {
      devinLabel = t('Devin enabled')
      devinTone = 'text-success'
    }
  }

  let percentText =
    remaining === null
      ? t('Unknown')
      : `${formatNumber(remaining, locale)}%`
  if (remaining !== null && remaining > 0 && remaining < 0.01) {
    percentText = `<${formatNumber(0.01, locale)}%`
  }
  let valueText = t('{{percent}} remaining', { percent: percentText })
  if (used !== null && used >= 100) valueText = t('Window quota exhausted')
  else if (remaining === null) valueText = t('Quota unknown')

  const resetText =
    account.reset_at === null
      ? '-'
      : `${dayjs(account.reset_at * 1000).format('MM/DD HH:mm')} · ${relative(
          new Date(account.reset_at * 1000),
          now
        )}`

  let renewText = '-'
  if (account.renew_at !== '') {
    const renewAt = new Date(account.renew_at)
    renewText = `${dayjs(renewAt).format('MM/DD HH:mm')} · ${relative(renewAt, now)}`
    if (account.renew_source !== 'live') {
      renewText = `${renewText} ${t('(sign-in snapshot)')}`
    }
  }

  const cardsText =
    account.cards.length === 0
      ? t('None')
      : `x${account.cards.length} · ${account.cards
          .map((card) => {
            const expires = new Date(card.expires_at)
            if (Number.isNaN(expires.getTime())) return card.expires_at
            return `${dayjs(expires).format('MM/DD HH:mm')} · ${relative(expires, now)}`
          })
          .join(' / ')}`

  return (
    <li className='space-y-2.5 px-3 py-3 sm:px-4'>
      <div className='flex flex-wrap items-start justify-between gap-x-3 gap-y-1.5'>
        <div className='min-w-0 flex-1'>
          <div className='flex flex-wrap items-center gap-2'>
            <h3 className='min-w-0 text-sm font-semibold [overflow-wrap:anywhere]'>
              {account.email}
            </h3>
            <span className={cn('text-xs font-medium', devinTone)}>{devinLabel}</span>
            {account.unlimited && (
              <span className='text-muted-foreground text-xs'>
                {t('Unlimited credits')}
              </span>
            )}
            {unavailable && (
              <span className='text-destructive text-xs'>{account.error}</span>
            )}
          </div>
        </div>
      </div>
      <div className='flex flex-wrap items-baseline justify-between gap-1 text-sm'>
        <span className='font-medium'>{t('Weekly allowance')}</span>
        <span
          className='tabular-nums'
          style={
            used === null
              ? undefined
              : {
                  color: `color-mix(in oklab, ${SEVERITY_TOKENS[severity]} 70%, var(--foreground))`,
                }
          }
        >
          {valueText}
        </span>
      </div>
      {remaining === null ? (
        <div className='bg-muted h-1 rounded-full' aria-hidden='true' />
      ) : (
        <Progress
          value={remaining}
          max={100}
          aria-label={account.email}
          aria-valuetext={valueText}
          className={cn('gap-0', BAR_TONES[severity])}
        />
      )}
      <dl className='text-muted-foreground grid gap-x-4 gap-y-1 text-xs sm:grid-cols-2'>
        <div className='flex min-w-0 items-baseline gap-1.5'>
          <dt className='shrink-0 whitespace-nowrap'>{t('Weekly resets')}</dt>
          <dd className='tabular-nums [overflow-wrap:anywhere]'>{resetText}</dd>
        </div>
        <div className='flex min-w-0 items-baseline gap-1.5'>
          <dt className='shrink-0 whitespace-nowrap'>{t('Credits')}</dt>
          <dd className='tabular-nums [overflow-wrap:anywhere]'>
            {account.unlimited ? t('Unlimited') : account.credit_balance || '-'}
          </dd>
        </div>
        <div className='flex min-w-0 items-baseline gap-1.5'>
          <dt className='shrink-0 whitespace-nowrap'>{t('Reset cards')}</dt>
          <dd className='tabular-nums [overflow-wrap:anywhere]'>{cardsText}</dd>
        </div>
        <div className='flex min-w-0 items-baseline gap-1.5'>
          <dt className='shrink-0 whitespace-nowrap'>{t('Subscription renews')}</dt>
          <dd className='tabular-nums [overflow-wrap:anywhere]'>{renewText}</dd>
        </div>
      </dl>
    </li>
  )
}