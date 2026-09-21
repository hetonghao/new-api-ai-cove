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
import { useInfiniteQuery, useQuery } from '@tanstack/react-query'
import { getRouteApi, useNavigate } from '@tanstack/react-router'
import { X } from 'lucide-react'
import { useEffect, useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { CopyButton } from '@/components/copy-button'
import { ErrorState } from '@/components/error-state'
import { StatusBadge } from '@/components/status-badge'
import { Button } from '@/components/ui/button'
import { Skeleton } from '@/components/ui/skeleton'
import { formatTimestampToDate } from '@/lib/format'

import { getQualityDashboard, getQualitySamples } from '../api'
import {
  SAMPLE_STATUS_VARIANT,
  SAMPLES_PAGE_SIZE,
  sampleStatusLabel,
} from '../constants'
import {
  formatPercentileMs,
  sampleEffectiveTime,
  scheduleSummary,
} from '../lib/quality-view'
import type {
  ModelQualitySample,
  QualityBucket,
  QualityCapabilities,
  QualityCaseView,
} from '../types'
import { ChannelSwitcher } from './channel-switcher'
import { CompareDialog } from './compare-dialog'
import { RunControls } from './run-controls'
import { SampleDetailDialog } from './sample-detail-dialog'
import { CompareTray, SampleWall } from './sample-wall'
import { TimelineStrip } from './timeline-strip'

const route = getRouteApi('/_authenticated/model-quality/')

type DashboardPanelProps = {
  qualityCase: QualityCaseView
  capabilities: QualityCapabilities | undefined
  onEdit: () => void
}

function MetricCell(props: { label: string; value: string }) {
  return (
    <div className='bg-card border rounded-lg p-3'>
      <div className='text-muted-foreground text-xs'>{props.label}</div>
      <div className='mt-1 text-lg font-medium tabular-nums'>{props.value}</div>
    </div>
  )
}

export function DashboardPanel(props: DashboardPanelProps) {
  const { t } = useTranslation()
  const navigate = useNavigate({ from: '/model-quality/' })
  const search = route.useSearch()
  const qualityCase = props.qualityCase
  const [selectedBucket, setSelectedBucket] = useState<QualityBucket | null>(
    null
  )
  const [selectedIds, setSelectedIds] = useState<Set<number>>(new Set())
  const [detailSample, setDetailSample] = useState<ModelQualitySample | null>(
    null
  )
  const [compareOpen, setCompareOpen] = useState(false)

  const setSearch = (patch: Record<string, number | undefined>) => {
    void navigate({
      search: (prev) => ({ ...prev, ...patch }),
      replace: true,
    })
  }

  const dashboardQuery = useQuery({
    queryKey: [
      'model-quality',
      'dashboard',
      qualityCase.id,
      search.channel ?? 'auto',
    ],
    queryFn: async () => {
      const response = await getQualityDashboard({
        caseId: qualityCase.id,
        version: -1,
        channel_id: search.channel,
      })
      if (!response.success || !response.data) {
        throw new Error(response.message)
      }
      return response.data
    },
    refetchInterval: qualityCase.active_run_id ? 2000 : 30000,
    refetchIntervalInBackground: false,
    retry: false,
  })
  const dashboard = dashboardQuery.data
  const channelId = dashboard?.channel_id

  // Keep the URL channel in sync with the server-selected default.
  useEffect(() => {
    if (dashboard && search.channel !== dashboard.channel_id) {
      setSearch({ channel: dashboard.channel_id })
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [dashboard?.channel_id])

  const samplesQuery = useInfiniteQuery({
    queryKey: [
      'model-quality',
      'samples',
      qualityCase.id,
      channelId,
      selectedBucket?.start ?? 0,
      selectedBucket?.end ?? 0,
    ],
    queryFn: async ({ pageParam }) => {
      if (channelId === undefined) throw new Error('channel unresolved')
      const response = await getQualitySamples({
        caseId: qualityCase.id,
        version: -1,
        channel_id: channelId,
        from_ms: selectedBucket?.start,
        to_ms: selectedBucket?.end,
        before_ms: pageParam?.ms,
        before_id: pageParam?.id,
        limit: SAMPLES_PAGE_SIZE,
      })
      if (!response.success || !response.data) {
        throw new Error(response.message)
      }
      return response.data
    },
    initialPageParam: undefined as { ms: number; id: number } | undefined,
    getNextPageParam: (lastPage) => {
      if (lastPage.length < SAMPLES_PAGE_SIZE) return undefined
      const last = lastPage.at(-1)
      if (!last) return undefined
      return { ms: sampleEffectiveTime(last), id: last.id }
    },
    enabled: channelId !== undefined,
    refetchInterval: qualityCase.active_run_id ? 2000 : 30000,
    refetchIntervalInBackground: false,
    retry: false,
  })
  const samples = useMemo(
    () => samplesQuery.data?.pages.flat() ?? [],
    [samplesQuery.data]
  )

  const toggleSelect = (sample: ModelQualitySample) => {
    setSelectedIds((prev) => {
      const next = new Set(prev)
      if (next.has(sample.id)) {
        next.delete(sample.id)
      } else if (next.size < 4) {
        next.add(sample.id)
      }
      return next
    })
  }
  const selectedSamples = samples.filter((sample) => selectedIds.has(sample.id))
  const summary = dashboard?.summary

  // A stale URL channel (not valid for this case/revision) 422s; dropping
  // the param lets the server pick the default channel and unblocks the panel.
  useEffect(() => {
    if (dashboardQuery.isError && search.channel !== undefined) {
      setSearch({ channel: undefined })
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [dashboardQuery.isError, search.channel])

  if (dashboardQuery.isError && search.channel === undefined) {
    return (
      <ErrorState
        description={t('Failed to load the dashboard')}
        onRetry={() => void dashboardQuery.refetch()}
      />
    )
  }
  if (!dashboard || !summary) {
    return (
      <div className='space-y-3'>
        <Skeleton className='h-20 w-full' />
        <Skeleton className='h-10 w-full' />
        <Skeleton className='h-64 w-full' />
      </div>
    )
  }

  const nonTerminalCounts: { status: string; count: number }[] = [
    { status: 'pending', count: summary.pending },
    { status: 'requesting', count: summary.running },
    { status: 'cancelled', count: summary.cancelled },
    { status: 'interrupted', count: summary.interrupted },
    { status: 'skipped', count: summary.skipped },
  ]

  const SKELETON_KEYS = ['s1', 's2', 's3', 's4', 's5', 's6', 's7', 's8']

  function samplesContent() {
    if (samplesQuery.isPending) {
      return (
        <div className='grid grid-cols-1 gap-3 sm:grid-cols-2 lg:grid-cols-4'>
          {SKELETON_KEYS.map((key) => (
            <Skeleton key={key} className='aspect-[3/2] w-full' />
          ))}
        </div>
      )
    }
    if (samplesQuery.isError) {
      return (
        <ErrorState
          description={t('Failed to load samples')}
          onRetry={() => void samplesQuery.refetch()}
        />
      )
    }
    return (
      <SampleWall
        samples={samples}
        selectedIds={selectedIds}
        onToggleSelect={toggleSelect}
        onOpen={setDetailSample}
        hasMore={samplesQuery.hasNextPage}
        isFetchingMore={samplesQuery.isFetchingNextPage}
        onLoadMore={() => void samplesQuery.fetchNextPage()}
      />
    )
  }

  return (
    <div className='space-y-4'>
      <div className='bg-card border rounded-lg p-3'>
        <div className='flex flex-wrap items-center gap-x-3 gap-y-2'>
          <span className='text-sm font-medium'>{qualityCase.name}</span>
          <span className='font-mono text-xs'>{qualityCase.config.model}</span>
          <span className='text-muted-foreground text-xs'>
            {qualityCase.config.mode === 'route'
              ? t('Normal routing')
              : t('Pinned channel')}
          </span>
          <span className='text-muted-foreground text-xs'>
            {scheduleSummary(qualityCase.schedule, t)}
            {qualityCase.schedule.enabled && qualityCase.next_run_at > 0 && (
              <>
                {' · '}
                {t('Next run')}:{' '}
                {formatTimestampToDate(qualityCase.next_run_at, 'milliseconds')}
              </>
            )}
          </span>
          <div className='ml-auto'>
            <RunControls
              qualityCase={qualityCase}
              canOperate={props.capabilities?.can_operate ?? false}
              onEdit={props.onEdit}
            />
          </div>
        </div>
        <details className='mt-2 text-xs'>
          <summary className='text-muted-foreground cursor-pointer'>
            {t('Prompt')}:{' '}
            <span className='text-foreground'>
              {qualityCase.config.prompt.split('\n')[0]}
            </span>
          </summary>
          <div className='mt-1 flex items-start gap-2'>
            <pre className='bg-muted flex-1 rounded p-2 break-words whitespace-pre-wrap'>
              {qualityCase.config.prompt}
            </pre>
            <CopyButton value={qualityCase.config.prompt} />
          </div>
        </details>
        <p className='text-muted-foreground mt-1 text-xs'>
          {t(
            'Observes the local gateway route; results reflect this execution token and group only.'
          )}
        </p>
      </div>

      <ChannelSwitcher
        channels={dashboard.channels}
        value={channelId}
        onChange={(id) => setSearch({ channel: id })}
      />

      <div className='grid grid-cols-2 gap-3 sm:grid-cols-3 lg:grid-cols-6'>
        <MetricCell label={t('Succeeded')} value={String(summary.success)} />
        <MetricCell label={t('Failed')} value={String(summary.failure)} />
        <MetricCell label={t('Terminal samples')} value={String(summary.total)} />
        <MetricCell
          label={t('Success rate')}
          value={
            summary.success_rate == null
              ? '—'
              : `${Math.round(summary.success_rate * 100)}%`
          }
        />
        <MetricCell
          label={t('P50 duration')}
          value={formatPercentileMs(summary.p50_ms, summary.total, 'p50', t)}
        />
        <MetricCell
          label={t('P95 duration')}
          value={formatPercentileMs(summary.p95_ms, summary.total, 'p95', t)}
        />
      </div>

      <div className='bg-card border space-y-2 rounded-lg p-3'>
        <div className='flex items-center justify-between gap-2 text-xs'>
          <span className='text-muted-foreground truncate'>
            {t('Window {{start}} – {{end}}', {
              start: formatTimestampToDate(
                dashboard.window_start,
                'milliseconds'
              ),
              end: formatTimestampToDate(dashboard.window_end, 'milliseconds'),
            })}
          </span>
          <span className='text-muted-foreground shrink-0 tabular-nums'>
            {t(
              '{{success}} succeeded · {{failure}} failed · {{total}} terminal',
              {
                success: summary.success,
                failure: summary.failure,
                total: summary.total,
              }
            )}
          </span>
        </div>
        <TimelineStrip
          buckets={dashboard.buckets}
          selectedBucket={selectedBucket}
          onSelect={setSelectedBucket}
        />
      </div>

      <div className='flex flex-wrap items-center gap-2'>
        {nonTerminalCounts
          .filter((entry) => entry.count > 0)
          .map((entry) => (
            <StatusBadge
              key={entry.status}
              variant={SAMPLE_STATUS_VARIANT[entry.status] ?? 'neutral'}
              copyable={false}
              label={`${t(sampleStatusLabel(entry.status))} ${entry.count}`}
            />
          ))}
      </div>

      {selectedBucket && (
        <div className='flex items-center gap-2'>
          <StatusBadge
            variant='info'
            copyable={false}
            label={t('{{start}} – {{end}}', {
              start: formatTimestampToDate(
                selectedBucket.start,
                'milliseconds'
              ),
              end: formatTimestampToDate(selectedBucket.end, 'milliseconds'),
            })}
          />
          <Button
            size='sm'
            variant='ghost'
            onClick={() => setSelectedBucket(null)}
          >
            <X data-icon='inline-start' />
            {t('Clear filter')}
          </Button>
        </div>
      )}

      {samplesContent()}

      <CompareTray
        count={selectedIds.size}
        onCompare={() => setCompareOpen(true)}
        onClear={() => setSelectedIds(new Set())}
      />
      <CompareDialog
        samples={selectedSamples}
        open={compareOpen}
        onOpenChange={setCompareOpen}
      />
      <SampleDetailDialog
        sample={detailSample}
        open={detailSample !== null}
        onOpenChange={(open) => !open && setDetailSample(null)}
        canOperate={props.capabilities?.can_operate ?? false}
      />
    </div>
  )
}
