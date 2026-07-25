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
  const categories = [...new Set(props.values)]

  return categories.length ? (
    <div className='flex min-w-0 flex-wrap gap-1'>
      {categories.map((category) => (
        <StatusBadge
          key={category}
          label={category}
          variant='neutral'
          copyable={false}
          className='h-auto max-w-full py-0.5 whitespace-normal'
        />
      ))}
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
  const { t } = useTranslation()
  const { record } = props
  const resultLabel = getRiskRecordResultLabel(record.result)
  const sourceLabel = getRiskRecordSourceLabel(record.source)

  return (
    <div className='flex min-w-0 flex-wrap gap-1'>
      <StatusBadge
        label={
          resultLabel
            ? t(resultLabel)
            : t('Unknown value: {{value}}', { value: record.result })
        }
        variant={getRiskRecordResultVariant(record.result)}
        copyable={false}
        className='h-auto max-w-full py-0.5 whitespace-normal'
      />
      {record.source && (
        <StatusBadge
          label={
            sourceLabel
              ? t(sourceLabel)
              : t('Unknown value: {{value}}', { value: record.source })
          }
          variant={getRiskRecordSourceVariant(record.source)}
          copyable={false}
          className='h-auto max-w-full py-0.5 whitespace-normal'
        />
      )}
    </div>
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
    <div className='min-w-0'>
      <p className='font-medium break-words' title={record.provider_name}>
        {record.provider_name}
      </p>
      <p className='text-muted-foreground text-xs'>#{record.provider_id}</p>
    </div>
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
