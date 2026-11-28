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
import {
  type MouseEvent,
  useEffect,
  useId,
  useMemo,
  useRef,
  useState,
} from 'react'
import { useTranslation } from 'react-i18next'

import { ConfirmDialog } from '@/components/confirm-dialog'
import { EmptyState } from '@/components/empty-state'
import { ErrorState } from '@/components/error-state'
import { Button } from '@/components/ui/button'
import { Label } from '@/components/ui/label'
import { Skeleton } from '@/components/ui/skeleton'
import { Switch } from '@/components/ui/switch'
import { SubscriptionSortControl } from '@/features/upstream-subscriptions/components/subscription-sort-control'
import { formatTimestampToDate } from '@/lib/format'
import { getServerErrorStatus } from '@/lib/server-error-message'

import {
  collectDeviMon,
  deviMonQueryKey,
  getDeviMonState,
  getDeviMonInventory,
  deviMonInventoryKey,
} from '../api'
import { DevinOperationMode } from '../operation-mode'
import { sortDevinAccounts, type DevinSort } from '../sort'
import type { DeviMonRouting, DevinRequests } from '../types'
import { DeviMonAccountActions } from './devi-mon-account-actions'
import { DeviMonAccountSettings } from './devi-mon-account-settings'
import { DevinOperationGroup } from './devi-mon-operation-group'
import { DevinRequestHistory } from './devi-mon-request-history'
import { DeviMonRow } from './devi-mon-row'

// Match the four-minute auth-files refresh in CPA Management Center.
const INVENTORY_INTERVAL_MS = 4 * 60_000

function canPollInventory(error: unknown) {
  const status = getServerErrorStatus(error)
  return status !== 401 && status !== 403
}

// An account without a Codex twin is exactly the one that may need the other
// channel, so it carries the same per-account settings button as a normal row.
function UnpairedDevinRow(props: {
  readonly email: string
  readonly disabled: boolean
  readonly requests?: DevinRequests | null
  readonly routing?: DeviMonRouting | null
  readonly configured: boolean
}) {
  const [settingsOpen, setSettingsOpen] = useState(false)
  return (
    <li className='grid min-w-0 items-center gap-x-5 gap-y-1.5 px-3 py-2.5 sm:px-4 @2xl/accounts:grid-cols-[minmax(0,1fr)_minmax(10rem,1.35fr)_auto]'>
      <span className='min-w-0 self-center text-sm font-semibold [overflow-wrap:anywhere]'>
        {props.email}
      </span>
      <DevinRequestHistory email={props.email} history={props.requests} />
      <DeviMonAccountActions
        email={props.email}
        disabled={props.disabled}
        routing={props.routing}
        configured={props.configured}
        onOpenSettings={() => setSettingsOpen(true)}
      />
      <DeviMonAccountSettings
        email={props.email}
        open={settingsOpen}
        onOpenChange={setSettingsOpen}
      />
    </li>
  )
}

