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
import { useTranslation } from 'react-i18next'

import { Dialog } from '@/components/dialog'
import { StatusBadge } from '@/components/status-badge'
import { Skeleton } from '@/components/ui/skeleton'
import { cn } from '@/lib/utils'

import { getQualityArtifact } from '../api'
import { SAMPLE_STATUS_VARIANT, sampleStatusLabel } from '../constants'
import { formatDurationMs, sanitizeSvg, svgDataUri } from '../lib/quality-view'
import type { ModelQualitySample } from '../types'

type CompareDialogProps = {
  samples: readonly ModelQualitySample[]
  open: boolean
  onOpenChange: (open: boolean) => void
  showChannel: boolean
}

function ComparePreview(props: { sample: ModelQualitySample }) {
  const { t } = useTranslation()
  const enabled =
    props.sample.status === 'succeeded' && !props.sample.artifact_expired
  const artifactQuery = useQuery({
    queryKey: ['model-quality', 'artifact', props.sample.id],
    queryFn: async () => {
      const response = await getQualityArtifact(props.sample.id)
      if (!response.success || !response.data) {
        throw new Error(response.message)
      }
      return response.data
    },
    enabled,
    staleTime: Infinity,
    retry: false,
  })

  if (!enabled) {
    return (
      <div className='bg-muted text-muted-foreground flex aspect-[3/2] items-center justify-center text-xs'>
        {t(sampleStatusLabel(props.sample.status))}
      </div>
    )
  }
  if (artifactQuery.isPending) {
    return <Skeleton className='aspect-[3/2] w-full' />
  }
  const artifact = artifactQuery.data
  if (!artifact) {
    return (
      <div className='bg-muted text-muted-foreground flex aspect-[3/2] items-center justify-center text-xs'>
        {t('Preview unavailable')}
      </div>
    )
  }
  if (props.sample.output_type === 'svg') {
    const sanitized = sanitizeSvg(artifact.svg)
    if (sanitized === '') {
      return (
        <div className='bg-muted text-muted-foreground flex aspect-[3/2] items-center justify-center text-xs'>
          {t('Preview blocked')}
        </div>
      )
    }
    return (
      <div className='flex aspect-[3/2] items-center justify-center bg-white'>
        <img
          src={svgDataUri(sanitized)}
          alt={t('Sample preview')}
          className='h-full w-full object-contain'
        />
      </div>
    )
  }
  return (
    <p className='bg-muted line-clamp-6 aspect-[3/2] overflow-hidden p-3 text-xs break-words whitespace-pre-wrap'>
      {artifact.text}
    </p>
  )
}

export function CompareDialog(props: CompareDialogProps) {
  const { t } = useTranslation()
  const samples = props.samples
  let cols = 'sm:grid-cols-2'
  if (samples.length === 3) {
    cols = 'sm:grid-cols-3'
  } else if (samples.length >= 4) {
    cols = 'sm:grid-cols-4'
  }

  return (
    <Dialog
      open={props.open}
      onOpenChange={props.onOpenChange}
      title={t('Compare samples')}
      contentClassName='sm:max-w-5xl'
    >
      <div className='space-y-3'>
        <div className={cn('grid grid-cols-1 gap-3', cols)}>
          {samples.map((sample) => (
            <div key={sample.id} className='space-y-1 rounded-lg border'>
              <ComparePreview sample={sample} />
              <div className='space-y-0.5 px-2 pb-2 text-xs'>
                {props.showChannel && (
                  <div className='flex items-center justify-between gap-2'>
                    <span
                      className='truncate'
                      title={sample.channel_name || `#${sample.channel_id}`}
                    >
                      {sample.channel_id === 0
                        ? t('Unattributed channel')
                        : sample.channel_name || `#${sample.channel_id}`}
                    </span>
                  </div>
                )}
                <div className='flex items-center justify-between gap-2'>
                  <span className='tabular-nums'>
                    {formatDurationMs(sample.duration_ms)}
                  </span>
                  <StatusBadge
                    variant={SAMPLE_STATUS_VARIANT[sample.status] ?? 'neutral'}
                    label={t(sampleStatusLabel(sample.status))}
                    copyable={false}
                  />
                </div>
              </div>
            </div>
          ))}
        </div>
      </div>
    </Dialog>
  )
}
