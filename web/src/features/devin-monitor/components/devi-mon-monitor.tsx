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

import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { ChevronDown, RefreshCw } from 'lucide-react'
import { useEffect, useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { EmptyState } from '@/components/empty-state'
import { ErrorState } from '@/components/error-state'
import { Button } from '@/components/ui/button'
import { Skeleton } from '@/components/ui/skeleton'
import { SubscriptionSortControl } from '@/features/upstream-subscriptions/components/subscription-sort-control'
import { formatTimestampToDate } from '@/lib/format'

import { collectDeviMon, deviMonQueryKey, getDeviMonState } from '../api'
import { sortDevinAccounts, type DevinSort } from '../sort'
import type { DeviMonState } from '../types'
import { DeviMonRow } from './devi-mon-row'
import { DevinAccountStatus } from './devi-mon-status'

export function DeviMonMonitor(props: {
  readonly sort: DevinSort
  readonly onSortChange: (sort: DevinSort) => void
  readonly onOpenSettings?: () => void
}) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const [now, setNow] = useState(() => Date.now())
  const state = useQuery({
    queryKey: deviMonQueryKey,
    queryFn: ({ signal }) => getDeviMonState(signal),
    refetchInterval: (query) => (query.state.data?.collecting ? 3_000 : 30_000),
    retry: false,
  })
  useEffect(() => {
    const timer = window.setInterval(() => setNow(Date.now()), 30_000)
    return () => window.clearInterval(timer)
  }, [])
  const collect = useMutation({
    mutationFn: collectDeviMon,
    onMutate: () =>
      queryClient.cancelQueries({ queryKey: deviMonQueryKey, exact: true }),
    onSuccess: async (data: DeviMonState) => {
      await queryClient.cancelQueries({
        queryKey: deviMonQueryKey,
        exact: true,
      })
      queryClient.setQueryData(deviMonQueryKey, data)
      setNow(Date.now())
    },
  })

  const snapshot = state.data?.snapshot
  const accounts = useMemo(
    () => sortDevinAccounts(snapshot?.accounts ?? [], props.sort),
    [snapshot?.accounts, props.sort]
  )
  const unpaired = useMemo(
    () => sortDevinAccounts(snapshot?.unpaired_devin ?? [], props.sort),
    [snapshot?.unpaired_devin, props.sort]
  )

  if (state.isLoading) {
    return (
      <div
        role='status'
        aria-label={t('Loading monitor data')}
        className='space-y-4'
      >
        <Skeleton className='h-40 w-full' />
        <Skeleton className='h-40 w-full' />
      </div>
    )
  }
  if (state.error || !state.data) {
    return (
      <ErrorState
        description={t('Failed to load the Devin monitor')}
        onRetry={() => void state.refetch()}
      />
    )
  }

  const data = state.data
  const busy = data.collecting || collect.isPending

  let nextRun = t('Not scheduled')
  if (data.next_run_at > 0) nextRun = formatTimestampToDate(data.next_run_at)
  else if (data.collecting) {
    const interval = data.interval_minutes ?? snapshot?.interval_minutes
    nextRun = interval
      ? t('{{minutes}} minutes after this collection finishes', {
          minutes: interval,
        })
      : t('Collecting')
  } else if (data.configured) nextRun = t('Awaiting first collection')

  const collectedAt = snapshot
    ? formatTimestampToDate(snapshot.collected_at)
    : t('Never')
  const seconds = Math.round((snapshot?.duration_ms ?? 0) / 1000)

  return (
    <div className='space-y-3'>
      <div className='bg-card flex flex-wrap items-center justify-between gap-3 rounded-lg border px-3 py-2.5 sm:px-4'>
        <div className='text-muted-foreground flex flex-wrap items-center gap-x-4 gap-y-1 text-xs'>
          <span>
            {t('Last collected')}:{' '}
            <span className='text-foreground tabular-nums'>{collectedAt}</span>
          </span>
          <span>
            {t('Next run')}:{' '}
            <span className='text-foreground tabular-nums'>{nextRun}</span>
          </span>
          {snapshot && (
            <span>
              {t('Took')}:{' '}
              <span className='text-foreground tabular-nums'>
                {seconds < 60
                  ? t('{{seconds}} sec', { seconds })
                  : t('{{minutes}} min {{seconds}} sec', {
                      minutes: Math.floor(seconds / 60),
                      seconds: seconds % 60,
                    })}
              </span>
            </span>
          )}
        </div>
        <div className='flex flex-wrap items-center gap-2'>
          {(accounts.length > 0 || unpaired.length > 0) && (
            <SubscriptionSortControl<DevinSort['field']>
              options={[
                { value: 'priority', label: t('Priority') },
                { value: 'reset', label: t('Reset time') },
                { value: 'renewal', label: t('Renewal time') },
              ]}
              value={props.sort.field}
              descending={props.sort.descending}
              onChange={(field) => {
                if (field === props.sort.field) return
                props.onSortChange({ field, descending: field === 'priority' })
              }}
              onToggle={() =>
                props.onSortChange({
                  ...props.sort,
                  descending: !props.sort.descending,
                })
              }
            />
          )}
          {props.onOpenSettings && (
            <Button variant='ghost' size='sm' onClick={props.onOpenSettings}>
              {t('Interval settings')}
            </Button>
          )}
          <Button
            variant='outline'
            size='sm'
            disabled={busy || !data.configured}
            onClick={() => collect.mutate()}
          >
            <RefreshCw
              className={busy ? 'animate-spin' : undefined}
              aria-hidden='true'
            />
            {busy ? t('Collecting...') : t('Collect now')}
          </Button>
        </div>
      </div>

      {!data.configured && (
        <p className='text-destructive text-xs'>
          {t('Set CPA_MANAGEMENT_KEY on the gateway before collecting.')}
        </p>
      )}

      {data.last_error && (
        <p role='status' className='text-destructive text-xs'>
          {t('Last collection failed')}: {data.last_error}{' '}
          {snapshot && t('Showing the last complete collection.')}
        </p>
      )}

      {snapshot && snapshot.errors.length > 0 && (
        <ul className='text-destructive space-y-0.5 text-xs'>
          {snapshot.errors.map((message) => (
            <li key={message}>{message}</li>
          ))}
        </ul>
      )}

      {accounts.length === 0 && (
        <EmptyState
          icon={ChevronDown}
          title={t('No Codex accounts collected yet')}
          description={t(
            'The monitor reads cpa-pro (Codex) and cpa-pro-1 (Devin) through the CPA management API.'
          )}
          action={
            <Button
              onClick={() => collect.mutate()}
              disabled={busy || !data.configured}
            >
              <RefreshCw aria-hidden='true' />
              {t('Collect now')}
            </Button>
          }
        />
      )}

      {accounts.length > 0 && (
        <ul
          className='bg-card @container/accounts divide-y overflow-hidden rounded-lg border'
          aria-label={t('Codex accounts')}
        >
          {accounts.map((account) => (
            <DeviMonRow key={account.email} account={account} now={now} />
          ))}
        </ul>
      )}

      {unpaired.length > 0 && (
        <div className='text-muted-foreground text-xs'>
          <p>{t('Devin accounts without a Codex twin')}</p>
          <ul className='mt-1 space-y-0.5'>
            {unpaired.map((devin) => (
              <li
                key={devin.email}
                className='flex flex-wrap items-center gap-x-2 gap-y-1'
              >
                <span className='min-w-0 [overflow-wrap:anywhere]'>
                  {devin.email}
                </span>
                <DevinAccountStatus
                  disabled={devin.disabled}
                  routing={devin.devin_routing}
                />
              </li>
            ))}
          </ul>
        </div>
      )}
    </div>
  )
}
