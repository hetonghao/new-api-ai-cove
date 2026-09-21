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
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Square } from 'lucide-react'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { ConfirmDialog } from '@/components/confirm-dialog'
import { CopyButton } from '@/components/copy-button'
import {
  DataTablePagination,
  StaticDataTable,
  useDataTable,
  type StaticDataTableColumn,
} from '@/components/data-table'
import { CompactDateTimeRangePicker } from '@/features/usage-logs/components/compact-date-time-range-picker'
import { getDefaultTimeRange } from '@/features/usage-logs/lib/utils'
import { EmptyState } from '@/components/empty-state'
import { ErrorState } from '@/components/error-state'
import { StatusBadge } from '@/components/status-badge'
import { Button } from '@/components/ui/button'
import {
  NativeSelect,
  NativeSelectOption,
} from '@/components/ui/native-select'
import { Skeleton } from '@/components/ui/skeleton'
import { TableCell, TableRow } from '@/components/ui/table'
import { formatTimestampToDate } from '@/lib/format'
import { handleServerError } from '@/lib/handle-server-error'

import { cancelQualityRun, getQualityRuns } from '../api'
import {
  RUN_SOURCE_LABEL,
  RUN_STATUS_LABEL,
  RUN_STATUS_VARIANT,
  RUNS_PAGE_SIZE,
  runStatusLabel,
} from '../constants'
import type { ModelQualityRun, QualityCapabilities, QualityCaseView } from '../types'
import { RunDetailDialog } from './run-detail-dialog'

type RunsTableProps = {
  cases: readonly QualityCaseView[]
  capabilities: QualityCapabilities | undefined
}

function StopRunButton(props: { runId: string; canOperate: boolean }) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const [open, setOpen] = useState(false)
  const mutation = useMutation({
    mutationFn: () => cancelQualityRun(props.runId),
    retry: false,
    onSuccess: (response) => {
      if (!response.success) {
        toast.error(response.message || t('Request failed'))
        return
      }
      toast.success(t('Run cancellation requested'))
      setOpen(false)
      void queryClient.invalidateQueries({ queryKey: ['model-quality'] })
    },
    onError: (error) => {
      handleServerError(error, t('Failed to cancel the run'))
    },
  })
  return (
    <>
      <Button
        size='sm'
        variant='outline'
        className='text-destructive'
        disabled={!props.canOperate}
        onClick={(event) => {
          event.stopPropagation()
          setOpen(true)
        }}
      >
        <Square data-icon='inline-start' />
        {t('Stop')}
      </Button>
      <ConfirmDialog
        open={open}
        onOpenChange={setOpen}
        title={t('Stop run')}
        desc={t(
          'Pending samples are cancelled; in-flight requests are asked to stop, upstream billing may still occur.'
        )}
        destructive
        isLoading={mutation.isPending}
        handleConfirm={() => mutation.mutate()}
      />
    </>
  )
}

