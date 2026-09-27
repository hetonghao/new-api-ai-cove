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
import { useState, type CSSProperties, type ReactNode } from 'react'
import { useTranslation } from 'react-i18next'

import {
  createTooltipHandle,
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from '@/components/ui/tooltip'
import { cn } from '@/lib/utils'

import { bucketTone, formatTimeRange } from '../lib/quality-view'
import type { QualityBucket } from '../types'

type TimelineStripProps = {
  buckets: readonly QualityBucket[]
  selectedBucket: QualityBucket | null
  onSelect: (bucket: QualityBucket | null) => void
  live?: boolean
  legendExtra?: ReactNode
}

const toneClass: Record<string, string> = {
  failure: 'bg-destructive',
  mostly_failure: 'bg-warning',
  mostly_success: 'bg-info',
  success: 'bg-success',
  empty: 'bg-neutral/35',
}

const legendItems: { tone: string; labelKey: string }[] = [
  { tone: 'success', labelKey: 'Success only' },
  { tone: 'mostly_success', labelKey: 'Mostly success' },
  { tone: 'mostly_failure', labelKey: 'Mostly failure' },
  { tone: 'failure', labelKey: 'Failure only' },
  { tone: 'empty', labelKey: 'No samples' },
]

export function TimelineStrip(props: TimelineStripProps) {
  const { t } = useTranslation()
  const [bucketTooltip] = useState(() => createTooltipHandle<number>())
  const newestEnd = props.buckets.at(-1)?.end ?? 0

  return (
    <div className='space-y-1.5'>
      <div
        className='flex h-10 items-end gap-px'
        role='group'
        aria-label={t('Last 24 hours results')}
      >
        {props.buckets.map((bucket, index) => {
          const tone = bucketTone(bucket)
          const selected = props.selectedBucket?.start === bucket.start
          const live = props.live && index === props.buckets.length - 1
          return (
            <TooltipTrigger
              key={newestEnd - bucket.end}
              handle={bucketTooltip}
              payload={index}
              delay={0}
              closeOnClick={false}
              type='button'
              data-tone={tone}
              data-mq-bar=''
              data-mq-live={live ? '' : undefined}
              style={{ '--mq-bar-index': index } as CSSProperties}
              aria-label={t(
                '{{range}}: {{success}} succeeded, {{failure}} failed',
                {
                  range: formatTimeRange(bucket.start, bucket.end),
                  success: bucket.success,
                  failure: bucket.failure,
                }
              )}
              aria-pressed={selected}
              onClick={() => props.onSelect(selected ? null : bucket)}
              className={cn(
                'focus-visible:ring-ring h-full min-w-0 flex-1 cursor-pointer rounded-sm transition-[opacity,filter] duration-100 focus-visible:ring-2 focus-visible:outline-none',
                'data-popup-open:brightness-90 dark:data-popup-open:brightness-125',
                toneClass[tone],
                props.selectedBucket && !selected && 'opacity-60',
                selected && 'ring-ring ring-2 ring-inset'
              )}
            />
          )
        })}
      </div>
      <Tooltip handle={bucketTooltip}>
        {({ payload }) => {
          const bucket =
            payload === undefined ? undefined : props.buckets[payload]
          if (!bucket) return null
          const tone = bucketTone(bucket)
          const selected = props.selectedBucket?.start === bucket.start
          return (
            <TooltipContent className='flex-col items-start gap-0.5 tabular-nums'>
              <span className='font-medium'>
                {formatTimeRange(bucket.start, bucket.end)}
              </span>
              <span className='flex items-center gap-1.5'>
                <span
                  aria-hidden='true'
                  className={cn('size-2 rounded-sm', toneClass[tone])}
                />
                {tone === 'empty'
                  ? t('No samples')
                  : t('{{success}} succeeded, {{failed}} failed', {
                      success: bucket.success,
                      failed: bucket.failure,
                    })}
              </span>
              <span className='text-background/70'>
                {selected
                  ? t('Click to clear filter')
                  : t('Click to filter samples')}
              </span>
            </TooltipContent>
          )
        }}
      </Tooltip>
      <div className='text-muted-foreground flex justify-between text-xs'>
        <span>{t('24h ago')}</span>
        <span>{t('12h ago')}</span>
        <span>{t('Now')}</span>
      </div>
      <div className='text-muted-foreground flex flex-wrap items-center gap-x-3 gap-y-1 text-xs'>
        {legendItems.map((item) => (
          <span key={item.tone} className='flex items-center gap-1'>
            <span
              aria-hidden='true'
              className={cn(
                'inline-block size-2 rounded-sm',
                toneClass[item.tone]
              )}
            />
            {t(item.labelKey)}
          </span>
        ))}
        {props.legendExtra && (
          <span className='ml-auto flex items-center gap-1'>
            {props.legendExtra}
          </span>
        )}
      </div>
    </div>
  )
}
