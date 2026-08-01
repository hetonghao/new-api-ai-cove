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
// allow: SIZE_OK -- the three charts share one filter, tooltip, and accessibility contract.
import { useState, type KeyboardEvent } from 'react'
import { useTranslation } from 'react-i18next'
import {
  Bar,
  BarChart,
  CartesianGrid,
  Legend,
  ResponsiveContainer,
  Tooltip,
  XAxis,
  YAxis,
} from 'recharts'

import type { RiskRecordFilters } from '@/features/risk-records/types'
import { formatNumber, formatPercent } from '@/lib/format'

import type {
  RiskStatisticsChannel,
  RiskStatisticsGranularity,
  RiskStatisticsUser,
} from '../types'
import {
  SOURCE_COLORS,
  SOURCE_KEYS,
  sourceLabel,
  type SourceChartRow,
} from './risk-statistics-data'
import {
  AccessibleDataTable,
  ChannelResultTooltip,
  ChartCard,
  ChartPlaceholder,
  SourceTooltip,
  UserResultTooltip,
} from './risk-statistics-shared'

function handleChartKeyDown(
  event: KeyboardEvent<HTMLDivElement>,
  itemCount: number,
  activeIndex: number,
  setActiveIndex: (index: number) => void,
  onNavigate: (index: number) => void
) {
  if (event.key === 'Enter' || event.key === ' ') {
    event.preventDefault()
    onNavigate(Math.min(activeIndex, Math.max(itemCount - 1, 0)))
    return
  }

  let nextIndex = activeIndex
  if (event.key === 'ArrowDown' || event.key === 'ArrowRight') {
    nextIndex = Math.min(activeIndex + 1, Math.max(itemCount - 1, 0))
  } else if (event.key === 'ArrowUp' || event.key === 'ArrowLeft') {
    nextIndex = Math.max(activeIndex - 1, 0)
  } else if (event.key === 'Home') {
    nextIndex = 0
  } else if (event.key === 'End') {
    nextIndex = Math.max(itemCount - 1, 0)
  } else {
    return
  }
  event.preventDefault()
  setActiveIndex(nextIndex)
}

