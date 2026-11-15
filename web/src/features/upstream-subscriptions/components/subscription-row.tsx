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

import { useMutation, useQueryClient } from '@tanstack/react-query'
import {
  CircleAlert,
  CircleCheck,
  Pencil,
  RefreshCw,
  Trash2,
} from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { StatusBadge, type StatusVariant } from '@/components/status-badge'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import dayjs from '@/lib/dayjs'
import { formatTimestampToDate } from '@/lib/format'

import { refreshUpstreamSubscription } from '../api'
import type { UpstreamSubscription } from '../types'
import { QuotaWindow } from './quota-window'
import { SubscriptionPlatform } from './subscription-platform'

type SubscriptionRowProps = {
  readonly subscription: UpstreamSubscription
  readonly now?: number
  readonly onEdit: (subscription: UpstreamSubscription) => void
  readonly onRemove: (subscription: UpstreamSubscription) => void
}

export function SubscriptionRow(props: SubscriptionRowProps) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const subscription = props.subscription
  const now = props.now ?? Date.now()
  const quota = subscription.quota
  const refresh = useMutation({
    mutationFn: () => refreshUpstreamSubscription(subscription.id),
    onSuccess: async () => {
      await queryClient.invalidateQueries({
        queryKey: ['upstream-subscriptions'],
      })
    },
  })
  const errorMessages: Readonly<Record<string, string>> = {
    unauthorized: t(
      'Credential expired. Edit this subscription to sign in again or replace the key.'
    ),
    forbidden: t(
      'Subscription access was denied. Check the plan and workspace permissions on the platform.'
    ),
    endpoint_changed: t(
      'The platform quota endpoint changed. Try again later.'
    ),
    rate_limited: t(
      'The platform is rate limiting requests. Refresh will resume after the retry delay.'
    ),
    timeout: t('The platform request timed out. Try again later.'),
    network_error: t('Could not connect to the platform. Try again later.'),
    invalid_response: t('The platform returned unrecognized quota data.'),
    credential_error: t(
      'Could not decrypt the credential. Configure this subscription again.'
    ),
  }
  const coolingDown =
    subscription.last_attempt_at > now / 1000 - 60 ||
    (subscription.error_code === 'rate_limited' &&
      subscription.next_refresh_at > now / 1000)
  let availability = t('Quota status unknown')
  let availabilityVariant: StatusVariant = 'neutral'
  let availabilityIcon: typeof CircleCheck | undefined
  let availabilityColor: string | undefined
  if (
    !subscription.stale &&
    !subscription.error_code &&
    quota &&
    quota.eligible !== false &&
    quota.windows.length > 0
  ) {
    if (
      quota.windows.some(
        (window) =>
          (window.used_percent !== null && window.used_percent >= 100) ||
          (window.status === 'ok' && window.remaining_credits === '0')
      )
    ) {
      availability = t('Quota exhausted now')
      availabilityVariant = 'danger'
      availabilityIcon = CircleAlert
      availabilityColor = 'var(--destructive)'
    } else if (
      quota.windows.every(
        (window) =>
          window.status === 'ok' &&
          window.used_percent !== null &&
          window.used_percent < 100
      )
    ) {
      availability = t('Available now')
      availabilityVariant = 'success'
      availabilityIcon = CircleCheck
      availabilityColor = 'var(--success)'
    }
  }

  return (
    <li className='space-y-2.5 px-3 py-3 sm:px-4'>
      <div className='flex flex-wrap items-center justify-between gap-x-3 gap-y-1.5'>
        <div className='min-w-0 flex-1'>
          <div className='flex flex-wrap items-center gap-2'>
            <h3 className='min-w-0 text-sm font-semibold [overflow-wrap:anywhere]'>
              {subscription.name}
            </h3>
            <Badge variant='secondary'>
              <SubscriptionPlatform platform={subscription.platform} />
            </Badge>
            <StatusBadge
              variant={availabilityVariant}
              icon={availabilityIcon}
              copyable={false}
              label={availability}
              className='shrink-0'
              style={
                availabilityColor
                  ? {
                      color: `color-mix(in oklab, ${availabilityColor} 70%, var(--foreground))`,
                    }
                  : undefined
              }
            />
            {subscription.stale && (
              <Badge variant='outline'>{t('Stale data')}</Badge>
            )}
            {subscription.channel && (
              <span className='text-muted-foreground text-xs tabular-nums'>
                {t('Channel')} #{subscription.channel.id} · {t('Priority')}{' '}
                {subscription.channel.priority} · {t('Weight')}{' '}
                {subscription.channel.weight}
              </span>
            )}
          </div>
        </div>
        <div className='ml-auto flex min-w-0 items-center justify-end gap-3'>
          <div className='flex shrink-0 items-center gap-1'>
            <Button
              variant='outline'
              size='icon-sm'
              disabled={refresh.isPending || coolingDown}
              onClick={() => refresh.mutate()}
              aria-label={t('Fetch quota for {{name}}', {
                name: subscription.name,
              })}
              title={refresh.isPending ? t('Fetching...') : t('Fetch quota')}
            >
              <RefreshCw aria-hidden='true' />
            </Button>
            <Button
              variant='ghost'
              size='icon-sm'
              onClick={() => props.onEdit(subscription)}
              aria-label={t('Edit {{name}}', { name: subscription.name })}
            >
              <Pencil aria-hidden='true' />
            </Button>
            <Button
              variant='ghost'
              size='icon-sm'
              onClick={() => props.onRemove(subscription)}
              aria-label={t('Remove {{name}}', { name: subscription.name })}
            >
              <Trash2 aria-hidden='true' />
            </Button>
          </div>
        </div>
      </div>
      {subscription.error_code && (
        <p role='status' className='text-destructive text-sm leading-relaxed'>
          {errorMessages[subscription.error_code] ??
            t('Quota refresh failed. Try again later.')}
          {subscription.stale && (
            <span className='mt-1 block text-xs'>
              {t('Showing the last successful quota snapshot.')}
            </span>
          )}
        </p>
      )}
      {quota && quota.windows.length > 0 ? (
        <div className='grid min-w-0 gap-x-5 gap-y-3 sm:grid-cols-3'>
          {quota.windows.map((window) => (
            <QuotaWindow
              key={window.key}
              window={window}
              subscriptionName={subscription.name}
              now={now}
            />
          ))}
        </div>
      ) : (
        <p className='text-muted-foreground py-2 text-sm'>
          {quota?.eligible === false
            ? t('This workspace has no available subscription allowance.')
            : t('No quota windows available yet.')}
        </p>
      )}
      <p className='text-muted-foreground flex flex-wrap gap-x-4 gap-y-1 text-xs'>
        <span>
          {t('Refreshes every {{minutes}} minutes', {
            minutes: subscription.refresh_interval_minutes,
          })}
        </span>
        <span className='tabular-nums'>
          {t('Updated')}:{' '}
          {subscription.last_success_at
            ? formatTimestampToDate(subscription.last_success_at)
            : t('Not fetched yet')}
        </span>
        <span className='tabular-nums'>
          {t('Subscription expires')}:{' '}
          {quota?.subscription_period_end ? (
            <time dateTime={quota.subscription_period_end}>
              {dayjs(quota.subscription_period_end).format('YYYY-MM-DD HH:mm')}
            </time>
          ) : (
            t('Not provided')
          )}
        </span>
        {!!subscription.channel_id && !subscription.channel && (
          <span className='text-warning'>
            {t(
              'Bound channel was deleted. Select another channel or use an independent name.'
            )}
          </span>
        )}
        {subscription.error_code && (
          <span>
            {t('Next retry')}:{' '}
            <span className='tabular-nums'>
              {formatTimestampToDate(subscription.next_refresh_at)}
            </span>
          </span>
        )}
        {subscription.account && (
          <span className='min-w-0 [overflow-wrap:anywhere]'>
            {t('Subscription account')}: {subscription.account}
          </span>
        )}
      </p>
    </li>
  )
}
