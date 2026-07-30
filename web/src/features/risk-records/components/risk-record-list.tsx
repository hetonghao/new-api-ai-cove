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
import type { ColumnDef, PaginationState } from '@tanstack/react-table'
import { ShieldCheck } from 'lucide-react'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import { DataTablePagination, useDataTable } from '@/components/data-table'
import { ErrorState } from '@/components/error-state'
import { PageFooterPortal } from '@/components/layout/components/page-footer'
import {
  Empty,
  EmptyDescription,
  EmptyHeader,
  EmptyMedia,
  EmptyTitle,
} from '@/components/ui/empty'
import { Skeleton } from '@/components/ui/skeleton'
import type { RiskProvider } from '@/features/risk-providers/types'
import { UserInfoDialog } from '@/features/usage-logs/components/dialogs/user-info-dialog'
import { cn } from '@/lib/utils'

import { listRiskRecords } from '../api'
import { createDefaultRiskRecordFilterDraft } from '../lib/default-filter'
import {
  commitRiskRecordFilters,
  shouldRefetchRiskRecords,
} from '../lib/risk-records'
import type { RiskRecord, RiskRecordFilters } from '../types'
import { RiskRecordFiltersForm } from './risk-record-filters'
import {
  RiskRecordDesktopTable,
  RiskRecordMobileList,
} from './risk-record-layouts'

const SKELETON_KEYS = ['record-1', 'record-2', 'record-3'] as const
const PAGINATION_COLUMNS: ColumnDef<RiskRecord>[] = []

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

type RiskRecordListProps = {
  readonly providers: readonly RiskProvider[]
}

export function RiskRecordList(props: RiskRecordListProps) {
  const { t } = useTranslation()
  const [pagination, setPagination] = useState<PaginationState>({
    pageIndex: 0,
    pageSize: 20,
  })
  const [initialDraft] = useState(createDefaultRiskRecordFilterDraft)
  const [filters, setFilters] = useState<RiskRecordFilters>(() =>
    commitRiskRecordFilters(initialDraft)
  )
  const [selectedUserId, setSelectedUserId] = useState<number | null>(null)
  const [userInfoOpen, setUserInfoOpen] = useState(false)
  const [usernameOverride, setUsernameOverride] = useState<{
    value: string
    requestId: number
  }>()
  const recordsQuery = useQuery({
    queryKey: [
      'risk',
      'records',
      pagination.pageIndex + 1,
      pagination.pageSize,
      filters,
    ],
    queryFn: async () => {
      const response = await listRiskRecords(
        pagination.pageIndex + 1,
        pagination.pageSize,
        filters
      )
      if (!response.success || !response.data) {
        throw new Error(response.message || t('Failed to load risk records'))
      }
      return response.data
    },
    placeholderData: keepPreviousData,
    retry: false,
  })
  const records = [...(recordsQuery.data?.items ?? [])]
  const isFetchingOnly = recordsQuery.isFetching && !recordsQuery.isLoading
  const { table } = useDataTable<RiskRecord>({
    data: records,
    columns: PAGINATION_COLUMNS,
    pagination,
    onPaginationChange: setPagination,
    manualPagination: true,
    totalCount: recordsQuery.data?.total ?? 0,
    enableRowSelection: false,
  })

  function applyFilters(nextFilters: RiskRecordFilters) {
    const shouldRefetch = shouldRefetchRiskRecords(
      pagination.pageIndex,
      filters,
      nextFilters
    )
    setPagination((current) => ({ ...current, pageIndex: 0 }))
    setFilters(nextFilters)
    if (shouldRefetch) void recordsQuery.refetch()
  }

  function openUser(record: RiskRecord) {
    setSelectedUserId(record.user_id)
    setUserInfoOpen(true)
  }

  function filterByUsername(username: string) {
    setUsernameOverride((current) => ({
      value: username,
      requestId: (current?.requestId ?? 0) + 1,
    }))
    applyFilters({ ...filters, username })
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
        <RiskRecordDesktopTable records={records} onUserClick={openUser} />
        <RiskRecordMobileList records={records} onUserClick={openUser} />
      </>
    ) : (
      <EmptyRecords />
    )
  }

  return (
    <>
      <div className='flex h-full min-h-0 flex-col gap-2.5 sm:gap-3'>
        <RiskRecordFiltersForm
          disabled={recordsQuery.isFetching}
          initialValues={initialDraft}
          onApply={applyFilters}
          providers={props.providers}
          usernameOverride={usernameOverride}
        />
        <div
          className={cn(
            'min-h-0 flex-1 overflow-y-auto transition-opacity duration-150',
            isFetchingOnly && 'pointer-events-none opacity-60'
          )}
        >
          {content}
        </div>
      </div>
      <PageFooterPortal>
        <DataTablePagination table={table} />
      </PageFooterPortal>
      <UserInfoDialog
        userId={selectedUserId}
        open={userInfoOpen}
        onOpenChange={setUserInfoOpen}
        onFilterByUsername={filterByUsername}
        filterButtonLabel='Filter risk records by this user'
      />
    </>
  )
}