export function UserResultChart(props: {
  readonly users: readonly RiskStatisticsUser[]
  readonly affectedUsers: number
  readonly loading: boolean
  readonly emptyText: string
  readonly recordFilters: RiskRecordFilters
  readonly onNavigateToRecords: (filters: RiskRecordFilters) => void
}) {
  const { t } = useTranslation()
  const [activeIndex, setActiveIndex] = useState(0)
  const rows = props.users.map((user) => ({
    ...user,
    label: user.username || `#${user.user_id}`,
  }))

  return (
    <ChartCard
      title={t('User result distribution')}
      description={t(
        'Top 10 users sorted by unsafe count, then error count, using absolute records.'
      )}
    >
      {props.loading ? (
        <ChartPlaceholder loading text={props.emptyText} />
      ) : null}
      {!props.loading && props.users.length ? (
        <>
          <div className='h-80 min-w-0'>
            <div
              role='button'
              tabIndex={0}
              aria-label={`${t('User result distribution')}: ${rows[activeIndex]?.label ?? ''}`}
              className='focus-visible:ring-ring h-full w-full rounded-sm focus-visible:ring-2 focus-visible:outline-none'
              onKeyDown={(event) =>
                handleChartKeyDown(
                  event,
                  rows.length,
                  activeIndex,
                  setActiveIndex,
                  (index) => {
                    const user = rows[index]
                    if (user) {
                      props.onNavigateToRecords({
                        ...props.recordFilters,
                        user_id: user.user_id,
                      })
                    }
                  }
                )
              }
            >
              <ResponsiveContainer width='100%' height='100%'>
                <BarChart
                  data={rows}
                  layout='vertical'
                  cursor='pointer'
                  onClick={(state) => {
                    const index =
                      typeof state.activeTooltipIndex === 'number'
                        ? state.activeTooltipIndex
                        : null
                    const user = index === null ? undefined : rows[index]
                    if (user) {
                      props.onNavigateToRecords({
                        ...props.recordFilters,
                        user_id: user.user_id,
                      })
                    }
                  }}
                  margin={{ top: 8, right: 12, bottom: 8, left: 8 }}
                >
                  <CartesianGrid strokeDasharray='3 3' horizontal={false} />
                  <XAxis type='number' allowDecimals={false} />
                  <YAxis
                    dataKey='label'
                    type='category'
                    width={128}
                    tick={{ fontSize: 11 }}
                  />
                  <Tooltip
                    content={
                      <UserResultTooltip affectedUsers={props.affectedUsers} />
                    }
                  />
                  <Legend />
                  <Bar
                    dataKey='unsafe'
                    name={t('Unsafe')}
                    stackId='user'
                    fill='var(--destructive)'
                  />
                  <Bar
                    dataKey='errors'
                    name={t('Errors')}
                    stackId='user'
                    fill='var(--warning)'
                  />
                  <Bar
                    dataKey='safe'
                    name={t('Safe')}
                    stackId='user'
                    fill='var(--success)'
                  />
                  <Bar
                    dataKey='not_reviewed'
                    name={t('Not reviewed')}
                    stackId='user'
                    fill='var(--neutral)'
                  />
                </BarChart>
              </ResponsiveContainer>
            </div>
          </div>
          <p className='text-muted-foreground mt-2 text-xs' aria-live='polite'>
            {t('Select')}:
            <span className='text-foreground font-medium'>
              {rows[activeIndex]?.label ?? ''}
            </span>
          </p>
          <AccessibleDataTable
            caption={t('User result distribution')}
            headers={[
              t('User'),
              t('Unsafe'),
              t('Errors'),
              t('Safe'),
              t('Not reviewed'),
              t('Affected users'),
              t('Total'),
            ]}
            rows={rows.map((row) => [
              row.label,
              formatNumber(row.unsafe),
              formatNumber(row.errors),
              formatNumber(row.safe),
              formatNumber(row.not_reviewed),
              formatNumber(props.affectedUsers),
              formatNumber(row.total),
            ])}
          />
        </>
      ) : null}
      {!props.loading && !props.users.length ? (
        <ChartPlaceholder loading={false} text={props.emptyText} />
      ) : null}
    </ChartCard>
  )
}

