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
import { CircleAlert, Zap } from 'lucide-react'
import { useTranslation } from 'react-i18next'

import {
  dotColorMap,
  textColorMap,
  type StatusVariant,
} from '@/components/status-badge'
import {
  Tooltip,
  TooltipContent,
  TooltipProvider,
  TooltipTrigger,
} from '@/components/ui/tooltip'
import { formatUseTime } from '@/lib/format'
import { cn } from '@/lib/utils'

import { getFirstResponseTimeColor, getResponseTimeColor } from '../lib/format'
import type { LogOtherData } from '../types'

/**
 * Softened fills for the full-height timing bar. The bar sits directly beside
 * dense numeric text, so the saturated `dotColorMap` tones (tuned for small
 * dots and badges) read as too high-contrast at that size; a translucent fill
 * keeps the status legible while matching the page's muted palette.
 */
const barColorMap: Record<StatusVariant, string> = {
  ...dotColorMap,
  success: 'bg-success/90',
  warning: 'bg-warning/80',
  danger: 'bg-destructive/80',
  neutral: 'bg-neutral/80',
}

interface TimingMetricsCellProps {
  useTimeSec: number
  completionTokens: number
  frtMs?: number
  isStream: boolean
  className?: string
  /**
   * `bar` (default) draws a full-height color segment beside the labels,
   * matching the dense desktop table. `dot` swaps that segment for small
   * status dots inline with each label, matching the lighter-weight status
   * indicator used elsewhere on the mobile card.
   */
  indicator?: 'bar' | 'dot'
}

export function TimingMetricsCell(props: TimingMetricsCellProps) {
  const { t } = useTranslation()
  const indicator = props.indicator ?? 'bar'
  const showFirstToken = props.isStream
  const firstTokenSeconds =
    props.frtMs != null && props.frtMs > 0 ? props.frtMs / 1000 : null
  const firstTokenVariant: StatusVariant =
    firstTokenSeconds == null
      ? 'neutral'
      : getFirstResponseTimeColor(firstTokenSeconds)
  const totalTimeVariant = getResponseTimeColor(
    props.useTimeSec,
    props.completionTokens
  )
  const firstTokenLabel =
    firstTokenSeconds == null ? t('N/A') : formatUseTime(firstTokenSeconds)
  const totalTimeLabel = formatUseTime(props.useTimeSec)

  const labels = (
    <div className='flex min-h-8 min-w-0 flex-col justify-center gap-0.5 text-xs leading-tight'>
      {showFirstToken && (
        <div className='flex items-baseline gap-1.5'>
          {indicator === 'dot' && (
            <span
              aria-hidden
              className={cn(
                'size-1.5 shrink-0 rounded-full',
                dotColorMap[firstTokenVariant]
              )}
            />
          )}
          <span className='text-muted-foreground shrink-0'>
            {t('First token')}
          </span>
          <span className={cn('tabular-nums', textColorMap[firstTokenVariant])}>
            {firstTokenLabel}
          </span>
        </div>
      )}
      <div className='flex items-baseline gap-1.5'>
        {indicator === 'dot' && (
          <span
            aria-hidden
            className={cn(
              'size-1.5 shrink-0 rounded-full',
              dotColorMap[totalTimeVariant]
            )}
          />
        )}
        <span className='text-muted-foreground shrink-0'>{t('Duration')}</span>
        <span className={cn('tabular-nums', textColorMap[totalTimeVariant])}>
          {totalTimeLabel}
        </span>
      </div>
    </div>
  )

  if (indicator === 'dot') {
    return (
      <div className={cn('flex items-stretch', props.className)}>{labels}</div>
    )
  }

  return (
    <div className={cn('flex items-stretch gap-2', props.className)}>
      <span
        aria-hidden
        className={cn(
          'flex w-1 shrink-0 flex-col overflow-hidden rounded-full',
          !showFirstToken && barColorMap[totalTimeVariant]
        )}
      >
        {showFirstToken && (
          <>
            <span className={cn('flex-1', barColorMap[firstTokenVariant])} />
            <span className={cn('flex-1', barColorMap[totalTimeVariant])} />
          </>
        )}
      </span>
      {labels}
    </div>
  )
}

interface StreamTpsCellProps {
  isStream: boolean
  isWebSocket?: boolean
  isTurbo?: boolean
  tokensPerSecond?: number | null
  streamStatus?: LogOtherData['stream_status']
  className?: string
}

export function StreamTpsCell(props: StreamTpsCellProps) {
  const { t } = useTranslation()
  const showStreamError =
    props.isStream && props.streamStatus && props.streamStatus.status !== 'ok'
  const tpsLabel =
    props.tokensPerSecond != null
      ? `${Math.round(props.tokensPerSecond)} t/s`
      : '—'
  const streamLabel = props.isStream ? t('Stream') : t('Non-stream')
  const webSocketMarker = props.isWebSocket ? (
    <TooltipProvider delay={150}>
      <Tooltip>
        <TooltipTrigger
          render={
            <button
              type='button'
              className='border-info/25 bg-info/10 text-info hover:bg-info/15 focus-visible:ring-info/50 inline-flex h-4 cursor-default items-center rounded border px-1 font-mono text-[9px] font-semibold tracking-wide transition-colors focus-visible:ring-2 focus-visible:outline-none'
              aria-label={t('WebSocket acceleration channel')}
            >
              WS
            </button>
          }
        />
        <TooltipContent>{t('WebSocket acceleration channel')}</TooltipContent>
      </Tooltip>
    </TooltipProvider>
  ) : null
  const turboMarker = props.isTurbo ? (
    <TooltipProvider delay={150}>
      <Tooltip>
        <TooltipTrigger
          render={
            <button
              type='button'
              className='text-success hover:bg-success/15 focus-visible:ring-success/50 inline-flex size-4 cursor-default items-center justify-center rounded-full transition-transform duration-200 hover:scale-110 focus-visible:ring-2 focus-visible:outline-none'
              aria-label={t('From Turbo')}
            >
              <Zap className='fill-success/20 size-3' aria-hidden='true' />
            </button>
          }
        />
        <TooltipContent>{t('From Turbo')}</TooltipContent>
      </Tooltip>
    </TooltipProvider>
  ) : null

  return (
    <div
      className={cn(
        'flex shrink-0 flex-col items-start justify-center gap-0.5 text-xs leading-tight',
        props.className
      )}
    >
      <span
        className={cn(
          'inline-flex items-center gap-1 font-medium',
          props.isStream ? 'text-info' : 'text-muted-foreground'
        )}
      >
        {streamLabel}
        {webSocketMarker}
        {turboMarker}
        {showStreamError && (
          <TooltipProvider>
            <Tooltip>
              <TooltipTrigger
                render={<CircleAlert className='text-destructive size-3' />}
              />
              <TooltipContent>
                <div className='space-y-0.5 text-xs'>
                  <p>
                    {t('Stream Status')}: {t('Error')}
                  </p>
                  <p>{props.streamStatus?.end_reason || 'unknown'}</p>
                  {(props.streamStatus?.error_count ?? 0) > 0 && (
                    <p>
                      {t('Soft Errors')}: {props.streamStatus?.error_count}
                    </p>
                  )}
                </div>
              </TooltipContent>
            </Tooltip>
          </TooltipProvider>
        )}
      </span>
      <span className='text-muted-foreground/60 px-0.5 tabular-nums'>
        {tpsLabel}
      </span>
    </div>
  )
}
