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
import { Pencil, Play, Square } from 'lucide-react'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { ConfirmDialog } from '@/components/confirm-dialog'
import { StatusBadge } from '@/components/status-badge'
import { Button } from '@/components/ui/button'
import { handleServerError } from '@/lib/handle-server-error'

import { cancelQualityRun, createQualityRun } from '../api'
import { runSampleTotal, runTargetCount } from '../lib/quality-view'
import type { QualityCaseView } from '../types'

type RunControlsProps = {
  qualityCase: QualityCaseView
  canOperate: boolean
  onEdit: () => void
}

export function RunControls(props: RunControlsProps) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const [startOpen, setStartOpen] = useState(false)
  const [stopOpen, setStopOpen] = useState(false)

  const invalidate = () => {
    void queryClient.invalidateQueries({ queryKey: ['model-quality'] })
  }

  const startMutation = useMutation({
    mutationFn: () =>
      createQualityRun(
        props.qualityCase.id,
        { version: props.qualityCase.version },
        crypto.randomUUID()
      ),
    retry: false,
    onSuccess: (response) => {
      if (!response.success) {
        toast.error(response.message || t('Request failed'))
        return
      }
      toast.success(t('Run queued'))
      setStartOpen(false)
      invalidate()
    },
    onError: (error) => {
      handleServerError(error, t('Failed to start the run'))
    },
  })

  const stopMutation = useMutation({
    mutationFn: () => cancelQualityRun(props.qualityCase.active_run_id),
    retry: false,
    onSuccess: (response) => {
      if (!response.success) {
        toast.error(response.message || t('Request failed'))
        return
      }
      toast.success(t('Run cancellation requested'))
      setStopOpen(false)
      invalidate()
    },
    onError: (error) => {
      handleServerError(error, t('Failed to cancel the run'))
    },
  })

  const running = props.qualityCase.active_run_id !== ''
  const targets = runTargetCount(props.qualityCase.config)
  const count = runSampleTotal(props.qualityCase.config)

  return (
    <div className='flex flex-wrap items-center gap-2'>
      <Button
        size='sm'
        variant='outline'
        onClick={props.onEdit}
        aria-label={t('Edit')}
      >
        <Pencil data-icon='inline-start' />
        {t('Edit')}
      </Button>
      {running ? (
        <>
          <StatusBadge
            variant='info'
            pulse
            copyable={false}
            label={t('Run in progress')}
          />
          <Button
            size='sm'
            variant='outline'
            className='text-destructive'
            disabled={!props.canOperate}
            onClick={() => setStopOpen(true)}
          >
            <Square data-icon='inline-start' />
            {t('Stop')}
          </Button>
        </>
      ) : (
        <Button
          size='sm'
          disabled={!props.canOperate || !props.qualityCase.enabled}
          onClick={() => setStartOpen(true)}
        >
          <Play data-icon='inline-start' />
          {t('Run now')}
        </Button>
      )}
      <ConfirmDialog
        open={startOpen}
        onOpenChange={setStartOpen}
        title={t('Run now')}
        desc={t(
          'This will start {{count}} sample(s) against {{targets}} target(s). Sampling consumes quota.',
          { count, targets }
        )}
        isLoading={startMutation.isPending}
        handleConfirm={() => startMutation.mutate()}
      />
      <ConfirmDialog
        open={stopOpen}
        onOpenChange={setStopOpen}
        title={t('Stop run')}
        desc={t(
          'Pending samples are cancelled; in-flight requests are asked to stop, upstream billing may still occur.'
        )}
        destructive
        isLoading={stopMutation.isPending}
        handleConfirm={() => stopMutation.mutate()}
      />
    </div>
  )
}