export function ChannelResultChart(props: {
  readonly channels: readonly RiskStatisticsChannel[]
  readonly loading: boolean
  readonly emptyText: string
  readonly recordFilters: RiskRecordFilters
  readonly onNavigateToRecords: (filters: RiskRecordFilters) => void
}) {
  const { t } = useTranslation()
  const [activeIndex, setActiveIndex] = useState(0)
  const rows = props.channels.map((channel) => ({
    ...channel,
    label: channel.channel_name || `#${channel.channel_id}`,
  }))

  return (
    <ChartCard
      title={t('Channel result distribution')}
      description={t(
        'Channels sorted by unsafe count, then error count, using absolute records.'
      )}
    >
      {props.loading ? (
        <ChartPlaceholder loading text={props.emptyText} />
      ) : null}
      {!props.loading && props.channels.length ? (
        <>
          <div className='h-80 min-w-0'>
            <div
              role='button'
              tabIndex={0}
              aria-label={`${t('Channel result distribution')}: ${rows[activeIndex]?.label ?? ''}`}
              className='focus-visible:ring-ring h-full w-full rounded-sm focus-visible:ring-2 focus-visible:outline-none'
              onKeyDown={(event) =>
                handleChartKeyDown(
                  event,
                  rows.length,
                  activeIndex,
                  setActiveIndex,
                  (index) => {
                    const channel = rows[index]
                    if (channel) {
                      props.onNavigateToRecords({
                        ...props.recordFilters,
                        channel_id: channel.channel_id,
                      })
                    }
                  }
                )
              }
            >
              <ResponsiveContainer width='100%' height='100%'>
                <BarChart
                  data={rows}
                  layout='vertical'
                  cursor='pointer'
                  onClick={(state) => {
                    const index =
                      typeof state.activeTooltipIndex === 'number'
                        ? state.activeTooltipIndex
                        : null
                    const channel = index === null ? undefined : rows[index]
                    if (channel) {
                      props.onNavigateToRecords({
                        ...props.recordFilters,
                        channel_id: channel.channel_id,
                      })
                    }
                  }}
                  margin={{ top: 8, right: 12, bottom: 8, left: 8 }}
                >
                  <CartesianGrid strokeDasharray='3 3' horizontal={false} />
                  <XAxis type='number' allowDecimals={false} />
                  <YAxis
                    dataKey='label'
                    type='category'
                    width={128}
                    tick={{ fontSize: 11 }}
                  />
                  <Tooltip content={<ChannelResultTooltip />} />
                  <Legend />
                  <Bar
                    dataKey='unsafe'
                    name={t('Unsafe')}
                    fill='var(--destructive)'
                  />
                  <Bar
                    dataKey='errors'
                    name={t('Errors')}
                    fill='var(--warning)'
                  />
                  <Bar dataKey='safe' name={t('Safe')} fill='var(--success)' />
                </BarChart>
              </ResponsiveContainer>
            </div>
          </div>
          <p className='text-muted-foreground mt-2 text-xs' aria-live='polite'>
            {t('Select')}:
            <span className='text-foreground font-medium'>
              {rows[activeIndex]?.label ?? ''}
            </span>
          </p>
          <AccessibleDataTable
            caption={t('Channel result distribution')}
            headers={[
              t('Channel'),
              t('Unsafe'),
              t('Errors'),
              t('Safe'),
              t('Total'),
            ]}
            rows={rows.map((row) => [
              row.label,
              formatNumber(row.unsafe),
              formatNumber(row.errors),
              formatNumber(row.safe),
              formatNumber(row.total),
            ])}
          />
        </>
      ) : null}
      {!props.loading && !props.channels.length ? (
        <ChartPlaceholder loading={false} text={props.emptyText} />
      ) : null}
    </ChartCard>
  )
}

