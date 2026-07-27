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

import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { formatDateTimeStr } from '@/lib/format'

import type { RiskRecord } from '../types'
import { RiskRecordDetailsButton } from './risk-record-details-dialog'
import {
  RiskRecordBadges,
  RiskRecordCategoryList,
  RiskRecordChannelSummary,
  RiskRecordIdList,
  RiskRecordLatency,
  RiskRecordProviderSummary,
  RiskRecordResultBadge,
  RiskRecordSourceBadge,
  RiskRecordUserSummary,
} from './risk-record-summary'

export function RiskRecordDesktopTable(props: {
  readonly records: readonly RiskRecord[]
  readonly onUserClick: (record: RiskRecord) => void
}) {
  const { t } = useTranslation()

  return (
    <div className='hidden overflow-hidden rounded-lg border xl:block'>
      <Table className='table-fixed'>
        <TableHeader>
          <TableRow>
            <TableHead className='w-[12%]'>{t('Time')}</TableHead>
            <TableHead className='w-[12%]'>{t('Channel')}</TableHead>
            <TableHead className='w-[13%]'>{t('User')}</TableHead>
            <TableHead className='w-[12%]'>{t('Provider')}</TableHead>
            <TableHead className='w-[10%]'>{t('Source')}</TableHead>
            <TableHead className='w-[9%]'>{t('Result')}</TableHead>
            <TableHead className='w-[14%]'>{t('Categories')}</TableHead>
            <TableHead className='w-[8%]'>{t('Latency')}</TableHead>
            <TableHead className='w-[10%]'>{t('Details')}</TableHead>
          </TableRow>
        </TableHeader>
        <TableBody>
          {props.records.map((record) => (
            <TableRow key={record.id}>
              <TableCell className='whitespace-normal'>
                <span className='text-xs tabular-nums'>
                  {formatDateTimeStr(new Date(record.observed_at))}
                </span>
              </TableCell>
              <TableCell className='whitespace-normal'>
                <RiskRecordChannelSummary record={record} />
              </TableCell>
              <TableCell className='whitespace-normal'>
                <RiskRecordUserSummary
                  record={record}
                  onClick={props.onUserClick}
                />
              </TableCell>
              <TableCell className='whitespace-normal'>
                <RiskRecordProviderSummary record={record} />
              </TableCell>
              <TableCell className='whitespace-normal'>
                <RiskRecordSourceBadge source={record.source} />
              </TableCell>
              <TableCell className='whitespace-normal'>
                <RiskRecordResultBadge result={record.result} />
              </TableCell>
              <TableCell className='whitespace-normal'>
                <RiskRecordCategoryList values={record.categories} />
                {record.rule_ids.length > 0 && (
                  <dl className='mt-1 grid grid-cols-[auto_1fr] gap-x-2 text-xs'>
                    <dt className='text-muted-foreground'>{t('Rules')}</dt>
                    <dd className='min-w-0'>
                      <RiskRecordIdList values={record.rule_ids} />
                    </dd>
                  </dl>
                )}
              </TableCell>
              <TableCell className='text-xs whitespace-normal'>
                <RiskRecordLatency latencyMs={record.latency_ms} />
              </TableCell>
              <TableCell className='whitespace-normal'>
                <RiskRecordDetailsButton
                  record={record}
                  onUserClick={props.onUserClick}
                />
              </TableCell>
            </TableRow>
          ))}
        </TableBody>
      </Table>
    </div>
  )
}

export function RiskRecordMobileList(props: {
  readonly records: readonly RiskRecord[]
  readonly onUserClick: (record: RiskRecord) => void
}) {
  const { t } = useTranslation()

  return (
    <div className='divide-y overflow-hidden rounded-lg border xl:hidden'>
      {props.records.map((record) => (
        <article key={record.id} className='bg-table-row min-w-0 p-3'>
          <div className='flex min-w-0 items-start justify-between gap-3'>
            <p className='text-muted-foreground min-w-0 text-xs tabular-nums'>
              {formatDateTimeStr(new Date(record.observed_at))}
            </p>
            <div className='flex max-w-1/2 justify-end'>
              <RiskRecordBadges record={record} />
            </div>
          </div>
          <dl className='mt-3 grid min-w-0 grid-cols-1 gap-3 sm:grid-cols-2'>
            <div className='min-w-0'>
              <dt className='text-muted-foreground text-xs'>{t('Channel')}</dt>
              <dd className='text-sm'>
                <RiskRecordChannelSummary record={record} />
              </dd>
            </div>
            <div className='min-w-0'>
              <dt className='text-muted-foreground text-xs'>{t('User')}</dt>
              <dd className='text-sm'>
                <RiskRecordUserSummary
                  record={record}
                  onClick={props.onUserClick}
                />
              </dd>
            </div>
            {record.rule_ids.length > 0 && (
              <div className='min-w-0'>
                <dt className='text-muted-foreground text-xs'>{t('Rules')}</dt>
                <dd className='text-sm'>
                  <RiskRecordIdList values={record.rule_ids} />
                </dd>
              </div>
            )}
            <div className='min-w-0'>
              <dt className='text-muted-foreground text-xs'>{t('Provider')}</dt>
              <dd className='text-sm'>
                <RiskRecordProviderSummary record={record} />
              </dd>
            </div>
            <div className='min-w-0 sm:col-span-2'>
              <dt className='text-muted-foreground text-xs'>
                {t('Categories')}
              </dt>
              <dd className='mt-1 text-sm'>
                <RiskRecordCategoryList values={record.categories} />
              </dd>
              {record.error_code && (
                <p
                  className='text-destructive mt-1 text-xs break-all'
                  title={record.error_code}
                >
                  {t('Error')}: {record.error_code}
                </p>
              )}
            </div>
            <div className='min-w-0'>
              <dt className='text-muted-foreground text-xs'>{t('Latency')}</dt>
              <dd className='text-sm'>
                <RiskRecordLatency latencyMs={record.latency_ms} />
              </dd>
            </div>
            <div className='min-w-0'>
              <dt className='text-muted-foreground text-xs'>{t('Details')}</dt>
              <dd>
                <RiskRecordDetailsButton
                  record={record}
                  onUserClick={props.onUserClick}
                />
              </dd>
            </div>
          </dl>
        </article>
      ))}
    </div>
  )
}
