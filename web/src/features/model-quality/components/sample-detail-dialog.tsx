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
import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { CopyButton } from '@/components/copy-button'
import { Dialog } from '@/components/dialog'
import { StatusBadge } from '@/components/status-badge'
import { Button } from '@/components/ui/button'
import { NativeSelect, NativeSelectOption } from '@/components/ui/native-select'
import { Skeleton } from '@/components/ui/skeleton'
import { Switch } from '@/components/ui/switch'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { Textarea } from '@/components/ui/textarea'
import { formatTimestampToDate } from '@/lib/format'
import { handleServerError } from '@/lib/handle-server-error'

import { annotateQualitySample, getQualityArtifact } from '../api'
import {
  ANNOTATION_OPTIONS,
  errorCodeLabel,
  SAMPLE_STATUS_VARIANT,
  sampleStatusLabel,
} from '../constants'
import { formatDurationMs, sanitizeSvg, svgDataUri } from '../lib/quality-view'
import type { ModelQualitySample } from '../types'

type SampleDetailDialogProps = {
  sample: ModelQualitySample | null
  open: boolean
  onOpenChange: (open: boolean) => void
  canOperate: boolean
  showChannel: boolean
}

function MetaRow(props: { label: string; children: React.ReactNode }) {
  return (
    <div className='grid grid-cols-[140px_minmax(0,1fr)] items-center gap-2 py-1 text-sm'>
      <dt className='text-muted-foreground'>{props.label}</dt>
      <dd className='min-w-0'>{props.children}</dd>
    </div>
  )
}