export function SourceTrendChart(props: {
  readonly rows: readonly SourceChartRow[]
  readonly loading: boolean
  readonly emptyText: string
  readonly granularity: RiskStatisticsGranularity
  readonly recordFilters: RiskRecordFilters
  readonly onNavigateToRecords: (filters: RiskRecordFilters) => void
}) {
  const { t } = useTranslation()
  const [activeIndex, setActiveIndex] = useState(0)
  const activeRowIndex = Math.floor(activeIndex / SOURCE_KEYS.length)
  const activeSource = SOURCE_KEYS[activeIndex % SOURCE_KEYS.length]
  const activeRow = props.rows[activeRowIndex]
  let bucketSeconds = 7 * 24 * 60 * 60
  if (props.granularity === 'hour') {
    bucketSeconds = 60 * 60
  } else if (props.granularity === 'day') {
    bucketSeconds = 24 * 60 * 60
  }

  return (
    <ChartCard
      title={t('Risk source proportion trend')}
      description={t(
        '100% stacked bars by time; hover to see both absolute counts and percentages.'
      )}
    >
      {props.loading ? (
        <ChartPlaceholder loading text={props.emptyText} />
      ) : null}
      {!props.loading && props.rows.length ? (
        <>
          <div className='h-80 min-w-0'>
            <div
              role='button'
              tabIndex={0}
              aria-label={`${t('Risk source proportion trend')}: ${props.rows[Math.floor(activeIndex / SOURCE_KEYS.length)]?.label ?? ''}`}
              className='focus-visible:ring-ring h-full w-full rounded-sm focus-visible:ring-2 focus-visible:outline-none'
              onKeyDown={(event) =>
                handleChartKeyDown(
                  event,
                  props.rows.length * SOURCE_KEYS.length,
                  activeIndex,
                  setActiveIndex,
                  (index) => {
                    const row =
                      props.rows[Math.floor(index / SOURCE_KEYS.length)]
                    const source = SOURCE_KEYS[index % SOURCE_KEYS.length]
                    if (row && source) {
                      props.onNavigateToRecords({
                        ...props.recordFilters,
                        source,
                        start_timestamp: row.bucket_start,
                        end_timestamp: row.bucket_start + bucketSeconds - 1,
                      })
                    }
                  }
                )
              }
            >
              <ResponsiveContainer width='100%' height='100%'>
                <BarChart
                  data={props.rows}
                  stackOffset='expand'
                  cursor='pointer'
                  onClick={(state) => {
                    const index =
                      typeof state.activeTooltipIndex === 'number'
                        ? state.activeTooltipIndex
                        : null
                    const row = index === null ? undefined : props.rows[index]
                    const source = SOURCE_KEYS.find(
                      (key) => `${key}_pct` === state.activeDataKey
                    )
                    if (row && source) {
                      props.onNavigateToRecords({
                        ...props.recordFilters,
                        source,
                        start_timestamp: row.bucket_start,
                        end_timestamp: row.bucket_start + bucketSeconds - 1,
                      })
                    }
                  }}
                  margin={{ top: 8, right: 12, bottom: 8, left: 8 }}
                >
                  <CartesianGrid strokeDasharray='3 3' />
                  <XAxis
                    dataKey='label'
                    interval='preserveStartEnd'
                    tick={{ fontSize: 11 }}
                  />
                  <YAxis
                    tickFormatter={(value) =>
                      `${Math.round(Number(value) * 100)}%`
                    }
                  />
                  <Tooltip content={<SourceTooltip />} />
                  {SOURCE_KEYS.map((key) => (
                    <Bar
                      key={key}
                      dataKey={`${key}_pct`}
                      stackId='source'
                      fill={SOURCE_COLORS[key]}
                      fillOpacity={0.78}
                      name={sourceLabel(t, key)}
                    />
                  ))}
                </BarChart>
              </ResponsiveContainer>
            </div>
          </div>
          <div className='mt-3 flex flex-wrap gap-x-4 gap-y-1.5 text-xs'>
            {SOURCE_KEYS.map((key) => (
              <span key={key} className='flex items-center gap-1.5'>
                <span
                  className='size-2 rounded-full'
                  style={{ backgroundColor: SOURCE_COLORS[key] }}
                />
                {sourceLabel(t, key)}
              </span>
            ))}
          </div>
          <p className='text-muted-foreground mt-2 text-xs' aria-live='polite'>
            {t('Select')}:
            <span className='text-foreground font-medium'>
              {activeRow?.label ?? ''}
              {activeRow && activeSource
                ? ` · ${sourceLabel(t, activeSource)}`
                : ''}
            </span>
          </p>
          <AccessibleDataTable
            caption={t('Risk source proportion trend')}
            headers={[
              t('Time'),
              ...SOURCE_KEYS.map((key) => sourceLabel(t, key)),
              t('Total'),
            ]}
            rows={props.rows.map((row) => [
              row.label,
              `${formatNumber(row.provider)} (${formatPercent(row.provider_pct * 100)})`,
              `${formatNumber(row.cache)} (${formatPercent(row.cache_pct * 100)})`,
              `${formatNumber(row.inflight)} (${formatPercent(row.inflight_pct * 100)})`,
              `${formatNumber(row.local)} (${formatPercent(row.local_pct * 100)})`,
              formatNumber(row.total),
            ])}
          />
        </>
      ) : null}
      {!props.loading && !props.rows.length ? (
        <ChartPlaceholder loading={false} text={props.emptyText} />
      ) : null}
    </ChartCard>
  )
}