export function RunsTable(props: RunsTableProps) {
  const { t } = useTranslation()
  const [caseFilter, setCaseFilter] = useState(0)
  const [sourceFilter, setSourceFilter] = useState('')
  const [statusFilter, setStatusFilter] = useState('')
  const [dateRange, setDateRange] = useState<{ start?: Date; end?: Date }>(
    () => getDefaultTimeRange()
  )
  const [pagination, setPagination] = useState({
    pageIndex: 0,
    pageSize: RUNS_PAGE_SIZE,
  })
  const [detailRunId, setDetailRunId] = useState<string | null>(null)

  const resetPage = () =>
    setPagination((previous) => ({ ...previous, pageIndex: 0 }))
  const resetFilters = () => {
    setCaseFilter(0)
    setSourceFilter('')
    setStatusFilter('')
    setDateRange(getDefaultTimeRange())
    resetPage()
  }

  const runsQuery = useQuery({
    queryKey: [
      'model-quality',
      'runs',
      {
        caseFilter,
        sourceFilter,
        statusFilter,
        start: dateRange.start?.getTime() ?? 0,
        end: dateRange.end?.getTime() ?? 0,
        pagination,
      },
    ],
    queryFn: async () => {
      const response = await getQualityRuns({
        p: pagination.pageIndex + 1,
        page_size: pagination.pageSize,
        case_id: caseFilter === 0 ? undefined : caseFilter,
        source: sourceFilter === '' ? undefined : sourceFilter,
        status: statusFilter === '' ? undefined : statusFilter,
        start: dateRange.start?.getTime(),
        end: dateRange.end?.getTime(),
      })
      if (!response.success || !response.data) {
        throw new Error(response.message)
      }
      return response.data
    },
    refetchInterval: (query) =>
      query.state.data?.items.some(
        (run) => run.status === 'queued' || run.status === 'running'
      )
        ? 2000
        : 30000,
    refetchIntervalInBackground: false,
    retry: false,
  })
  const runs = runsQuery.data?.items ?? []
  const { table } = useDataTable({
    data: runs,
    columns: [],
    totalCount: runsQuery.data?.total ?? 0,
    manualPagination: true,
    columnFilters: [],
    pagination,
    onPaginationChange: setPagination,
    columnVisibilityStorageKey: false,
    columnSizingStorageKey: false,
    ensurePageInRange: (pageCount) => {
      if (
        runsQuery.isSuccess &&
        !runsQuery.isFetching &&
        pagination.pageIndex >= Math.max(1, pageCount)
      ) {
        setPagination((previous) => ({
          ...previous,
          pageIndex: Math.max(0, pageCount - 1),
        }))
      }
    },
  })

  const caseName = (caseId: number) =>
    props.cases.find((entry) => entry.id === caseId)?.name ?? `#${caseId}`

  const columns: StaticDataTableColumn<ModelQualityRun>[] = [
    {
      id: 'id',
      header: t('Run ID'),
      cell: (run) => (
        <span className='flex items-center gap-1 font-mono text-xs'>
          <span className='max-w-28 truncate' title={run.id}>
            {run.id}
          </span>
          <CopyButton value={run.id} />
        </span>
      ),
    },
    {
      id: 'case',
      header: t('Case'),
      cell: (run) => caseName(run.case_id),
    },
    {
      id: 'source',
      header: t('Trigger'),
      cell: (run) => (
        <span>{RUN_SOURCE_LABEL[run.source] ? t(RUN_SOURCE_LABEL[run.source]) : run.source}</span>
      ),
    },
    {
      id: 'status',
      header: t('Status'),
      cell: (run) => (
        <StatusBadge
          variant={RUN_STATUS_VARIANT[run.status] ?? 'neutral'}
          pulse={run.status === 'running'}
          copyable={false}
          label={t(runStatusLabel(run.status))}
        />
      ),
    },
    {
      id: 'samples',
      header: t('Samples'),
      cellClassName: 'tabular-nums',
      cell: (run) => run.sample_count,
    },
    {
      id: 'created',
      header: t('Created'),
      cellClassName: 'tabular-nums',
      cell: (run) => formatTimestampToDate(run.created_at, 'milliseconds'),
    },
    {
      id: 'finished',
      header: t('Finished'),
      cellClassName: 'tabular-nums',
      cell: (run) =>
        run.finished_at > 0
          ? formatTimestampToDate(run.finished_at, 'milliseconds')
          : '—',
    },
    {
      id: 'actions',
      header: '',
      cell: (run) =>
        (run.status === 'queued' || run.status === 'running') && (
          <StopRunButton
            runId={run.id}
            canOperate={props.capabilities?.can_operate ?? false}
          />
        ),
    },
  ]

  return (
    <div className='space-y-3'>
      <div className='flex flex-wrap items-center gap-2'>
        <NativeSelect
          aria-label={t('Filter by case')}
          className='w-full sm:w-48'
          value={String(caseFilter)}
          onChange={(event) => {
            setCaseFilter(Number(event.target.value))
            resetPage()
          }}
        >
          <NativeSelectOption value={0}>{t('All cases')}</NativeSelectOption>
          {props.cases.map((qualityCase) => (
            <NativeSelectOption key={qualityCase.id} value={qualityCase.id}>
              {qualityCase.name}
            </NativeSelectOption>
          ))}
        </NativeSelect>
        <NativeSelect
          aria-label={t('Filter by trigger')}
          className='w-full sm:w-36'
          value={sourceFilter}
          onChange={(event) => {
            setSourceFilter(event.target.value)
            resetPage()
          }}
        >
          <NativeSelectOption value=''>{t('All triggers')}</NativeSelectOption>
          <NativeSelectOption value='manual'>{t('Manual')}</NativeSelectOption>
          <NativeSelectOption value='scheduled'>
            {t('Scheduled')}
          </NativeSelectOption>
        </NativeSelect>
        <NativeSelect
          aria-label={t('Filter by status')}
          className='w-full sm:w-36'
          value={statusFilter}
          onChange={(event) => {
            setStatusFilter(event.target.value)
            resetPage()
          }}
        >
          <NativeSelectOption value=''>{t('All statuses')}</NativeSelectOption>
          {Object.keys(RUN_STATUS_LABEL).map((status) => (
            <NativeSelectOption key={status} value={status}>
              {t(RUN_STATUS_LABEL[status])}
            </NativeSelectOption>
          ))}
        </NativeSelect>
        <CompactDateTimeRangePicker
          start={dateRange.start}
          end={dateRange.end}
          onChange={(range) => {
            setDateRange(range)
            resetPage()
          }}
        />
        <Button variant='ghost' size='sm' onClick={resetFilters}>
          {t('Reset filters')}
        </Button>
      </div>

      {runsBody()}

      <RunDetailDialog
        runId={detailRunId}
        open={detailRunId !== null}
        onOpenChange={(open) => !open && setDetailRunId(null)}
        canOperate={props.capabilities?.can_operate ?? false}
      />
    </div>
  )

  function runsBody() {
    if (runsQuery.isPending) {
      return <Skeleton className='h-64 w-full' />
    }
    if (runsQuery.isError) {
      return (
        <ErrorState
          description={t('Failed to load runs')}
          onRetry={() => void runsQuery.refetch()}
        />
      )
    }
    if (runs.length === 0) {
      return (
        <EmptyState
          bordered
          title={t('No runs yet')}
          description={t('Start a test from the panel to see batches here.')}
        />
      )
    }
    return (
        <>
          <div className='hidden sm:block'>
            <StaticDataTable
              columns={columns}
              data={runs}
              getRowKey={(run) => run.id}
              renderRow={(run) => (
                <TableRow
                  className='cursor-pointer'
                  onClick={() => setDetailRunId(run.id)}
                >
                  {columns.map((column) => (
                    <TableCell
                      key={column.id}
                      className={
                        typeof column.cellClassName === 'string'
                          ? column.cellClassName
                          : undefined
                      }
                    >
                      {column.cell?.(run, 0)}
                    </TableCell>
                  ))}
                </TableRow>
              )}
            />
          </div>
          <div className='border divide-border divide-y rounded-lg sm:hidden'>
            {runs.map((run) => (
              <div
                key={run.id}
                className='flex items-start gap-2 px-3 py-2'
              >
                <button
                  type='button'
                  onClick={() => setDetailRunId(run.id)}
                  className='min-w-0 flex-1 space-y-1 text-left'
                >
                  <div className='flex items-center justify-between gap-2'>
                    <span className='truncate font-mono text-xs'>{run.id}</span>
                    <StatusBadge
                      variant={RUN_STATUS_VARIANT[run.status] ?? 'neutral'}
                      pulse={run.status === 'running'}
                      copyable={false}
                      label={t(runStatusLabel(run.status))}
                    />
                  </div>
                  <div className='text-muted-foreground flex justify-between text-xs'>
                    <span className='truncate'>{caseName(run.case_id)}</span>
                    <span className='tabular-nums'>
                      {t('{{count}} samples', { count: run.sample_count })}
                    </span>
                  </div>
                  <div className='text-muted-foreground text-xs'>
                    {formatTimestampToDate(run.created_at, 'milliseconds')}
                  </div>
                </button>
                {(run.status === 'queued' || run.status === 'running') && (
                  <StopRunButton
                    runId={run.id}
                    canOperate={props.capabilities?.can_operate ?? false}
                  />
                )}
              </div>
            ))}
          </div>
          <div className='hidden sm:block'>
            <DataTablePagination table={table} />
          </div>
          <div className='sm:hidden'>
            <DataTablePagination table={table} compact />
          </div>
        </>
    )
  }
}