export function SampleDetailDialog(props: SampleDetailDialogProps) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const sample = props.sample
  const [tag, setTag] = useState('')
  const [note, setNote] = useState('')
  const [pinned, setPinned] = useState(false)

  useEffect(() => {
    setTag(sample?.annotation ?? '')
    setNote(sample?.note ?? '')
    setPinned(sample?.pinned ?? false)
  }, [sample])

  const artifactQuery = useQuery({
    queryKey: ['model-quality', 'artifact', sample?.id ?? 0],
    queryFn: async () => {
      const sampleId = sample?.id
      if (sampleId === undefined) throw new Error('missing sample id')
      const response = await getQualityArtifact(sampleId)
      if (!response.success || !response.data) {
        throw new Error(response.message)
      }
      return response.data
    },
    enabled:
      props.open &&
      sample !== null &&
      (sample.status === 'succeeded' || sample.status === 'failed'),
    staleTime: Infinity,
    retry: false,
    meta: { errorToast: false },
  })

  const annotateMutation = useMutation({
    mutationFn: () => {
      if (!sample) throw new Error('missing sample')
      return annotateQualitySample(sample.id, { tag, note, pinned })
    },
    retry: false,
    onSuccess: (response) => {
      if (!response.success) {
        toast.error(response.message || t('Request failed'))
        return
      }
      toast.success(t('Annotation saved'))
      void queryClient.invalidateQueries({
        queryKey: ['model-quality', 'samples'],
      })
    },
    onError: (error) => {
      handleServerError(error, t('Failed to save the annotation'))
    },
  })

  function downloadSvg() {
    const artifact = artifactQuery.data
    if (!artifact || artifact.svg === '') return
    const sanitized = sanitizeSvg(artifact.svg)
    if (sanitized === '') return
    const blob = new Blob([sanitized], { type: 'image/svg+xml' })
    const url = URL.createObjectURL(blob)
    const anchor = document.createElement('a')
    anchor.href = url
    anchor.download = `sample-${sample?.id ?? 'unknown'}.svg`
    anchor.click()
    URL.revokeObjectURL(url)
  }

  if (!sample) return null
  const artifact = artifactQuery.data
  const sanitizedSvg = artifact ? sanitizeSvg(artifact.svg) : ''
  let source = ''
  if (artifact) {
    source =
      sample.output_type === 'svg' && artifact.svg !== ''
        ? artifact.svg
        : artifact.text
  }
  let errorLabel = '—'
  if (sample.error_code !== '') {
    errorLabel = errorCodeLabel(sample.error_code, t)
  }

  function previewContent(sample: ModelQualitySample) {
    if (sample.status !== 'succeeded') {
      return (
        <div className='text-muted-foreground flex min-h-40 flex-col items-center justify-center gap-1'>
          <StatusBadge
            variant={SAMPLE_STATUS_VARIANT[sample.status] ?? 'neutral'}
            label={t(sampleStatusLabel(sample.status))}
            copyable={false}
          />
          <span className='text-sm'>{errorLabel}</span>
          {sample.error_code !== '' && (
            <span className='font-mono text-xs'>{sample.error_code}</span>
          )}
        </div>
      )
    }
    if (artifactQuery.isPending) {
      return <Skeleton className='h-64 w-full' />
    }
    if (sample.output_type !== 'svg') {
      return (
        <pre className='bg-muted rounded-lg p-3 text-sm whitespace-pre-wrap'>
          {artifact?.text || '—'}
        </pre>
      )
    }
    if (sanitizedSvg === '') {
      return (
        <div className='text-muted-foreground flex min-h-40 items-center justify-center text-sm'>
          {t('Preview blocked')}
        </div>
      )
    }
    return (
      <div className='flex items-center justify-center rounded-lg bg-white p-2'>
        <img
          src={svgDataUri(sanitizedSvg)}
          alt={t('Sample preview')}
          className='max-h-96 object-contain'
        />
      </div>
    )
  }

  const footer = (
    <div className='space-y-3 border-t pt-3'>
      {sample.output_type === 'svg' &&
        sample.status === 'succeeded' &&
        sanitizedSvg !== '' && (
          <div>
            <Button size='sm' variant='outline' onClick={downloadSvg}>
              {t('Download SVG')}
            </Button>
          </div>
        )}
      <div className='flex flex-wrap items-end gap-3'>
        <div className='space-y-1'>
          <span className='text-sm font-medium'>{t('Annotation')}</span>
          <NativeSelect
            aria-label={t('Annotation tag')}
            value={tag}
            disabled={!props.canOperate}
            onChange={(event) => setTag(event.target.value)}
          >
            {ANNOTATION_OPTIONS.map((option) => (
              <NativeSelectOption key={option.value} value={option.value}>
                {t(option.labelKey)}
              </NativeSelectOption>
            ))}
          </NativeSelect>
        </div>
        <div className='min-w-40 flex-1 space-y-1'>
          <span className='text-sm font-medium'>{t('Note')}</span>
          <Textarea
            value={note}
            maxLength={2000}
            disabled={!props.canOperate}
            onChange={(event) => setNote(event.target.value)}
            rows={2}
            aria-label={t('Annotation note')}
          />
        </div>
        <label className='flex items-center gap-2 text-sm'>
          <Switch
            checked={pinned}
            disabled={!props.canOperate}
            onCheckedChange={setPinned}
          />
          {t('Pinned')}
        </label>
        <Button
          size='sm'
          disabled={!props.canOperate || annotateMutation.isPending}
          onClick={() => annotateMutation.mutate()}
        >
          {t('Save annotation')}
        </Button>
      </div>
    </div>
  )

  return (
    <Dialog
      open={props.open}
      onOpenChange={props.onOpenChange}
      title={t('Sample {{id}}', { id: sample.id })}
      contentClassName='sm:max-w-3xl'
      footer={footer}
    >
      <Tabs defaultValue='preview'>
        <TabsList>
          <TabsTrigger value='preview'>{t('Preview')}</TabsTrigger>
          <TabsTrigger value='source'>{t('Source')}</TabsTrigger>
          <TabsTrigger value='metadata'>{t('Metadata')}</TabsTrigger>
        </TabsList>
        <TabsContent value='preview' className='mt-2'>
          {previewContent(sample)}
        </TabsContent>
        <TabsContent value='source' className='mt-2 space-y-2'>
          <div className='flex justify-end'>
            <CopyButton value={source} aria-label={t('Copy source')} />
          </div>
          <pre className='bg-muted max-h-96 overflow-auto rounded-lg p-3 font-mono text-xs whitespace-pre-wrap'>
            {source || '—'}
          </pre>
        </TabsContent>
        <TabsContent value='metadata' className='mt-2'>
          <dl className='divide-border divide-y'>
            <MetaRow label={t('Sample ID')}>{sample.id}</MetaRow>
            <MetaRow label={t('Run ID')}>
              <span className='flex items-center gap-1 font-mono text-xs'>
                <span className='truncate'>{sample.run_id}</span>
                <CopyButton value={sample.run_id} />
              </span>
            </MetaRow>
            {props.showChannel && (
              <MetaRow label={t('Channel')}>
                {sample.channel_id === 0
                  ? t('Unattributed channel')
                  : sample.channel_name || `#${sample.channel_id}`}
              </MetaRow>
            )}
            <MetaRow label={t('Status')}>
              <StatusBadge
                variant={SAMPLE_STATUS_VARIANT[sample.status] ?? 'neutral'}
                label={t(sampleStatusLabel(sample.status))}
                copyable={false}
              />
            </MetaRow>
            <MetaRow label={t('Request ID')}>
              <span className='font-mono text-xs break-all'>
                {sample.request_id || '—'}
              </span>
            </MetaRow>
            <MetaRow label={t('Response model')}>
              <span className='font-mono text-xs'>
                {sample.response_model || '—'}
              </span>
            </MetaRow>
            <MetaRow label={t('Started at')}>
              {formatTimestampToDate(sample.started_at, 'milliseconds')}
            </MetaRow>
            <MetaRow label={t('Finished at')}>
              {formatTimestampToDate(sample.finished_at, 'milliseconds')}
            </MetaRow>
            <MetaRow label={t('Duration')}>
              {formatDurationMs(sample.duration_ms)}
            </MetaRow>
            <MetaRow label={t('First text latency')}>
              {sample.first_text_ms == null
                ? '—'
                : formatDurationMs(sample.first_text_ms)}
            </MetaRow>
            <MetaRow label={t('Input tokens')}>
              {sample.input_tokens == null ? '—' : sample.input_tokens}
            </MetaRow>
            <MetaRow label={t('Output tokens')}>
              {sample.output_tokens == null ? '—' : sample.output_tokens}
            </MetaRow>
            <MetaRow label={t('Error code')}>{errorLabel}</MetaRow>
            <MetaRow label={t('Validation')}>
              {sample.validation || '—'}
            </MetaRow>
            <MetaRow label={t('Finish reason')}>
              {sample.finish_reason || '—'}
            </MetaRow>
            <MetaRow label={t('Request succeeded')}>
              {sample.request_success ? t('Yes') : t('No')}
            </MetaRow>
          </dl>
        </TabsContent>
      </Tabs>
    </Dialog>
  )
}
