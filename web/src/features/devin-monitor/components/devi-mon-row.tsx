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

import { QuotaWindow } from '@/features/upstream-subscriptions/components/quota-window'
import { toIntlLocale } from '@/i18n/languages'
import dayjs from '@/lib/dayjs'
import { formatNumber } from '@/lib/format'

import type { DeviMonAccount } from '../types'
import { DevinAccountStatus } from './devi-mon-status'

function AccountDate({ value }: { readonly value: string }) {
  const { t } = useTranslation()
  const date = dayjs(value)
  if (!date.isValid()) return <>{t('Not provided')}</>
  return (
    <time dateTime={value} title={date.format('YYYY-MM-DD HH:mm:ss')}>
      {date.format('YYYY-MM-DD HH:mm')}
    </time>
  )
}

export function DeviMonRow({
  account,
  now,
}: {
  readonly account: DeviMonAccount
  readonly now: number
}) {
  const { t, i18n } = useTranslation()
  const locale = toIntlLocale(i18n.resolvedLanguage || i18n.language)
  const balance = account.credit_balance.trim()
  let balanceText = balance || t('Not provided')
  if (account.unlimited) {
    balanceText = t('Unlimited')
  } else if (balance && Number.isFinite(Number(balance))) {
    balanceText = formatNumber(Number(balance), locale)
  }
  const emptyCards = account.error ? t('Not provided') : t('None')
  const cardsByExpiry = new Map<string, number>()
  for (const { expires_at } of account.cards) {
    cardsByExpiry.set(expires_at, (cardsByExpiry.get(expires_at) ?? 0) + 1)
  }
  const reset =
    account.reset_at !== null && account.reset_at > 0
      ? dayjs.unix(account.reset_at)
      : null
  return (
    <li className='grid min-w-0 gap-x-10 gap-y-3 px-3 py-3 sm:px-4 @3xl/accounts:grid-cols-2 @3xl/accounts:items-center'>
      <div className='min-w-0 space-y-2'>
        <div className='flex flex-wrap items-center gap-x-2 gap-y-1'>
          <h3 className='min-w-0 text-sm font-semibold [overflow-wrap:anywhere]'>
            {account.email}
          </h3>
          <DevinAccountStatus
            disabled={account.devin_disabled}
            routing={account.devin_routing}
          />
        </div>
        <div className='w-full max-w-80'>
          <QuotaWindow
            label={t('Weekly allowance')}
            subscriptionName={account.email}
            now={now}
            window={{
              key: 'weekly',
              duration_seconds: 7 * 24 * 60 * 60,
              status: 'ok',
              used_percent: account.used_percent,
              resets_at: reset?.isValid() ? reset.toISOString() : null,
            }}
          />
        </div>
        {account.error && (
          <p className='text-destructive text-xs [overflow-wrap:anywhere]'>
            {account.error}
          </p>
        )}
      </div>
      <dl className='grid min-w-0 grid-cols-[max-content_minmax(0,1fr)] items-baseline gap-x-3 gap-y-1 text-xs leading-relaxed tabular-nums'>
        <dt className='text-muted-foreground'>{t('Credits')}</dt>
        <dd
          className='min-w-0 [overflow-wrap:anywhere]'
          title={balance || undefined}
        >
          {balanceText}
        </dd>
        <dt className='text-muted-foreground'>{t('Subscription renews')}</dt>
        <dd>
          <AccountDate value={account.renew_at} />{' '}
          {account.renew_at && account.renew_source === 'id_token' && (
            <span className='text-muted-foreground'>
              {t('(sign-in snapshot)')}
            </span>
          )}
        </dd>
        <dt className='text-muted-foreground'>{t('Reset cards')}</dt>
        <dd className='min-w-0'>
          {account.cards.length ? (
            <div className='flex items-baseline gap-x-2'>
              <span className='shrink-0'>×{account.cards.length}</span>
              <ul className='flex min-w-0 flex-1 flex-wrap items-baseline gap-x-3 gap-y-0.5'>
                {Array.from(cardsByExpiry, ([expires, count]) => (
                  <li key={expires} className='whitespace-nowrap'>
                    <AccountDate value={expires} />
                    {count > 1 && (
                      <span className='text-muted-foreground'> ×{count}</span>
                    )}
                  </li>
                ))}
              </ul>
            </div>
          ) : (
            emptyCards
          )}
        </dd>
      </dl>
    </li>
  )
}