export function DeviMonMonitor(props: {
  readonly sort: DevinSort
  readonly onSortChange: (sort: DevinSort) => void
  readonly onOpenSettings?: () => void
}) {
  const { t } = useTranslation()
  const modeId = useId()
  const [operationMode, setOperationMode] = useState(false)
  const [confirmCollect, setConfirmCollect] = useState(false)
  const collectTrigger = useRef<HTMLButtonElement>(null)
  function requestCollection(event: MouseEvent<HTMLButtonElement>) {
    if (!operationMode) return
    collectTrigger.current = event.currentTarget
    setConfirmCollect(true)
  }
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
    onSuccess: async () => {
      const query = { queryKey: deviMonQueryKey, exact: true }
      await queryClient.cancelQueries(query)
      await queryClient.invalidateQueries(query)
      setNow(Date.now())
    },
  })

  const snapshot = state.data?.snapshot
  const inventory = useQuery({
    queryKey: deviMonInventoryKey,
    queryFn: ({ signal }) => getDeviMonInventory(signal),
    enabled: !!state.data?.configured,
    refetchInterval: ({ state }) =>
      canPollInventory(state.error) ? INVENTORY_INTERVAL_MS : false,
    refetchOnWindowFocus: ({ state }) => canPollInventory(state.error),
    refetchIntervalInBackground: false,
    staleTime: INVENTORY_INTERVAL_MS,
    retry: false,
  })
  const live = inventory.data
  const accounts = useMemo(
    () =>
      sortDevinAccounts(
        (snapshot?.accounts ?? []).map((row) => {
          if (!live) return row
          const current = live[row.email.toLowerCase()]
          return {
            ...row,
            devin_routing: current?.devin_routing,
            devin_disabled: current?.disabled ?? null,
            devin_requests: current?.requests,
          }
        }),
        props.sort
      ),
    [snapshot?.accounts, live, props.sort]
  )
  const unpaired = useMemo(() => {
    if (!live) {
      return sortDevinAccounts(snapshot?.unpaired_devin ?? [], props.sort)
    }
    const paired = new Set(
      (snapshot?.accounts ?? []).map((row) => row.email.toLowerCase())
    )
    return sortDevinAccounts(
      Object.values(live).filter((row) => !paired.has(row.email.toLowerCase())),
      props.sort
    )
  }, [live, snapshot, props.sort])

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
  if (!state.data) {
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
  const collectLabel = busy ? t('Collecting...') : t('Collect now')
  const collectButton = (
    <DevinOperationGroup label={collectLabel}>
      <Button
        variant='outline'
        size='sm'
        disabled={busy || !data.configured}
        onClick={requestCollection}
      >
        <RefreshCw
          className={busy ? 'animate-spin' : undefined}
          aria-hidden='true'
        />
        {collectLabel}
      </Button>
    </DevinOperationGroup>
  )

  return (
    <DevinOperationMode value={operationMode}>
      <div className='space-y-3'>
        <div className='bg-card flex flex-wrap items-center justify-between gap-3 rounded-lg border px-3 py-2.5 sm:px-4'>
          <div className='text-muted-foreground flex flex-wrap items-center gap-x-4 gap-y-1 text-xs'>
            <span>
              {t('Last collected')}:{' '}
              <span className='text-foreground tabular-nums'>
                {collectedAt}
              </span>
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
            <Label
              htmlFor={modeId}
              className='min-h-8 cursor-pointer gap-1.5 px-1 text-xs whitespace-nowrap pointer-coarse:min-h-11'
            >
              <Switch
                id={modeId}
                size='sm'
                checked={operationMode}
                onCheckedChange={setOperationMode}
              />
              {t('Operation mode')}
            </Label>
            {(accounts.length > 0 || unpaired.length > 0) && (
              <DevinOperationGroup label={t('Sort subscriptions by')}>
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
                    props.onSortChange({
                      field,
                      descending: field === 'priority',
                    })
                  }}
                  onToggle={() =>
                    props.onSortChange({
                      ...props.sort,
                      descending: !props.sort.descending,
                    })
                  }
                />
              </DevinOperationGroup>
            )}
            {props.onOpenSettings && (
              <Button variant='ghost' size='sm' onClick={props.onOpenSettings}>
                {t('Interval settings')}
              </Button>
            )}
            {collectButton}
          </div>
        </div>

        <ConfirmDialog
          open={operationMode && confirmCollect}
          onOpenChange={setConfirmCollect}
          finalFocus={collectTrigger}
          title={t('Collect all accounts?')}
          desc={t(
            'Each monitored account’s quota will be checked in the background. Its policy may enable or disable the Devin account.'
          )}
          confirmText={t('Collect now')}
          disabled={busy || !data.configured}
          handleConfirm={() => {
            if (!operationMode || busy || !data.configured) return
            setConfirmCollect(false)
            collect.mutate()
          }}
        />
        {!data.configured && (
          <p className='text-destructive text-xs'>
            {t('Set CPA_MANAGEMENT_KEY on the gateway before collecting.')}
          </p>
        )}

        {inventory.isError && (
          <p
            role='status'
            className='text-muted-foreground flex items-center gap-2 text-xs'
          >
            {t('Live Devin data unavailable. Showing the last reading.')}{' '}
            <Button
              size='xs'
              variant='outline'
              disabled={inventory.isFetching}
              onClick={() => void inventory.refetch()}
            >
              {t('Retry')}
            </Button>
          </p>
        )}
        {data.last_error && (
          <p role='status' className='text-destructive text-xs'>
            {t('Last collection failed')}:{' '}
            {data.last_error === 'task lease expired'
              ? t('Collection interrupted')
              : data.last_error}{' '}
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
            action={collectButton}
          />
        )}

        {accounts.length > 0 && (
          <ul
            className='bg-card @container/accounts divide-y overflow-hidden rounded-lg border'
            aria-label={t('Codex accounts')}
          >
            {accounts.map((account) => (
              <DeviMonRow
                key={account.email}
                account={account}
                now={now}
                configured={data.configured}
              />
            ))}
          </ul>
        )}

        {unpaired.length > 0 && (
          <div className='text-muted-foreground text-xs'>
            <p>
              {snapshot ? t('Devin accounts without a Codex twin') : t('Devin')}
            </p>
            <ul className='bg-card text-foreground @container/accounts mt-2 divide-y rounded-lg border'>
              {unpaired.map((devin) => (
                <UnpairedDevinRow
                  key={devin.email}
                  email={devin.email}
                  disabled={devin.disabled}
                  routing={devin.devin_routing}
                  requests={devin.requests}
                  configured={data.configured}
                />
              ))}
            </ul>
          </div>
        )}
      </div>
    </DevinOperationMode>
  )
}
