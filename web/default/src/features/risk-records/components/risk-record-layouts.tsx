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
import { Fragment } from 'react'
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
import {
  RiskRecordBadges,
  RiskRecordCategoryList,
  RiskRecordChunkList,
  RiskRecordIdList,
  RiskRecordProviderSummary,
  RiskRecordResultSummary,
  RiskRecordUsageSummary,
} from './risk-record-summary'

export function RiskRecordDesktopTable(props: {
  readonly records: readonly RiskRecord[]
}) {
  const { t } = useTranslation()

  return (
    <div className='hidden overflow-hidden rounded-lg border xl:block'>
      <Table className='table-fixed'>
        <TableHeader>
          <TableRow>
            <TableHead className='w-[25%]'>{t('Request ID')}</TableHead>
            <TableHead className='w-[18%]'>
              {t('Channel')} / {t('User')}
            </TableHead>
            <TableHead className='w-[18%]'>{t('Provider')}</TableHead>
            <TableHead className='w-[21%]'>{t('Result')}</TableHead>
            <TableHead className='w-[18%]'>{t('Tokens')}</TableHead>
          </TableRow>
        </TableHeader>
        <TableBody>
          {props.records.map((record) => (
            <Fragment key={record.id}>
              <TableRow>
                <TableCell className='whitespace-normal'>
                  <p
                    className='truncate font-mono text-xs font-medium'
                    title={record.request_id}
                  >
                    {record.request_id}
                  </p>
                  <p className='text-muted-foreground mt-1 text-xs'>
                    {formatDateTimeStr(new Date(record.observed_at))}
                  </p>
                </TableCell>
                <TableCell className='whitespace-normal'>
                  <dl className='grid grid-cols-[auto_1fr] gap-x-2 gap-y-1 text-xs'>
                    <dt className='text-muted-foreground'>{t('Channel')}</dt>
                    <dd>#{record.channel_id}</dd>
                    <dt className='text-muted-foreground'>{t('User')}</dt>
                    <dd>#{record.user_id}</dd>
                    <dt className='text-muted-foreground'>{t('Rules')}</dt>
                    <dd className='min-w-0'>
                      <RiskRecordIdList values={record.rule_ids} />
                    </dd>
                  </dl>
                </TableCell>
                <TableCell className='whitespace-normal'>
                  <RiskRecordProviderSummary record={record} />
                </TableCell>
                <TableCell className='whitespace-normal'>
                  <RiskRecordResultSummary record={record} />
                </TableCell>
                <TableCell className='whitespace-normal'>
                  <RiskRecordUsageSummary record={record} />
                </TableCell>
              </TableRow>
              {record.chunks.length > 0 && (
                <TableRow>
                  <TableCell
                    colSpan={5}
                    className='bg-muted/20 p-3 whitespace-normal'
                  >
                    <RiskRecordChunkList chunks={record.chunks} />
                  </TableCell>
                </TableRow>
              )}
            </Fragment>
          ))}
        </TableBody>
      </Table>
    </div>
  )
}

export function RiskRecordMobileList(props: {
  readonly records: readonly RiskRecord[]
}) {
  const { t } = useTranslation()

  return (
    <div className='divide-y overflow-hidden rounded-lg border xl:hidden'>
      {props.records.map((record) => (
        <article key={record.id} className='bg-table-row min-w-0 p-3'>
          <div className='flex min-w-0 items-start justify-between gap-3'>
            <div className='min-w-0'>
              <p
                className='truncate font-mono text-xs font-medium'
                title={record.request_id}
              >
                {record.request_id}
              </p>
              <p className='text-muted-foreground mt-1 text-xs'>
                {formatDateTimeStr(new Date(record.observed_at))}
              </p>
            </div>
            <div className='flex max-w-1/2 justify-end'>
              <RiskRecordBadges record={record} />
            </div>
          </div>
          <dl className='mt-3 grid min-w-0 grid-cols-1 gap-3 sm:grid-cols-2'>
            <div className='min-w-0'>
              <dt className='text-muted-foreground text-xs'>{t('Channel')}</dt>
              <dd className='text-sm'>#{record.channel_id}</dd>
            </div>
            <div className='min-w-0'>
              <dt className='text-muted-foreground text-xs'>{t('User')}</dt>
              <dd className='text-sm'>#{record.user_id}</dd>
            </div>
            <div className='min-w-0'>
              <dt className='text-muted-foreground text-xs'>{t('Rules')}</dt>
              <dd className='text-sm'>
                <RiskRecordIdList values={record.rule_ids} />
              </dd>
            </div>
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
            <div className='min-w-0 sm:col-span-2'>
              <RiskRecordUsageSummary record={record} />
            </div>
          </dl>
          {record.chunks.length > 0 && (
            <div className='mt-3 min-w-0'>
              <RiskRecordChunkList chunks={record.chunks} />
            </div>
          )}
        </article>
      ))}
    </div>
  )
}
