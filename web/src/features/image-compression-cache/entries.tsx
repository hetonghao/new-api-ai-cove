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
import { useMutation } from '@tanstack/react-query'
import type { OnChangeFn, PaginationState } from '@tanstack/react-table'
import { Trash2 } from 'lucide-react'
import { useCallback, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { ConfirmDialog } from '@/components/confirm-dialog'
import {
  DataTableBulkActions,
  DataTablePage,
  useDataTable,
} from '@/components/data-table'
import { Button } from '@/components/ui/button'
import { FieldError } from '@/components/ui/field'
import { TooltipProvider } from '@/components/ui/tooltip'
import {
  getServerErrorMessage,
  getServerErrorStatus,
} from '@/lib/server-error-message'

import { clearCacheEntries, type CacheStats, type ClearTarget } from './api'
import { useEntryColumns } from './entry-columns'

export function CacheEntries(props: {
  readonly stats: CacheStats
  readonly pagination: PaginationState
  readonly setPagination: OnChangeFn<PaginationState>
  readonly stale: boolean
  readonly refresh: () => Promise<void>
}) {
  const { t } = useTranslation()
  const titleRef = useRef<HTMLHeadingElement>(null)
  const [target, setTarget] = useState<ClearTarget | null>(null)
  const stats = props.stats
  const clear = useMutation({
    mutationFn: clearCacheEntries,
    retry: false,
    onSuccess: async (count) => {
      setTarget(null)
      table.resetRowSelection()
      await props.refresh()
      toast.success(t('Cache entries cleared: {{count}}', { count }))
    },
    onError: async (error) => {
      const status = getServerErrorStatus(error)
      if (
        status === 409 ||
        status === 404 ||
        status === 503 ||
        status === undefined
      ) {
        setTarget(null)
        table.resetRowSelection()
        await props.refresh()
        toast.error(
          status === 503 || status === undefined
            ? t(
                'Could not confirm the removal. Check the latest statistics before trying again.'
              )
            : getServerErrorMessage(error)
        )
      }
    },
    meta: { errorToast: false },
  })
  const resetClear = clear.reset
  const beginClear = useCallback(
    (ids: readonly string[]) => {
      resetClear()
      setTarget({
        node_name: stats.node_name,
        instance_id: stats.instance_id,
        ids,
      })
    },
    [resetClear, stats.instance_id, stats.node_name]
  )
  const disabled = props.stale || clear.isPending
  const columns = useEntryColumns(beginClear, disabled)
  const table = useDataTable({
    columns,
    data: stats.entries,
    enableRowSelection: true,
    enableSorting: false,
    getRowId: (entry) => entry.id,
    manualPagination: true,
    pagination: props.pagination,
    onPaginationChange: props.setPagination,
    totalCount: stats.entry_count,
    ensurePageInRange: (pageCount) => {
      const pageIndex = Math.max(0, pageCount - 1)
      if (!props.stale && props.pagination.pageIndex > pageIndex) {
        props.setPagination((previous) => ({ ...previous, pageIndex }))
      }
    },
    columnSizingStorageKey: false,
    columnVisibilityStorageKey: false,
  }).table
  const selectedIds = table
    .getFilteredSelectedRowModel()
    .rows.map((row) => row.original.id)

  return (
    <section className='min-w-0 space-y-3'>
      <div className='flex flex-wrap items-center justify-between gap-2'>
        <h3
          className='focus-visible:outline-ring text-sm font-semibold'
          ref={titleRef}
          tabIndex={-1}
        >
          {t('Cache Entries')}
        </h3>
        <TooltipProvider>
          <DataTableBulkActions
            table={table}
            placement='inline'
            entityName={t('Cache Entries')}
          >
            <Button
              type='button'
              variant='outline'
              disabled={disabled}
              onClick={() => beginClear(selectedIds)}
            >
              <Trash2 aria-hidden='true' />
              {t('Clear selected')}
            </Button>
          </DataTableBulkActions>
        </TooltipProvider>
      </div>
      {clear.isError && target === null && (
        <FieldError>{getServerErrorMessage(clear.error)}</FieldError>
      )}
      <DataTablePage
        table={table}
        columns={columns}
        emptyTitle={t('No cached images')}
        emptyDescription={t(
          'Compressed images will appear here when this node caches them.'
        )}
        toolbarProps={null}
        mobileProps={{ enableRowSelection: true }}
        compactPagination
        showPagination={stats.entry_count > props.pagination.pageSize}
        paginationInFooter={false}
        fixedHeight={false}
      />
      <ConfirmDialog
        open={target !== null}
        onOpenChange={(open) => {
          if (!open && !clear.isPending) setTarget(null)
        }}
        title={
          target?.ids.length === 1
            ? t('Clear cache entry?')
            : t('Clear selected cache entries?')
        }
        desc={t(
          'Remove {{count}} cache entries from {{node}}. Future requests may compress these images again.',
          { count: target?.ids.length ?? 0, node: target?.node_name }
        )}
        confirmText={t('Clear')}
        destructive
        disabled={disabled}
        isLoading={clear.isPending}
        finalFocus={titleRef}
        handleConfirm={() => {
          if (target && !disabled) clear.mutate(target)
        }}
      >
        {clear.isError && (
          <FieldError>{getServerErrorMessage(clear.error)}</FieldError>
        )}
      </ConfirmDialog>
    </section>
  )
}
