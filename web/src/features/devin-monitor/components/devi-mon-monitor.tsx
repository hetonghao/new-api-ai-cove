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
import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { EmptyState } from '@/components/empty-state'
import { ErrorState } from '@/components/error-state'
import { Button } from '@/components/ui/button'
import { Skeleton } from '@/components/ui/skeleton'
import dayjs from '@/lib/dayjs'

import { collectDeviMon, deviMonQueryKey, getDeviMonState } from '../api'
import type { DeviMonState } from '../types'
import { DeviMonRow } from './devi-mon-row'

export function DeviMonMonitor(props: { readonly onOpenSettings?: () => void }) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const [now, setNow] = useState(() => Date.now())
  const state = useQuery({
    queryKey: deviMonQueryKey,
    queryFn: ({ signal }) => getDeviMonState(signal),
    refetchInterval: 30_000,
    retry: false,
  })
  useEffect(() => {
    const timer = window.setInterval(() => setNow(Date.now()), 30_000)
    return () => window.clearInterval(timer)
  }, [])
  const collect = useMutation({
    mutationFn: collectDeviMon,
    onSuccess: (data: DeviMonState) => {
      queryClient.setQueryData(deviMonQueryKey, data)
      setNow(Date.now())
    },
  })

  if (state.isLoading) {
    return (
      <div role='status' aria-label={t('Loading monitor data')} className='space-y-4'>
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
  const snapshot = data.snapshot
  const busy = data.collecting || collect.isPending
  const accounts = snapshot?.accounts ?? []
  const unpaired = snapshot?.unpaired_devin ?? []

  let nextRun = t('Not scheduled')
  if (data.next_run_at > 0) {
    nextRun = dayjs(data.next_run_at * 1000).format('MM/DD HH:mm')
  }
  const collectedAt = snapshot
    ? dayjs(snapshot.collected_at * 1000).format('YYYY-MM-DD HH:mm')
    : t('Never')

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
                {Math.round(snapshot.duration_ms / 1000)}s
              </span>
            </span>
          )}
        </div>
        <div className='flex items-center gap-2'>
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
            <Button onClick={() => collect.mutate()} disabled={busy || !data.configured}>
              <RefreshCw aria-hidden='true' />
              {t('Collect now')}
            </Button>
          }
        />
      )}

      {accounts.length > 0 && (
        <ul
          className='bg-card divide-y overflow-hidden rounded-lg border'
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
              <li key={devin.email}>
                {devin.email}{' '}
                <span className={devin.disabled ? 'text-destructive' : 'text-success'}>
                  {devin.disabled ? t('Devin disabled') : t('Devin enabled')}
                </span>
              </li>
            ))}
          </ul>
        </div>
      )}
    </div>
  )
}
