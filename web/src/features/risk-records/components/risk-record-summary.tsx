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

import { StatusBadge } from '@/components/status-badge'

import {
  getRiskRecordResultLabel,
  getRiskRecordResultVariant,
  getRiskRecordCategoryLabel,
  getRiskRecordSourceLabel,
  getRiskRecordSourceVariant,
} from '../lib/risk-records'
import type { RiskRecord, RiskRecordChunk } from '../types'

export function RiskRecordIdList(props: {
  readonly values: readonly number[]
}) {
  return props.values.length ? (
    <span className='break-words'>{props.values.join(', ')}</span>
  ) : (
    <span className='text-muted-foreground'>-</span>
  )
}

export function RiskRecordCategoryList(props: {
  readonly values: readonly string[]
}) {
  const { t } = useTranslation()
  const categories = [...new Set(props.values)]

  return categories.length ? (
    <div className='flex min-w-0 flex-wrap gap-1'>
      {categories.map((category) => {
        const label = getRiskRecordCategoryLabel(category)
        const displayLabel = label ? `${category} · ${t(label)}` : category

        return (
          <StatusBadge
            key={category}
            label={displayLabel}
            variant='neutral'
            type='text'
            copyable={false}
            className='h-auto max-w-full py-0.5 whitespace-normal'
          >
            <span className='min-w-0 leading-normal break-words whitespace-normal'>
              {displayLabel}
            </span>
          </StatusBadge>
        )
      })}
    </div>
  ) : (
    <span className='text-muted-foreground'>-</span>
  )
}

export function RiskRecordChunkList(props: {
  readonly chunks: readonly RiskRecordChunk[]
}) {
  const { t } = useTranslation()

  return (
    <div className='space-y-2'>
      {props.chunks.map((chunk) => {
        const resultLabel = getRiskRecordResultLabel(chunk.result)

        return (
          <div
            key={chunk.index}
            className='bg-muted/40 min-w-0 rounded-md border p-2'
          >
            <div className='flex min-w-0 items-start justify-between gap-2'>
              <p className='text-xs font-medium tabular-nums'>
                {t('Cloud call')} #{chunk.index + 1}
              </p>
              <StatusBadge
                label={
                  resultLabel
                    ? t(resultLabel)
                    : t('Unknown value: {{value}}', { value: chunk.result })
                }
                variant={getRiskRecordResultVariant(chunk.result)}
                copyable={false}
                className='h-auto max-w-full py-0.5 whitespace-normal'
              />
            </div>
            <div className='mt-1.5'>
              <RiskRecordCategoryList values={chunk.categories} />
            </div>
            <dl className='mt-2 grid min-w-0 grid-cols-2 gap-2 text-xs sm:grid-cols-3 xl:grid-cols-5'>
              <div>
                <dt className='text-muted-foreground'>{t('Latency')}</dt>
                <dd className='tabular-nums'>{chunk.latency_ms} ms</dd>
              </div>
              <div>
                <dt className='text-muted-foreground'>{t('Prompt')}</dt>
                <dd className='tabular-nums'>
                  {chunk.prompt_tokens.toLocaleString()}
                </dd>
              </div>
              <div>
                <dt className='text-muted-foreground'>{t('Completion')}</dt>
                <dd className='tabular-nums'>
                  {chunk.completion_tokens.toLocaleString()}
                </dd>
              </div>
              <div>
                <dt className='text-muted-foreground'>{t('Total tokens')}</dt>
                <dd className='tabular-nums'>
                  {chunk.total_tokens.toLocaleString()}
                </dd>
              </div>
              <div>
                <dt className='text-muted-foreground'>{t('Neurons')}</dt>
                <dd className='tabular-nums'>
                  {chunk.neurons.toLocaleString()}
                </dd>
              </div>
            </dl>
          </div>
        )
      })}
    </div>
  )
}

export function RiskRecordBadges(props: { readonly record: RiskRecord }) {
  const { record } = props

  return (
    <div className='flex min-w-0 flex-wrap gap-1'>
      <RiskRecordSourceBadge source={record.source} />
      <RiskRecordResultBadge result={record.result} />
    </div>
  )
}

export function RiskRecordResultBadge(props: { readonly result: string }) {
  const { t } = useTranslation()
  const label = getRiskRecordResultLabel(props.result)

  return (
    <StatusBadge
      label={
        label
          ? t(label)
          : t('Unknown value: {{value}}', { value: props.result })
      }
      variant={getRiskRecordResultVariant(props.result)}
      type='text'
      copyable={false}
      className='h-auto max-w-full py-0.5 whitespace-normal'
    />
  )
}

export function RiskRecordSourceBadge(props: { readonly source: string }) {
  const { t } = useTranslation()
  if (!props.source) return null
  const label = getRiskRecordSourceLabel(props.source)

  return (
    <StatusBadge
      label={
        label
          ? t(label)
          : t('Unknown value: {{value}}', { value: props.source })
      }
      variant={getRiskRecordSourceVariant(props.source)}
      type='text'
      copyable={false}
      className='h-auto max-w-full py-0.5 whitespace-normal'
    />
  )
}

export function RiskRecordResultSummary(props: {
  readonly record: RiskRecord
}) {
  const { t } = useTranslation()
  const { record } = props

  return (
    <div className='min-w-0 space-y-1.5'>
      <RiskRecordBadges record={record} />
      <RiskRecordCategoryList values={record.categories} />
      {record.error_code && (
        <p
          className='text-destructive text-xs break-all'
          title={record.error_code}
        >
          {t('Error')}: {record.error_code}
        </p>
      )}
    </div>
  )
}

export function RiskRecordProviderSummary(props: {
  readonly record: RiskRecord
}) {
  const { t } = useTranslation()
  const { record } = props

  if (!record.provider_id && !record.provider_name) {
    return <span className='text-muted-foreground'>{t('None')}</span>
  }

  return (
    <span
      className='inline-flex max-w-full min-w-0 items-center gap-1.5 whitespace-nowrap'
      title={`${record.provider_name} #${record.provider_id}`}
    >
      <span className='min-w-0 truncate font-medium'>
        {record.provider_name}
      </span>
      <span className='text-muted-foreground shrink-0 text-xs'>
        #{record.provider_id}
      </span>
    </span>
  )
}

export function RiskRecordUsageSummary(props: { readonly record: RiskRecord }) {
  const { t } = useTranslation()
  const { record } = props

  return (
    <dl className='grid min-w-0 grid-cols-[auto_1fr] gap-x-2 gap-y-1 text-xs'>
      <dt className='text-muted-foreground'>{t('Latency')}</dt>
      <dd className='text-right tabular-nums'>{record.latency_ms} ms</dd>
      <dt className='text-muted-foreground'>{t('Cloud call')}</dt>
      <dd className='text-right'>
        {record.provider_called ? t('Yes') : t('No')}
      </dd>
      <dt className='text-muted-foreground'>{t('Prompt')}</dt>
      <dd className='text-right tabular-nums'>
        {record.prompt_tokens.toLocaleString()}
      </dd>
      <dt className='text-muted-foreground'>{t('Completion')}</dt>
      <dd className='text-right tabular-nums'>
        {record.completion_tokens.toLocaleString()}
      </dd>
      <dt className='text-muted-foreground'>{t('Total tokens')}</dt>
      <dd className='text-right tabular-nums'>
        {record.total_tokens.toLocaleString()}
      </dd>
      <dt className='text-muted-foreground'>{t('Neurons')}</dt>
      <dd className='text-right tabular-nums'>
        {record.neurons.toLocaleString()}
      </dd>
    </dl>
  )
}
