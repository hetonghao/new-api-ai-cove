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
import { RefreshCw } from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { StatusBadge } from '@/components/status-badge'
import { Button } from '@/components/ui/button'
import { Progress } from '@/components/ui/progress'
import { TitledCard } from '@/components/ui/titled-card'
import { toIntlLocale } from '@/i18n/languages'
import { formatBytes, formatNumber, formatTimestampToDate } from '@/lib/format'

import { MIB, type CacheStats } from './api'

export function CacheStatsSummary(props: {
  readonly stats: CacheStats
  readonly updatedAt: number
  readonly refresh: () => void
  readonly fetching: boolean
}) {
  const { t, i18n } = useTranslation()
  const locale = toIntlLocale(i18n.resolvedLanguage || i18n.language)
  const stats = props.stats
  const metrics = [
    { label: t('Entries'), value: stats.entry_count },
    { label: t('Hits'), value: stats.hits },
    { label: t('Misses'), value: stats.misses },
    { label: t('Evictions'), value: stats.evictions },
    { label: t('Expirations'), value: stats.expirations },
    { label: t('Removals'), value: stats.removals },
  ]
  const used =
    stats.used_bytes < MIB
      ? formatBytes(stats.used_bytes, locale)
      : [formatNumber(stats.used_bytes / MIB, locale), 'MiB'].join(' ')
  const max = [formatNumber(stats.max_bytes / MIB, locale), 'MiB'].join(' ')
  const capacity = [used, max].join(' / ')
  const lookups = stats.hits + stats.misses
  const hitRate =
    lookups === 0
      ? '–'
      : new Intl.NumberFormat(locale, {
          style: 'percent',
          maximumFractionDigits: 1,
        }).format(stats.hits / lookups)
  return (
    <TitledCard
      title={t('Cache statistics')}
      description={t('Last updated: {{time}}', {
        time: formatTimestampToDate(props.updatedAt, 'milliseconds'),
      })}
      disableHoverEffect
      action={
        <Button
          type='button'
          variant='outline'
          disabled={props.fetching}
          onClick={props.refresh}
        >
          <RefreshCw aria-hidden='true' />
          {t('Refresh')}
        </Button>
      }
    >
      <div className='space-y-4'>
        <div className='flex flex-wrap items-center gap-x-3 gap-y-2'>
          <span className='font-medium [overflow-wrap:anywhere]'>
            {stats.node_name}
          </span>
          <StatusBadge
            variant={stats.enabled ? 'success' : 'neutral'}
            copyable={false}
          >
            {stats.enabled ? t('Enabled') : t('Disabled')}
          </StatusBadge>
          <span className='text-muted-foreground inline-flex min-w-0 items-baseline gap-1 text-xs'>
            {t('Instance')}
            <code className='min-w-0 truncate' title={stats.instance_id}>
              {stats.instance_id}
            </code>
          </span>
        </div>
        <div className='space-y-2'>
          <div className='flex flex-wrap justify-between gap-2'>
            <span className='text-muted-foreground'>{t('Memory usage')}</span>
            <span className='tabular-nums'>{capacity}</span>
          </div>
          <Progress
            aria-label={t('Memory usage')}
            aria-valuetext={capacity}
            value={
              stats.max_bytes > 0
                ? Math.min(100, (stats.used_bytes / stats.max_bytes) * 100)
                : 0
            }
          />
        </div>
        <dl className='grid grid-cols-2 gap-3 sm:grid-cols-4'>
          {metrics.map((metric) => (
            <div key={metric.label}>
              <dt className='text-muted-foreground'>{metric.label}</dt>
              <dd className='font-medium tabular-nums'>
                {formatNumber(metric.value, locale)}
              </dd>
            </div>
          ))}
          <div>
            <dt className='text-muted-foreground'>{t('Cache hit rate')}</dt>
            <dd className='font-medium tabular-nums'>{hitRate}</dd>
          </div>
          <div>
            <dt className='text-muted-foreground'>
              {t('Cache lifetime (seconds)')}
            </dt>
            <dd className='font-medium tabular-nums'>
              {formatNumber(stats.ttl_seconds, locale)}
            </dd>
          </div>
        </dl>
      </div>
    </TitledCard>
  )
}
