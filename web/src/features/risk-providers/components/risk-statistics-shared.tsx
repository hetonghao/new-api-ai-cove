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
import type { TFunction } from 'i18next'
import type { ComponentType, ReactNode } from 'react'
import type { TooltipContentProps } from 'recharts'

import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Skeleton } from '@/components/ui/skeleton'
import { formatNumber, formatPercent } from '@/lib/format'

import type { RiskStatisticsChannel, RiskStatisticsUser } from '../types'
import {
  SOURCE_COLORS,
  SOURCE_KEYS,
  sourceLabel,
  type SourceChartRow,
} from './risk-statistics-data'

type UserChartRow = RiskStatisticsUser & { readonly label: string }
type ChannelChartRow = RiskStatisticsChannel & { readonly label: string }

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === 'object' && value !== null
}

function hasNumericFields(
  value: Record<string, unknown>,
  keys: readonly string[]
) {
  return keys.every((key) => typeof value[key] === 'number')
}

function isUserChartRow(value: unknown): value is UserChartRow {
  return (
    isRecord(value) &&
    typeof value.label === 'string' &&
    typeof value.username === 'string' &&
    hasNumericFields(value, [
      'user_id',
      'safe',
      'unsafe',
      'errors',
      'not_reviewed',
      'total',
    ])
  )
}

function isChannelChartRow(value: unknown): value is ChannelChartRow {
  return (
    isRecord(value) &&
    typeof value.label === 'string' &&
    typeof value.channel_name === 'string' &&
    hasNumericFields(value, ['channel_id', 'safe', 'unsafe', 'errors', 'total'])
  )
}

function isSourceChartRow(value: unknown): value is SourceChartRow {
  return (
    isRecord(value) &&
    typeof value.label === 'string' &&
    typeof value.bucket_start === 'number' &&
    typeof value.total === 'number' &&
    SOURCE_KEYS.every(
      (key) =>
        typeof value[key] === 'number' &&
        typeof value[`${key}_pct`] === 'number'
    )
  )
}

function TooltipBox(props: {
  readonly title: string
  readonly children: ReactNode
}) {
  return (
    <div className='bg-popover text-popover-foreground min-w-48 rounded-lg border p-3 text-xs shadow-lg'>
      <div className='mb-2 font-medium'>{props.title}</div>
      {props.children}
    </div>
  )
}

function TooltipValueRow(props: {
  readonly label: string
  readonly value: string
}) {
  return (
    <div className='flex items-center justify-between gap-4 py-0.5'>
      <span>{props.label}</span>
      <span className='tabular-nums'>{props.value}</span>
    </div>
  )
}

export function UserResultTooltip(
  props: Partial<TooltipContentProps<number, string>> & {
    readonly translate: TFunction
    readonly affectedUsers: number
  }
) {
  if (!props.active || !props.payload?.length) return null
  const point = props.payload[0]?.payload
  if (!isUserChartRow(point)) return null

  return (
    <TooltipBox title={point.label}>
      <TooltipValueRow
        label={props.translate('Unsafe')}
        value={formatNumber(point.unsafe)}
      />
      <TooltipValueRow
        label={props.translate('Errors')}
        value={formatNumber(point.errors)}
      />
      <TooltipValueRow
        label={props.translate('Safe')}
        value={formatNumber(point.safe)}
      />
      <TooltipValueRow
        label={props.translate('Not reviewed')}
        value={formatNumber(point.not_reviewed)}
      />
      <TooltipValueRow
        label={props.translate('Affected users')}
        value={formatNumber(props.affectedUsers)}
      />
      <div className='mt-2 border-t pt-2'>
        <TooltipValueRow
          label={props.translate('Total')}
          value={formatNumber(point.total)}
        />
      </div>
    </TooltipBox>
  )
}

