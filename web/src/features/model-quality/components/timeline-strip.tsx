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
import { useTranslation } from 'react-i18next'

import { formatTimestampToDate } from '@/lib/format'
import { cn } from '@/lib/utils'

import { bucketTone } from '../lib/quality-view'
import type { QualityBucket } from '../types'

type TimelineStripProps = {
  buckets: readonly QualityBucket[]
  selectedBucket: QualityBucket | null
  onSelect: (bucket: QualityBucket | null) => void
}

const toneClass: Record<string, string> = {
  failure: 'bg-destructive',
  mostly_failure: 'bg-warning',
  mostly_success: 'bg-info',
  success: 'bg-success',
  empty: 'bg-muted',
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

  return (
    <div className='space-y-1.5'>
      <div
        className='flex h-10 items-end gap-px'
        role='group'
        aria-label={t('Last 24 hours results')}
      >
        {props.buckets.map((bucket) => {
          const tone = bucketTone(bucket)
          const label = t(
            '{{range}}: {{success}} succeeded, {{failure}} failed',
            {
              range: `${formatTimestampToDate(bucket.start, 'milliseconds')} – ${formatTimestampToDate(bucket.end, 'milliseconds')}`,
              success: bucket.success,
              failure: bucket.failure,
            }
          )
          const selected = props.selectedBucket?.start === bucket.start
          return (
            <button
              key={bucket.start}
              type='button'
              data-tone={tone}
              aria-label={label}
              title={label}
              aria-pressed={selected}
              onClick={() => props.onSelect(selected ? null : bucket)}
              className={cn(
                'focus-visible:ring-ring h-full min-w-0 flex-1 rounded-sm transition-opacity focus-visible:ring-2 focus-visible:outline-none',
                toneClass[tone],
                props.selectedBucket && !selected && 'opacity-75',
                selected && 'ring-ring ring-2 ring-inset'
              )}
            />
          )
        })}
      </div>
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
              className={cn('inline-block size-2 rounded-sm', toneClass[item.tone])}
            />
            {t(item.labelKey)}
          </span>
        ))}
      </div>
    </div>
  )
}
