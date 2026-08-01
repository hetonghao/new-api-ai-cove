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
*/
import { useQuery } from '@tanstack/react-query'
import {
  Activity,
  Ban,
  BarChart3,
  CircleAlert,
  Clock3,
  Database,
  Loader2,
  ShieldAlert,
  Users,
} from 'lucide-react'
import { useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { ErrorState } from '@/components/error-state'
import { Tabs, TabsList, TabsTrigger } from '@/components/ui/tabs'
import {
  TIME_GRANULARITY_OPTIONS,
  TIME_RANGE_PRESETS,
} from '@/features/dashboard/constants'
import type { RiskRecordFilters } from '@/features/risk-records/types'
import { formatNumber, formatPercent } from '@/lib/format'
import { getRollingDateRange } from '@/lib/time'

import { getRiskStatistics } from '../api'
import type { RiskStatisticsGranularity } from '../types'
import {
  ChannelResultChart,
  SourceTrendChart,
  UserResultChart,
} from './risk-statistics-charts'
import { toSourceChartRows } from './risk-statistics-data'
import { MetricCard } from './risk-statistics-shared'

function isRiskStatisticsGranularity(
  value: string
): value is RiskStatisticsGranularity {
  return value === 'hour' || value === 'day' || value === 'week'
}

export function RiskStatistics(props: {
  readonly onNavigateToRecords: (filters: RiskRecordFilters) => void
}) {
  const { t } = useTranslation()
  const [rangeDays, setRangeDays] = useState(1)
  const [granularity, setGranularity] =
    useState<RiskStatisticsGranularity>('hour')
  const queryParams = useMemo(() => {
    const { start, end } = getRollingDateRange(rangeDays)
    return {
      start_timestamp: Math.floor(start.getTime() / 1000),
      end_timestamp: Math.floor(end.getTime() / 1000),
      granularity,
    }
  }, [granularity, rangeDays])
  const recordFilters: RiskRecordFilters = {
    start_timestamp: queryParams.start_timestamp,
    end_timestamp: queryParams.end_timestamp,
  }
  const statisticsQuery = useQuery({
    queryKey: ['risk', 'statistics', queryParams],
    queryFn: async () => {
      const response = await getRiskStatistics(queryParams)
      if (!response.success || !response.data) {
        throw new Error(response.message || t('Failed to load risk statistics'))
      }
      return response.data
    },
    retry: false,
    staleTime: 60_000,
  })

  const statistics = statisticsQuery.data
  const sourceRows = useMemo(
    () => toSourceChartRows(statistics?.source_trend ?? [], granularity),
    [granularity, statistics?.source_trend]
  )

  function changeGranularity(value: RiskStatisticsGranularity) {
    setGranularity(value)
    if (value === 'hour') {
      setRangeDays(1)
      return
    }
    if (value === 'day') {
      setRangeDays(7)
      return
    }
    setRangeDays(29)
  }

  const emptyText = t('No risk statistics available')
  const summary = statistics?.summary

  return (
    <div className='space-y-3'>
      <div className='flex flex-wrap items-center gap-2'>
        <Tabs
          value={String(rangeDays)}
          onValueChange={(value) => setRangeDays(Number(value))}
        >
          <TabsList>
            {TIME_RANGE_PRESETS.map((preset) => (
              <TabsTrigger
                key={preset.days}
                value={String(preset.days)}
                className='px-2.5 text-xs'
              >
                {t(preset.label)}
              </TabsTrigger>
            ))}
          </TabsList>
        </Tabs>
        <Tabs
          value={granularity}
          onValueChange={(value) => {
            if (isRiskStatisticsGranularity(value)) changeGranularity(value)
          }}
        >
          <TabsList>
            {TIME_GRANULARITY_OPTIONS.map((option) => (
              <TabsTrigger
                key={option.value}
                value={option.value}
                className='px-2.5 text-xs'
              >
                {t(option.label)}
              </TabsTrigger>
            ))}
          </TabsList>
        </Tabs>
        {statisticsQuery.isFetching ? (
          <Loader2 className='text-muted-foreground size-4 animate-spin' />
        ) : null}
      </div>

      {statisticsQuery.error ? (
        <ErrorState
          title={t('Failed to load risk statistics')}
          description={
            statisticsQuery.error instanceof Error
              ? statisticsQuery.error.message
              : t('Request failed')
          }
          onRetry={() => void statisticsQuery.refetch()}
        />
      ) : (
        <>
          <div className='grid gap-3 sm:grid-cols-2 lg:grid-cols-3 xl:grid-cols-6'>
            <MetricCard
              label={t('Risk records')}
              value={formatNumber(summary?.records ?? 0)}
              icon={Database}
            />
            <MetricCard
              label={t('Affected users')}
              value={formatNumber(summary?.affected_users ?? 0)}
              icon={Users}
            />
            <MetricCard
              label={t('Unsafe')}
              value={formatNumber(summary?.unsafe ?? 0)}
              detail={formatPercent(summary?.unsafe_rate ?? 0)}
              icon={ShieldAlert}
            />
            <MetricCard
              label={t('Blocked')}
              value={formatNumber(summary?.blocked ?? 0)}
              detail={formatPercent(summary?.blocked_rate ?? 0)}
              icon={Ban}
            />
            <MetricCard
              label={t('Errors')}
              value={formatNumber(summary?.errors ?? 0)}
              detail={formatPercent(summary?.error_rate ?? 0)}
              icon={CircleAlert}
            />
            <MetricCard
              label={t('Cache hit rate')}
              value={formatPercent(summary?.cache_hit_rate ?? 0)}
              detail={`${formatNumber(summary?.cache_hits ?? 0)} ${t('hits')}`}
              icon={Activity}
            />
          </div>

          <div className='grid gap-3 sm:grid-cols-3'>
            <MetricCard
              label={t('Provider calls')}
              value={formatNumber(summary?.provider_calls ?? 0)}
              icon={BarChart3}
            />
            <MetricCard
              label={t('Neurons')}
              value={formatNumber(summary?.neurons ?? 0)}
              icon={Activity}
            />
            <MetricCard
              label={t('P95 review latency')}
              value={`${formatNumber(summary?.p95_latency_ms ?? 0)} ms`}
              icon={Clock3}
            />
          </div>

          <div className='grid gap-3 xl:grid-cols-2'>
            <UserResultChart
              users={statistics?.users ?? []}
              affectedUsers={summary?.affected_users ?? 0}
              loading={statisticsQuery.isLoading}
              emptyText={emptyText}
              recordFilters={recordFilters}
              onNavigateToRecords={props.onNavigateToRecords}
            />
            <ChannelResultChart
              channels={statistics?.channels ?? []}
              loading={statisticsQuery.isLoading}
              emptyText={emptyText}
              recordFilters={recordFilters}
              onNavigateToRecords={props.onNavigateToRecords}
            />
          </div>

          <SourceTrendChart
            rows={sourceRows}
            loading={statisticsQuery.isLoading}
            emptyText={emptyText}
            granularity={granularity}
            recordFilters={recordFilters}
            onNavigateToRecords={props.onNavigateToRecords}
          />
        </>
      )}
    </div>
  )
}
