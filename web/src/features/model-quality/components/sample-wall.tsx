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
import { useQuery } from '@tanstack/react-query'
import {
  AlertTriangle,
  GitCompareArrows,
  ImageOff,
  Loader2,
  Tag,
} from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { EmptyState } from '@/components/empty-state'
import { StatusBadge } from '@/components/status-badge'
import { Button } from '@/components/ui/button'
import { Checkbox } from '@/components/ui/checkbox'
import { Skeleton } from '@/components/ui/skeleton'
import dayjs from '@/lib/dayjs'
import { formatTimestampToDate } from '@/lib/format'
import { cn } from '@/lib/utils'

import { getQualityArtifact } from '../api'
import {
  errorCodeLabel,
  MAX_COMPARE_SAMPLES,
  SAMPLE_STATUS_VARIANT,
  sampleStatusLabel,
} from '../constants'
import {
  formatDurationMs,
  isTerminalSample,
  sanitizeSvg,
  svgDataUri,
} from '../lib/quality-view'
import type { ModelQualitySample } from '../types'

type SampleCardProps = {
  sample: ModelQualitySample
  selectable: boolean
  selected: boolean
  showChannel: boolean
  onToggleSelect: (sample: ModelQualitySample) => void
  onOpen: (sample: ModelQualitySample) => void
}

function SampleArtifactPreview(props: { sample: ModelQualitySample }) {
  const { t } = useTranslation()
  const artifactQuery = useQuery({
    queryKey: ['model-quality', 'artifact', props.sample.id],
    queryFn: async () => {
      const response = await getQualityArtifact(props.sample.id)
      if (!response.success || !response.data) {
        throw new Error(response.message)
      }
      return response.data
    },
    staleTime: Infinity,
    retry: false,
  })
  if (artifactQuery.isPending) {
    return <Skeleton className='h-4/5 w-4/5' />
  }
  if (artifactQuery.isError) {
    return (
      <span className='text-muted-foreground text-xs'>
        {t('Preview unavailable')}
      </span>
    )
  }
  const sanitized = sanitizeSvg(artifactQuery.data.svg)
  if (sanitized === '') {
    return (
      <span className='text-muted-foreground text-xs'>
        {t('Preview blocked')}
      </span>
    )
  }
  return (
    <div className='h-full w-full bg-white'>
      <img
        src={svgDataUri(sanitized)}
        alt={t('Sample preview')}
        loading='lazy'
        className='h-full w-full object-contain'
      />
    </div>
  )
}

function SampleCardBody(props: { sample: ModelQualitySample }) {
  const { t } = useTranslation()
  const sample = props.sample

  if (sample.status === 'pending' || sample.status === 'requesting') {
    return (
      <div className='flex h-full flex-col items-center justify-center gap-2'>
        <Skeleton className='h-4 w-24' />
        <span className='text-muted-foreground flex items-center gap-1 text-xs'>
          <Loader2 className='size-3 animate-spin' aria-hidden='true' />
          {t(sampleStatusLabel(sample.status))}
        </span>
      </div>
    )
  }
  if (sample.status === 'cancelled') {
    return (
      <div className='text-muted-foreground flex h-full flex-col items-center justify-center gap-1'>
        <ImageOff className='size-6' aria-hidden='true' />
        <span className='text-xs'>{t('Cancelled')}</span>
      </div>
    )
  }
  if (sample.status !== 'succeeded') {
    const label =
      sample.error_code === ''
        ? t('Failed')
        : errorCodeLabel(sample.error_code, t)
    return (
      <div className='flex h-full flex-col items-center justify-center gap-1 px-3'>
        <AlertTriangle className='text-warning size-6' aria-hidden='true' />
        <span className='text-center text-xs break-all'>{label}</span>
        {sample.error_code !== '' && (
          <span className='text-muted-foreground font-mono text-xs break-all'>
            {sample.error_code}
          </span>
        )}
      </div>
    )
  }
  if (sample.artifact_expired) {
    return (
      <div className='text-muted-foreground flex h-full items-center justify-center text-xs'>
        {t('Artifact expired')}
      </div>
    )
  }
  if (sample.output_type === 'svg') {
    return <SampleArtifactPreview sample={sample} />
  }
  // Text samples render the final text via the artifact endpoint.
  return <SampleTextPreview sample={sample} />
}

function SampleTextPreview(props: { sample: ModelQualitySample }) {
  const { t } = useTranslation()
  const artifactQuery = useQuery({
    queryKey: ['model-quality', 'artifact', props.sample.id],
    queryFn: async () => {
      const response = await getQualityArtifact(props.sample.id)
      if (!response.success || !response.data) {
        throw new Error(response.message)
      }
      return response.data
    },
    staleTime: Infinity,
    retry: false,
  })
  if (artifactQuery.isPending) {
    return <Skeleton className='h-4/5 w-4/5' />
  }
  if (artifactQuery.isError || artifactQuery.data.text === '') {
    return (
      <span className='text-muted-foreground text-xs'>
        {t('Preview unavailable')}
      </span>
    )
  }
  return (
    <p className='text-foreground line-clamp-5 w-full px-3 text-xs break-words whitespace-pre-wrap'>
      {artifactQuery.data.text}
    </p>
  )
}