export function ChannelResultTooltip(
  props: Partial<TooltipContentProps<number, string>> & {
    readonly translate: TFunction
  }
) {
  if (!props.active || !props.payload?.length) return null
  const point = props.payload[0]?.payload
  if (!isChannelChartRow(point)) return null

  return (
    <TooltipBox title={point.label}>
      <TooltipValueRow
        label={props.translate('Unsafe')}
        value={formatNumber(point.unsafe)}
      />
      <TooltipValueRow
        label={props.translate('Errors')}
        value={formatNumber(point.errors)}
      />
      <TooltipValueRow
        label={props.translate('Safe')}
        value={formatNumber(point.safe)}
      />
      <div className='mt-2 border-t pt-2'>
        <TooltipValueRow
          label={props.translate('Total')}
          value={formatNumber(point.total)}
        />
      </div>
    </TooltipBox>
  )
}

export function SourceTooltip(
  props: Partial<TooltipContentProps<number, string>> & {
    readonly translate: TFunction
  }
) {
  if (!props.active || !props.payload?.length) return null
  const point = props.payload[0]?.payload
  if (!isSourceChartRow(point)) return null

  return (
    <TooltipBox title={point.label}>
      {SOURCE_KEYS.map((key) => (
        <div
          key={key}
          className='flex items-center justify-between gap-4 py-0.5'
        >
          <span className='flex items-center gap-1.5'>
            <span
              className='size-2 rounded-full'
              style={{ backgroundColor: SOURCE_COLORS[key] }}
            />
            {sourceLabel(props.translate, key)}
          </span>
          <span className='tabular-nums'>
            {formatNumber(point[key])} ·{' '}
            {formatPercent(point[`${key}_pct`] * 100)}
          </span>
        </div>
      ))}
      <div className='mt-2 border-t pt-2 font-medium tabular-nums'>
        {props.translate('Total')}: {formatNumber(point.total)}
      </div>
    </TooltipBox>
  )
}

export function AccessibleDataTable(props: {
  readonly caption: string
  readonly headers: readonly string[]
  readonly rows: readonly (readonly string[])[]
}) {
  return (
    <table className='sr-only'>
      <caption>{props.caption}</caption>
      <thead>
        <tr>
          {props.headers.map((header) => (
            <th key={header} scope='col'>
              {header}
            </th>
          ))}
        </tr>
      </thead>
      <tbody>
        {props.rows.map((row) => (
          <tr key={row.join('\u0000')}>
            {row.map((cell, cellIndex) => (
              <td key={`${props.headers[cellIndex]}-${cell}`}>{cell}</td>
            ))}
          </tr>
        ))}
      </tbody>
    </table>
  )
}

export function MetricCard(props: {
  readonly label: string
  readonly value: string
  readonly detail?: string
  readonly icon: ComponentType<{ className?: string }>
}) {
  const Icon = props.icon
  return (
    <Card className='gap-2 py-3'>
      <CardHeader className='flex flex-row items-center justify-between gap-2 px-4 pb-0'>
        <CardTitle className='text-muted-foreground text-xs font-medium'>
          {props.label}
        </CardTitle>
        <Icon className='text-muted-foreground size-4' />
      </CardHeader>
      <CardContent className='px-4'>
        <div className='text-xl font-semibold tabular-nums'>{props.value}</div>
        {props.detail ? (
          <div className='text-muted-foreground mt-0.5 text-xs tabular-nums'>
            {props.detail}
          </div>
        ) : null}
      </CardContent>
    </Card>
  )
}

export function ChartCard(props: {
  readonly title: string
  readonly description: string
  readonly children: ReactNode
}) {
  return (
    <Card className='gap-0 overflow-hidden py-0'>
      <CardHeader className='border-b px-4 py-3'>
        <CardTitle className='text-sm'>{props.title}</CardTitle>
        <p className='text-muted-foreground text-xs text-pretty'>
          {props.description}
        </p>
      </CardHeader>
      <CardContent className='p-3 sm:p-4'>{props.children}</CardContent>
    </Card>
  )
}

export function ChartPlaceholder(props: {
  readonly loading: boolean
  readonly text: string
}) {
  if (props.loading) return <Skeleton className='h-72 w-full' />
  return (
    <div className='text-muted-foreground flex h-72 items-center justify-center rounded-lg border text-sm'>
      {props.text}
    </div>
  )
}
