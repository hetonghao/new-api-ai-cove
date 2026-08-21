import { keepPreviousData, useQuery } from '@tanstack/react-query'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import { DataTablePagination, useDataTable } from '@/components/data-table'
import { Dialog } from '@/components/dialog'
import { ErrorState } from '@/components/error-state'
import { PageFooterPortal } from '@/components/layout/components/page-footer'
import { Button } from '@/components/ui/button'
import {
  Empty,
  EmptyDescription,
  EmptyHeader,
  EmptyMedia,
  EmptyTitle,
} from '@/components/ui/empty'
import { Skeleton } from '@/components/ui/skeleton'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { formatDateTimeStr } from '@/lib/format'

import { getSevereRiskRecord, listSevereRiskRecords } from '../api'
import type { SevereRiskActionStatus, SevereRiskRecord } from '../types'

const ACTION_STATUS_LABELS: Readonly<Record<SevereRiskActionStatus, string>> = {
  pending: 'pending',
  success: 'success',
  failed: 'failed',
  disabled: 'disabled',
}

function Status(props: { readonly value: SevereRiskActionStatus }) {
  const { t } = useTranslation()
  let className = 'text-warning'
  if (props.value === 'success') className = 'text-success'
  if (props.value === 'disabled') className = 'text-muted-foreground'
  return <span className={className}>{t(ACTION_STATUS_LABELS[props.value])}</span>
}

export function SevereRiskRecordList() {
  const { t } = useTranslation()
  const [page, setPage] = useState({ pageIndex: 0, pageSize: 20 })
  const [selected, setSelected] = useState<SevereRiskRecord | null>(null)
  const detailQuery = useQuery({
    queryKey: ['risk', 'severe-record', selected?.id],
    queryFn: () => getSevereRiskRecord(selected?.id ?? 0),
    enabled: selected !== null,
    retry: false,
  })
  const recordsQuery = useQuery({
    queryKey: ['risk', 'severe-records', page],
    queryFn: () => listSevereRiskRecords(page.pageIndex + 1, page.pageSize),
    placeholderData: keepPreviousData,
    retry: false,
  })
  const records = recordsQuery.data?.data?.items ?? []
  const table = useDataTable<SevereRiskRecord>({
    data: records,
    columns: [],
    pagination: page,
    onPaginationChange: setPage,
    manualPagination: true,
    totalCount: recordsQuery.data?.data?.total ?? 0,
    enableRowSelection: false,
  }).table

  let content = <Skeleton className='h-48 w-full' />
  if (recordsQuery.error) {
    content = (
      <ErrorState
        title={t('Failed to load severe risk records')}
        description={t('Request failed')}
        onRetry={() => void recordsQuery.refetch()}
      />
    )
  } else if (!recordsQuery.isLoading) {
    content =
      records.length === 0 ? (
        <Empty className='min-h-64 rounded-lg border'>
          <EmptyHeader>
            <EmptyMedia />
            <EmptyTitle>{t('No severe risk records')}</EmptyTitle>
            <EmptyDescription>
              {t(
                'Severe risk records appear here after an invalid prompt rejection.'
              )}
            </EmptyDescription>
          </EmptyHeader>
        </Empty>
      ) : (
        <div className='overflow-x-auto rounded-lg border'>
          <Table className='w-full table-fixed'>
            <TableHeader>
              <TableRow>
                <TableHead className='w-[32%] sm:w-[18%]'>
                  {t('Time')}
                </TableHead>
                <TableHead className='w-[28%] sm:w-[16%]'>
                  {t('User')}
                </TableHead>
                <TableHead className='w-[24%] sm:w-[15%]'>
                  {t('Channel')}
                </TableHead>
                <TableHead className='hidden w-[14%] sm:table-cell'>
                  {t('Model')}
                </TableHead>
                <TableHead className='hidden w-[23%] sm:table-cell'>
                  {t('Isolation')}
                </TableHead>
                <TableHead className='w-[16%] sm:w-[14%]'>
                  {t('Details')}
                </TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {records.map((record) => (
                <TableRow key={record.id}>
                  <TableCell className='truncate text-xs whitespace-nowrap'>
                    {formatDateTimeStr(new Date(record.triggered_at))}
                  </TableCell>
                  <TableCell className='truncate'>
                    {record.username || `#${record.user_id}`}
                  </TableCell>
                  <TableCell className='truncate'>
                    {record.channel_name || `#${record.channel_id}`}
                  </TableCell>
                  <TableCell
                    className='hidden max-w-48 truncate sm:table-cell'
                    title={record.model}
                  >
                    {record.model}
                  </TableCell>
                  <TableCell className='hidden space-y-1 text-xs sm:table-cell'>
                    <div>
                      {t('User')}: <Status value={record.user_action_status} />
                    </div>
                    <div>
                      {t('Channel')}:{' '}
                      <Status value={record.channel_action_status} />
                    </div>
                  </TableCell>
                  <TableCell>
                    <Button
                      type='button'
                      variant='link'
                      size='sm'
                      onClick={() => setSelected(record)}
                    >
                      {t('View')}
                    </Button>
                  </TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        </div>
      )
  }

  return (
    <>
      <div className='h-full min-h-0 overflow-y-auto'>{content}</div>
      <PageFooterPortal>
        <DataTablePagination table={table} />
      </PageFooterPortal>
      <Dialog
        open={selected !== null}
        onOpenChange={(open) => !open && setSelected(null)}
        title={t('Severe risk record details')}
        description={
          selected
            ? formatDateTimeStr(new Date(selected.triggered_at))
            : undefined
        }
        showCloseButton
      >
        {detailQuery.isLoading ? (
          <Skeleton className='h-48 w-full' />
        ) : (
          detailQuery.data?.data && (
            <div className='space-y-3 text-sm'>
              <p>
                <strong>{t('Error')}:</strong>{' '}
                {detailQuery.data.data.record.error_detail}
              </p>
              <p>
                <strong>{t('Request ID')}:</strong>{' '}
                {detailQuery.data.data.record.request_id}
              </p>
              <p>
                <strong>{t('User')}:</strong>{' '}
                {detailQuery.data.data.record.username ||
                  `#${detailQuery.data.data.record.user_id}`}
              </p>
              <p>
                <strong>{t('API token')}:</strong>{' '}
                {detailQuery.data.data.record.token_name ||
                  `#${detailQuery.data.data.record.token_id}`}
              </p>
              <p>
                <strong>{t('Model')}:</strong>{' '}
                {detailQuery.data.data.record.model}
              </p>
              <p>
                <strong>{t('Path')}:</strong>{' '}
                {detailQuery.data.data.record.path}
              </p>
              <p>
                <strong>{t('Channel')}:</strong>{' '}
                {detailQuery.data.data.record.channel_name ||
                  `#${detailQuery.data.data.record.channel_id}`}
              </p>
              <p>
                <strong>{t('Isolation')}:</strong>{' '}
                {detailQuery.data.data.record.channel_scope === 'key'
                  ? t('Matched key only')
                  : t('Whole channel')}
              </p>
              <pre className='max-h-96 overflow-auto rounded border p-3 whitespace-pre-wrap'>
                {detailQuery.data.data.context}
              </pre>
            </div>
          )
        )}
      </Dialog>
    </>
  )
}