export function SampleCard(props: SampleCardProps) {
  const { t } = useTranslation()
  const sample = props.sample

  return (
    <div className='bg-card relative rounded-lg border'>
      <button
        type='button'
        onClick={() => props.onOpen(sample)}
        className='focus-visible:ring-ring block w-full rounded-lg text-left focus-visible:ring-2 focus-visible:outline-none'
      >
        <div className='bg-muted flex aspect-[3/2] items-center justify-center overflow-hidden rounded-t-lg'>
          <SampleCardBody sample={sample} />
        </div>
        <div className='flex items-center gap-2 px-3 py-2 text-xs'>
          {props.showChannel && (
            <span className='truncate' title={sample.channel_name}>
              {sample.channel_id === 0
                ? t('Unattributed channel')
                : sample.channel_name || `#${sample.channel_id}`}
            </span>
          )}
          <span
            className='text-muted-foreground shrink-0 tabular-nums'
            title={formatTimestampToDate(sample.created_at, 'milliseconds')}
          >
            {dayjs(sample.created_at).format('MM-DD HH:mm')}
          </span>
          {sample.annotation !== '' && (
            <Tag
              className='text-muted-foreground size-3 shrink-0'
              aria-hidden='true'
            />
          )}
          <span className='text-muted-foreground ml-auto tabular-nums'>
            {formatDurationMs(sample.duration_ms)}
          </span>
          <StatusBadge
            variant={SAMPLE_STATUS_VARIANT[sample.status] ?? 'neutral'}
            label={t(sampleStatusLabel(sample.status))}
            copyable={false}
          />
        </div>
      </button>
      {props.selectable && (
        <Checkbox
          aria-label={t('Select for compare')}
          checked={props.selected}
          onCheckedChange={() => props.onToggleSelect(sample)}
          onClick={(event) => event.stopPropagation()}
          className='bg-background absolute top-2 left-2'
        />
      )}
    </div>
  )
}

type SampleWallProps = {
  samples: readonly ModelQualitySample[]
  selectedIds: ReadonlySet<number>
  showChannel: boolean
  onToggleSelect: (sample: ModelQualitySample) => void
  onOpen: (sample: ModelQualitySample) => void
  hasMore: boolean
  isFetchingMore: boolean
  onLoadMore: () => void
}

export function SampleWall(props: SampleWallProps) {
  const { t } = useTranslation()

  if (props.samples.length === 0) {
    return (
      <EmptyState
        bordered
        title={t('No samples yet')}
        description={t(
          'Run a test to produce samples for this case and channel.'
        )}
      />
    )
  }
  return (
    <div className='space-y-3'>
      <div className='grid grid-cols-1 gap-3 sm:grid-cols-2 lg:grid-cols-4'>
        {props.samples.map((sample) => (
          <SampleCard
            key={sample.id}
            sample={sample}
            selectable={isTerminalSample(sample)}
            selected={props.selectedIds.has(sample.id)}
            showChannel={props.showChannel}
            onToggleSelect={props.onToggleSelect}
            onOpen={props.onOpen}
          />
        ))}
      </div>
      {props.hasMore && (
        <div className='flex justify-center'>
          <Button
            variant='outline'
            size='sm'
            disabled={props.isFetchingMore}
            onClick={props.onLoadMore}
          >
            {props.isFetchingMore ? t('Loading...') : t('Load more')}
          </Button>
        </div>
      )}
    </div>
  )
}

type CompareTrayProps = {
  count: number
  onCompare: () => void
  onClear: () => void
}

export function CompareTray(props: CompareTrayProps) {
  const { t } = useTranslation()
  if (props.count === 0) return null
  return (
    <div
      className={cn(
        'bg-popover ring-border fixed right-4 bottom-4 z-40 flex items-center gap-3',
        'rounded-lg p-3 shadow-lg ring-1'
      )}
    >
      <span className='text-sm'>
        {t('{{count}} selected', { count: props.count })}
      </span>
      <Button
        size='sm'
        disabled={props.count < 2 || props.count > MAX_COMPARE_SAMPLES}
        onClick={props.onCompare}
      >
        <GitCompareArrows data-icon='inline-start' />
        {t('Compare')}
      </Button>
      <Button size='sm' variant='ghost' onClick={props.onClear}>
        {t('Clear selection')}
      </Button>
    </div>
  )
}
