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
import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import { Dialog } from '@/components/dialog'
import { ErrorState } from '@/components/error-state'
import { StatusBadge } from '@/components/status-badge'
import { Skeleton } from '@/components/ui/skeleton'
import { formatTimestampToDate } from '@/lib/format'

import { getQualityRun } from '../api'
import {
  RUN_SOURCE_LABEL,
  RUN_STATUS_VARIANT,
  SAMPLE_STATUS_VARIANT,
  errorCodeLabel,
  runStatusLabel,
  sampleStatusLabel,
} from '../constants'
import { formatDurationMs } from '../lib/quality-view'
import type { ModelQualitySample } from '../types'
import { SampleDetailDialog } from './sample-detail-dialog'

type RunDetailDialogProps = {
  runId: string | null
  open: boolean
  onOpenChange: (open: boolean) => void
  canOperate: boolean
}

export function RunDetailDialog(props: RunDetailDialogProps) {
  const { t } = useTranslation()
  const [detailSample, setDetailSample] = useState<ModelQualitySample | null>(
    null
  )

  const runQuery = useQuery({
    queryKey: ['model-quality', 'run', props.runId],
    queryFn: async () => {
      const runId = props.runId
      if (!runId) throw new Error('missing run id')
      const response = await getQualityRun(runId)
      if (!response.success || !response.data) {
        throw new Error(response.message)
      }
      return response.data
    },
    enabled: props.open && props.runId !== null,
    retry: false,
  })
  const run = runQuery.data?.run
  const samples = runQuery.data?.samples ?? []

  function dialogBody() {
    if (runQuery.isPending) {
      return <Skeleton className='h-64 w-full' />
    }
    if (runQuery.isError || !run) {
      return (
        <ErrorState
          description={t('Failed to load the run')}
          onRetry={() => void runQuery.refetch()}
        />
      )
    }
    return runContent()
  }

  function runContent() {
    if (!run) return null
    return (
          <div className='space-y-3'>
            <dl className='grid grid-cols-2 gap-x-4 gap-y-1 text-sm sm:grid-cols-3'>
              <div>
                <dt className='text-muted-foreground text-xs'>{t('Status')}</dt>
                <dd>
                  <StatusBadge
                    variant={RUN_STATUS_VARIANT[run.status] ?? 'neutral'}
                    copyable={false}
                    label={t(runStatusLabel(run.status))}
                  />
                </dd>
              </div>
              <div>
                <dt className='text-muted-foreground text-xs'>{t('Source')}</dt>
                <dd>
                  {RUN_SOURCE_LABEL[run.source]
                    ? t(RUN_SOURCE_LABEL[run.source])
                    : run.source}
                </dd>
              </div>
              <div>
                <dt className='text-muted-foreground text-xs'>
                  {t('Created')}
                </dt>
                <dd className='tabular-nums'>
                  {formatTimestampToDate(run.created_at, 'milliseconds')}
                </dd>
              </div>
              <div>
                <dt className='text-muted-foreground text-xs'>
                  {t('Finished')}
                </dt>
                <dd className='tabular-nums'>
                  {run.finished_at > 0
                    ? formatTimestampToDate(run.finished_at, 'milliseconds')
                    : '—'}
                </dd>
              </div>
              <div>
                <dt className='text-muted-foreground text-xs'>
                  {t('Samples')}
                </dt>
                <dd className='tabular-nums'>{run.sample_count}</dd>
              </div>
            </dl>
            <div className='border divide-border divide-y rounded-lg'>
              {samples.length === 0 && (
                <div className='text-muted-foreground p-3 text-sm'>
                  {t('No samples yet')}
                </div>
              )}
              {samples.map((sample) => (
                <button
                  key={sample.id}
                  type='button'
                  onClick={() => setDetailSample(sample)}
                  className='hover:bg-muted/50 flex w-full items-center gap-2 px-3 py-2 text-left text-sm'
                >
                  <span className='text-muted-foreground w-8 tabular-nums'>
                    #{sample.ordinal}
                  </span>
                  <span
                    className='min-w-0 flex-1 truncate'
                    title={sample.channel_name}
                  >
                    {sample.channel_id === 0
                      ? t('Unattributed channel')
                      : sample.channel_name || `#${sample.channel_id}`}
                  </span>
                  <StatusBadge
                    variant={SAMPLE_STATUS_VARIANT[sample.status] ?? 'neutral'}
                    copyable={false}
                    label={t(sampleStatusLabel(sample.status))}
                  />
                  <span className='text-muted-foreground w-14 text-right tabular-nums'>
                    {formatDurationMs(sample.duration_ms)}
                  </span>
                  <span className='text-muted-foreground w-28 truncate font-mono text-xs'>
                    {sample.error_code === ''
                      ? ''
                      : errorCodeLabel(sample.error_code, t)}
                  </span>
                </button>
              ))}
            </div>
          </div>
        )
  }

  return (
    <>
      <Dialog
        open={props.open}
        onOpenChange={props.onOpenChange}
        title={t('Run detail')}
        contentClassName='sm:max-w-3xl'
      >
        {dialogBody()}
      </Dialog>
      <SampleDetailDialog
        sample={detailSample}
        open={detailSample !== null}
        onOpenChange={(open) => !open && setDetailSample(null)}
        canOperate={props.canOperate}
      />
    </>
  )
}
