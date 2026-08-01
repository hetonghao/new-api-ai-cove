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
import type { TFunction } from 'i18next'
import {
  Area,
  AreaChart,
  Bar,
  BarChart,
  CartesianGrid,
  Legend,
  ResponsiveContainer,
  Tooltip,
  XAxis,
  YAxis,
} from 'recharts'

import { formatNumber, formatPercent } from '@/lib/format'

import type { RiskStatisticsChannel, RiskStatisticsUser } from '../types'
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

export function UserResultChart(props: {
  readonly users: readonly RiskStatisticsUser[]
  readonly loading: boolean
  readonly emptyText: string
  readonly translate: TFunction
}) {
  const rows = props.users.map((user) => ({
    ...user,
    label: user.username || `#${user.user_id}`,
  }))

  return (
    <ChartCard
      title={props.translate('User result distribution')}
      description={props.translate(
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
              role='img'
              aria-label={props.translate('User result distribution')}
              className='h-full w-full'
            >
              <ResponsiveContainer width='100%' height='100%'>
                <BarChart
                  data={rows}
                  layout='vertical'
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
                    content={<UserResultTooltip translate={props.translate} />}
                  />
                  <Legend />
                  <Bar
                    dataKey='unsafe'
                    name={props.translate('Unsafe')}
                    stackId='user'
                    fill='var(--destructive)'
                  />
                  <Bar
                    dataKey='errors'
                    name={props.translate('Errors')}
                    stackId='user'
                    fill='var(--warning)'
                  />
                  <Bar
                    dataKey='safe'
                    name={props.translate('Safe')}
                    stackId='user'
                    fill='var(--success)'
                  />
                  <Bar
                    dataKey='not_reviewed'
                    name={props.translate('Not reviewed')}
                    stackId='user'
                    fill='var(--neutral)'
                  />
                </BarChart>
              </ResponsiveContainer>
            </div>
          </div>
          <AccessibleDataTable
            caption={props.translate('User result distribution')}
            headers={[
              props.translate('User'),
              props.translate('Unsafe'),
              props.translate('Errors'),
              props.translate('Safe'),
              props.translate('Not reviewed'),
              props.translate('Affected users'),
              props.translate('Total'),
            ]}
            rows={rows.map((row) => [
              row.label,
              formatNumber(row.unsafe),
              formatNumber(row.errors),
              formatNumber(row.safe),
              formatNumber(row.not_reviewed),
              formatNumber(1),
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
  readonly translate: TFunction
}) {
  const rows = props.channels.map((channel) => ({
    ...channel,
    label: channel.channel_name || `#${channel.channel_id}`,
  }))

  return (
    <ChartCard
      title={props.translate('Channel result distribution')}
      description={props.translate(
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
              role='img'
              aria-label={props.translate('Channel result distribution')}
              className='h-full w-full'
            >
              <ResponsiveContainer width='100%' height='100%'>
                <BarChart
                  data={rows}
                  layout='vertical'
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
                      <ChannelResultTooltip translate={props.translate} />
                    }
                  />
                  <Legend />
                  <Bar
                    dataKey='unsafe'
                    name={props.translate('Unsafe')}
                    fill='var(--destructive)'
                  />
                  <Bar
                    dataKey='errors'
                    name={props.translate('Errors')}
                    fill='var(--warning)'
                  />
                  <Bar
                    dataKey='safe'
                    name={props.translate('Safe')}
                    fill='var(--success)'
                  />
                </BarChart>
              </ResponsiveContainer>
            </div>
          </div>
          <AccessibleDataTable
            caption={props.translate('Channel result distribution')}
            headers={[
              props.translate('Channel'),
              props.translate('Unsafe'),
              props.translate('Errors'),
              props.translate('Safe'),
              props.translate('Total'),
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
  readonly translate: TFunction
}) {
  return (
    <ChartCard
      title={props.translate('Risk source proportion trend')}
      description={props.translate(
        '100% stacked area by time; hover to see both absolute counts and percentages.'
      )}
    >
      {props.loading ? (
        <ChartPlaceholder loading text={props.emptyText} />
      ) : null}
      {!props.loading && props.rows.length ? (
        <>
          <div className='h-80 min-w-0'>
            <div
              role='img'
              aria-label={props.translate('Risk source proportion trend')}
              className='h-full w-full'
            >
              <ResponsiveContainer width='100%' height='100%'>
                <AreaChart
                  data={props.rows}
                  stackOffset='expand'
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
                  <Tooltip
                    content={<SourceTooltip translate={props.translate} />}
                  />
                  {SOURCE_KEYS.map((key) => (
                    <Area
                      key={key}
                      type='monotone'
                      dataKey={`${key}_pct`}
                      stackId='source'
                      stroke={SOURCE_COLORS[key]}
                      fill={SOURCE_COLORS[key]}
                      fillOpacity={0.78}
                      name={sourceLabel(props.translate, key)}
                    />
                  ))}
                </AreaChart>
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
                {sourceLabel(props.translate, key)}
              </span>
            ))}
          </div>
          <AccessibleDataTable
            caption={props.translate('Risk source proportion trend')}
            headers={[
              props.translate('Time'),
              ...SOURCE_KEYS.map((key) => sourceLabel(props.translate, key)),
              props.translate('Total'),
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
