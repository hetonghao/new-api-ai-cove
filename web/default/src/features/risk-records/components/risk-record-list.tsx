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
import { keepPreviousData, useQuery } from '@tanstack/react-query'
import {
  ChevronLeft,
  ChevronRight,
  History,
  RefreshCw,
  ShieldCheck,
} from 'lucide-react'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import { ErrorState } from '@/components/error-state'
import { Button } from '@/components/ui/button'
import {
  Empty,
  EmptyDescription,
  EmptyHeader,
  EmptyMedia,
  EmptyTitle,
} from '@/components/ui/empty'
import { Skeleton } from '@/components/ui/skeleton'
import { TitledCard } from '@/components/ui/titled-card'

import { listRiskRecords } from '../api'
import { getRiskRecordTotalPages } from '../lib/risk-records'
import type { RiskRecordFilters } from '../types'
import { RiskRecordFiltersForm } from './risk-record-filters'
import {
  RiskRecordDesktopTable,
  RiskRecordMobileList,
} from './risk-record-layouts'

const PAGE_SIZE = 20
const SKELETON_KEYS = ['record-1', 'record-2', 'record-3'] as const

function RecordSkeleton() {
  return (
    <div className='divide-y overflow-hidden rounded-lg border'>
      {SKELETON_KEYS.map((key) => (
        <div key={key} className='space-y-3 p-3'>
          <div className='flex justify-between gap-4'>
            <Skeleton className='h-4 w-1/2' />
            <Skeleton className='h-5 w-16 rounded-full' />
          </div>
          <div className='grid grid-cols-2 gap-3'>
            <Skeleton className='h-10' />
            <Skeleton className='h-10' />
          </div>
        </div>
      ))}
    </div>
  )
}

function EmptyRecords() {
  const { t } = useTranslation()

  return (
    <Empty className='min-h-64 rounded-lg border'>
      <EmptyHeader>
        <EmptyMedia variant='icon'>
          <ShieldCheck />
        </EmptyMedia>
        <EmptyTitle>{t('No risk records')}</EmptyTitle>
        <EmptyDescription>
          {t('Records appear here after cloud review runs.')}
        </EmptyDescription>
      </EmptyHeader>
    </Empty>
  )
}

export function RiskRecordList() {
  const { t } = useTranslation()
  const [page, setPage] = useState(1)
  const [filters, setFilters] = useState<RiskRecordFilters>({})
  const recordsQuery = useQuery({
    queryKey: ['risk', 'records', page, PAGE_SIZE, filters],
    queryFn: async () => {
      const response = await listRiskRecords(page, PAGE_SIZE, filters)
      if (!response.success || !response.data) {
        throw new Error(response.message || t('Failed to load risk records'))
      }
      return response.data
    },
    placeholderData: keepPreviousData,
    retry: false,
  })
  const totalPages = getRiskRecordTotalPages(
    recordsQuery.data?.total ?? 0,
    PAGE_SIZE
  )
  const records = recordsQuery.data?.items ?? []

  function applyFilters(nextFilters: RiskRecordFilters) {
    setPage(1)
    setFilters(nextFilters)
  }

  let content = <RecordSkeleton />
  if (recordsQuery.error) {
    content = (
      <ErrorState
        className='min-h-64 rounded-lg border'
        title={t('Failed to load risk records')}
        description={t('Request failed')}
        onRetry={() => void recordsQuery.refetch()}
      />
    )
  } else if (!recordsQuery.isLoading) {
    content = records.length ? (
      <>
        <RiskRecordDesktopTable records={records} />
        <RiskRecordMobileList records={records} />
      </>
    ) : (
      <EmptyRecords />
    )
  }

  return (
    <TitledCard
      title={t('Risk records')}
      description={t('Review metadata only. Message content is never stored.')}
      descriptionClassName='text-pretty'
      icon={<History className='size-5' />}
      action={
        <Button
          size='sm'
          variant='outline'
          className='w-full sm:w-auto'
          onClick={() => void recordsQuery.refetch()}
          disabled={recordsQuery.isFetching}
        >
          <RefreshCw
            className={
              recordsQuery.isFetching ? 'size-4 animate-spin' : 'size-4'
            }
          />
          {t('Refresh records')}
        </Button>
      }
      disableHoverEffect
    >
      <RiskRecordFiltersForm
        disabled={recordsQuery.isFetching}
        onApply={applyFilters}
      />
      {content}
      {!recordsQuery.error && !recordsQuery.isLoading && (
        <div className='bg-muted/40 mt-3 flex flex-col gap-2 rounded-lg border px-3 py-2 text-sm sm:flex-row sm:items-center sm:justify-between'>
          <p className='text-muted-foreground text-xs tabular-nums'>
            {t('{{count}} risk records', {
              count: recordsQuery.data?.total ?? 0,
            })}
          </p>
          <div className='flex items-center justify-between gap-2 sm:justify-end'>
            <Button
              variant='outline'
              size='icon'
              className='size-8'
              onClick={() => setPage((current) => Math.max(1, current - 1))}
              disabled={page === 1 || recordsQuery.isFetching}
              aria-label={t('Previous page')}
            >
              <ChevronLeft className='size-4' />
            </Button>
            <span className='min-w-24 text-center text-xs font-medium tabular-nums'>
              {t('Page {{current}} of {{total}}', {
                current: page,
                total: totalPages,
              })}
            </span>
            <Button
              variant='outline'
              size='icon'
              className='size-8'
              onClick={() =>
                setPage((current) => Math.min(totalPages, current + 1))
              }
              disabled={
                page >= totalPages ||
                recordsQuery.isFetching ||
                (recordsQuery.data?.total ?? 0) === 0
              }
              aria-label={t('Next page')}
            >
              <ChevronRight className='size-4' />
            </Button>
          </div>
        </div>
      )}
    </TitledCard>
  )
}
