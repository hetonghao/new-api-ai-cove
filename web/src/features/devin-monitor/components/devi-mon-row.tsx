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

import { useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { QuotaWindow } from '@/features/upstream-subscriptions/components/quota-window'
import { toIntlLocale } from '@/i18n/languages'
import dayjs from '@/lib/dayjs'
import { formatNumber } from '@/lib/format'

import type { DeviMonAccount, DeviMonResetOffer } from '../types'
import { DeviMonAccountActions } from './devi-mon-account-actions'
import { DeviMonAccountSettings } from './devi-mon-account-settings'
import { DevinRequestHistory } from './devi-mon-request-history'
import { DeviMonResetCards } from './devi-mon-reset-cards'
import { DeviMonResetControls } from './devi-mon-reset-controls'

function RenewalDate({
  value,
  now,
}: {
  readonly value: string
  readonly now: number
}) {
  const { t, i18n } = useTranslation()
  const date = dayjs(value)
  if (!date.isValid()) return <>{t('Not provided')}</>
  const days = date.startOf('day').diff(dayjs(now).startOf('day'), 'day')
  const locale = toIntlLocale(i18n.resolvedLanguage || i18n.language)
  const relative =
    days === 0
      ? t('Today')
      : new Intl.RelativeTimeFormat(locale, { numeric: 'always' }).format(
          days,
          'day'
        )
  return (
    <time
      className='inline-flex flex-wrap gap-x-2 tabular-nums'
      dateTime={value}
      title={date.format('YYYY-MM-DD HH:mm:ss')}
    >
      <span className='whitespace-nowrap'>{date.format('MM-DD HH:mm')}</span>
      <span className='text-muted-foreground whitespace-nowrap'>
        {relative}
      </span>
    </time>
  )
}

export function DeviMonRow({
  account,
  now,
  configured,
}: {
  readonly account: DeviMonAccount
  readonly now: number
  readonly configured: boolean
}) {
  const { t, i18n } = useTranslation()
  const locale = toIntlLocale(i18n.resolvedLanguage || i18n.language)
  const refreshButton = useRef<HTMLButtonElement>(null)
  const [resetOffer, setResetOffer] = useState<DeviMonResetOffer | null>(null)
  const [settingsOpen, setSettingsOpen] = useState(false)
  const balance = account.credit_balance.trim()
  let balanceText = balance || t('Not provided')
  if (account.unlimited) {
    balanceText = t('Unlimited')
  } else if (balance && Number.isFinite(Number(balance))) {
    balanceText = formatNumber(Number(balance), locale)
  }
  const reset =
    account.reset_at !== null && account.reset_at > 0
      ? dayjs.unix(account.reset_at)
      : null
  return (
    <li className='grid min-w-0 gap-x-5 gap-y-1.5 px-3 py-2.5 sm:px-4 @3xl/accounts:grid-cols-[minmax(0,1fr)_auto]'>
      <h3 className='col-start-1 row-start-1 min-w-0 self-center text-sm font-semibold [overflow-wrap:anywhere]'>
        {account.email}
      </h3>
      <div className='min-w-0 space-y-1 @3xl/accounts:col-start-2 @3xl/accounts:row-span-2 @3xl/accounts:row-start-1 @3xl/accounts:w-82'>
        <DeviMonAccountActions
          email={account.email}
          disabled={account.devin_disabled}
          routing={account.devin_routing}
          configured={configured}
          onOpenSettings={() => setSettingsOpen(true)}
          refreshButton={refreshButton}
          onResetOffer={setResetOffer}
        />
        {account.devin_disabled !== null && (
          <DevinRequestHistory
            email={account.email}
            history={account.devin_requests}
          />
        )}
      </div>
      <div className='@container/reading min-w-0 @3xl/accounts:col-start-1 @3xl/accounts:row-start-2'>
        <div className='grid min-w-0 items-start gap-x-4 gap-y-2 @min-[36rem]/reading:grid-cols-[minmax(0,1fr)_16rem]'>
          <div className='min-w-0 space-y-1'>
            <DeviMonAccountSettings
              email={account.email}
              open={settingsOpen}
              onOpenChange={setSettingsOpen}
            />
            <QuotaWindow
              label={t('Weekly allowance')}
              subscriptionName={account.email}
              now={now}
              footer={
                <DeviMonResetControls
                  account={account}
                  configured={configured}
                  offer={resetOffer}
                  onOfferChange={setResetOffer}
                  returnFocus={refreshButton}
                />
              }
              window={{
                key: 'weekly',
                duration_seconds: 7 * 24 * 60 * 60,
                status: 'ok',
                used_percent: account.used_percent,
                resets_at: reset?.isValid() ? reset.toISOString() : null,
              }}
            />

            {account.error && (
              <p className='text-destructive text-xs [overflow-wrap:anywhere]'>
                {t(account.error)}
              </p>
            )}
          </div>
          <div className='min-w-0'>
            <dl className='grid min-w-0 grid-cols-[max-content_minmax(0,1fr)] items-baseline gap-x-3 gap-y-1 text-xs leading-relaxed tabular-nums'>
              <dt className='text-muted-foreground'>{t('Credits')}</dt>
              <dd
                className='min-w-0 [overflow-wrap:anywhere]'
                title={balance || undefined}
              >
                {balanceText}
              </dd>
              <dt className='text-muted-foreground'>
                {t('Subscription renews')}
              </dt>
              <dd>
                <RenewalDate value={account.renew_at} now={now} />{' '}
                {account.renew_at && account.renew_source === 'id_token' && (
                  <span className='text-muted-foreground'>
                    {t('(sign-in snapshot)')}
                  </span>
                )}
              </dd>
              <dt className='text-muted-foreground'>{t('Reset cards')}</dt>
              <dd className='min-w-0'>
                <DeviMonResetCards
                  cards={account.cards}
                  email={account.email}
                  hasError={Boolean(account.error)}
                />
              </dd>
            </dl>
          </div>
        </div>
      </div>
    </li>
  )
}
