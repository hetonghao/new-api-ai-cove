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
import { Pencil, RefreshCw, Trash2 } from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { formatTimestampToDate } from '@/lib/format'

import { refreshUpstreamSubscription } from '../api'
import type { UpstreamSubscription } from '../types'
import { QuotaWindow } from './quota-window'

type SubscriptionRowProps = {
  readonly subscription: UpstreamSubscription
  readonly onEdit: (subscription: UpstreamSubscription) => void
  readonly onRemove: (subscription: UpstreamSubscription) => void
}

export function SubscriptionRow(props: SubscriptionRowProps) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const subscription = props.subscription
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
    subscription.last_attempt_at > Date.now() / 1000 - 60 ||
    (subscription.error_code === 'rate_limited' &&
      subscription.next_refresh_at > Date.now() / 1000)

  return (
    <li className='space-y-4 p-4 sm:p-5'>
      <div className='flex flex-wrap items-start justify-between gap-3'>
        <div className='min-w-0 flex-1 space-y-1.5'>
          <div className='flex flex-wrap items-center gap-2'>
            <h3 className='min-w-0 text-sm font-semibold [overflow-wrap:anywhere]'>
              {subscription.name}
            </h3>
            <Badge variant='secondary'>
              {subscription.platform === 'opencode'
                ? 'OpenCode Go'
                : 'OpenDesign Go'}
            </Badge>
            {subscription.stale && (
              <Badge variant='outline'>{t('Stale data')}</Badge>
            )}
          </div>
          {(quota?.workspace_name ||
            quota?.tier ||
            quota?.balance_usd !== undefined) && (
            <p className='text-muted-foreground flex flex-wrap gap-x-3 gap-y-1 text-xs [overflow-wrap:anywhere]'>
              {quota?.workspace_name && <span>{quota.workspace_name}</span>}
              {quota?.tier && (
                <span>
                  {t('Plan')}: {quota.tier}
                </span>
              )}
              {quota?.balance_usd !== undefined && (
                <span className='tabular-nums'>
                  {t('Wallet balance (USD)')}: {quota.balance_usd}
                </span>
              )}
            </p>
          )}
        </div>
        <div className='flex shrink-0 items-center gap-1'>
          <Button
            variant='outline'
            size='sm'
            disabled={refresh.isPending || coolingDown}
            onClick={() => refresh.mutate()}
            aria-label={t('Fetch quota for {{name}}', {
              name: subscription.name,
            })}
          >
            <RefreshCw className='size-3.5' aria-hidden='true' />
            {refresh.isPending ? t('Fetching...') : t('Fetch quota')}
          </Button>
          <Button
            variant='ghost'
            size='icon'
            onClick={() => props.onEdit(subscription)}
            aria-label={t('Edit {{name}}', { name: subscription.name })}
          >
            <Pencil className='size-4' aria-hidden='true' />
          </Button>
          <Button
            variant='ghost'
            size='icon'
            onClick={() => props.onRemove(subscription)}
            aria-label={t('Remove {{name}}', { name: subscription.name })}
          >
            <Trash2 className='size-4' aria-hidden='true' />
          </Button>
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
        <div className='grid min-w-0 gap-x-8 gap-y-5 sm:grid-cols-2 xl:grid-cols-3'>
          {quota.windows.map((window) => (
            <QuotaWindow
              key={window.key}
              window={window}
              subscriptionName={subscription.name}
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
        <span>
          {t('Last successful refresh')}:{' '}
          <span className='tabular-nums'>
            {subscription.last_success_at
              ? formatTimestampToDate(subscription.last_success_at)
              : t('Not fetched yet')}
          </span>
        </span>
        {coolingDown && (
          <span>
            {t('Refresh is temporarily paused to respect platform limits.')}
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
      </p>
    </li>
  )
}
