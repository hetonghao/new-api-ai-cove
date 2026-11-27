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
import type { ColumnDef, ColumnFiltersState } from '@tanstack/react-table'
import { CreditCard, Plus, RefreshCw } from 'lucide-react'
import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { ConfirmDialog } from '@/components/confirm-dialog'
import { DataTableToolbar, useDataTable } from '@/components/data-table'
import { EmptyState } from '@/components/empty-state'
import { ErrorState } from '@/components/error-state'
import { SectionPageLayout } from '@/components/layout'
import { Button } from '@/components/ui/button'
import { Skeleton } from '@/components/ui/skeleton'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { DeviMonPage } from '@/features/devin-monitor'
import { DEFAULT_DEVIN_SORT } from '@/features/devin-monitor/sort'

import { deleteUpstreamSubscription, listUpstreamSubscriptions } from './api'
import { SubscriptionFormDialog } from './components/subscription-form-dialog'
import { SubscriptionPlatform } from './components/subscription-platform'
import { SubscriptionRow } from './components/subscription-row'
import { SubscriptionSortControl } from './components/subscription-sort-control'
import type { UpstreamSubscription } from './types'

const EMPTY_SUBSCRIPTIONS: UpstreamSubscription[] = []
const QUERY_KEY = ['upstream-subscriptions'] as const
const FILTER_COLUMNS: ColumnDef<UpstreamSubscription, unknown>[] = [
  { accessorKey: 'name' },
  {
    accessorKey: 'platform',
    filterFn: (row, columnID, value: unknown) =>
      Array.isArray(value) && value.includes(row.getValue(columnID)),
  },
]
const PLATFORM_FILTERS = [
  {
    columnId: 'platform',
    title: 'Platform',
    singleSelect: true,
    options: [
      {
        value: 'opencode',
        label: 'OpenCode Go',
        iconNode: <SubscriptionPlatform platform='opencode' iconOnly />,
      },
      {
        value: 'opendesign',
        label: 'OpenDesign Go',
        iconNode: <SubscriptionPlatform platform='opendesign' iconOnly />,
      },
      {
        value: 'commandcode',
        label: 'CommandCode',
        iconNode: <SubscriptionPlatform platform='commandcode' iconOnly />,
      },
    ],
  },
]

type SubscriptionOperationsTab = 'coding-plan' | 'devin'

export function UpstreamSubscriptions() {
  const { t } = useTranslation()
  const [tab, setTab] = useState<SubscriptionOperationsTab>('devin')
  const queryClient = useQueryClient()
  const [formOpen, setFormOpen] = useState(false)
  const [editing, setEditing] = useState<UpstreamSubscription | null>(null)
  const [removing, setRemoving] = useState<UpstreamSubscription | null>(null)
  const [now, setNow] = useState(Date.now)
  const [sortBy, setSortBy] = useState('platform')
  const [sortDesc, setSortDesc] = useState(false)
  const [devinSort, setDevinSort] = useState(DEFAULT_DEVIN_SORT)
  const [columnFilters, setColumnFilters] = useState<ColumnFiltersState>([])
  const [search, setSearch] = useState('')
  useEffect(() => {
    if (tab !== 'coding-plan') return
    const timer = window.setInterval(() => setNow(Date.now()), 30_000)
    return () => window.clearInterval(timer)
  }, [tab])
  const subscriptions = useQuery({
    queryKey: QUERY_KEY,
    queryFn: ({ signal }) => listUpstreamSubscriptions(signal),
    enabled: tab === 'coding-plan',
    refetchInterval: tab === 'coding-plan' ? 15_000 : false,
    retry: false,
  })
  const { table } = useDataTable({
    data: subscriptions.data ?? EMPTY_SUBSCRIPTIONS,
    autoResetPageIndex: false,
    columns: FILTER_COLUMNS,
    columnFilters,
    onColumnFiltersChange: setColumnFilters,
    globalFilter: search,
    onGlobalFilterChange: setSearch,
    withPaginationRowModel: false,
    withSortedRowModel: false,
    enableRowSelection: false,
    columnVisibilityStorageKey: false,
    globalFilterFn: (row, _columnID, value: unknown) => {
      const search =
        typeof value === 'string' ? value.trim().toLocaleLowerCase() : ''
      return (
        row.original.name.toLocaleLowerCase().includes(search) ||
        (row.original.account ?? '').toLocaleLowerCase().includes(search)
      )
    },
  })
  const remove = useMutation({
    mutationFn: deleteUpstreamSubscription,
    onSuccess: async () => {
      setRemoving(null)
      await queryClient.invalidateQueries({ queryKey: QUERY_KEY })
      toast.success(t('Subscription removed'))
    },
  })

  function create() {
    setEditing(null)
    setFormOpen(true)
  }
  function edit(subscription: UpstreamSubscription) {
    setEditing(subscription)
    setFormOpen(true)
  }
  const visibleSubscriptions = table
    .getFilteredRowModel()
    .rows.map((row) => row.original)
    .sort((a, b) => {
      if (sortBy === 'expiry') {
        const aEnd = a.quota?.subscription_period_end
          ? Date.parse(a.quota.subscription_period_end)
          : Infinity
        const bEnd = b.quota?.subscription_period_end
          ? Date.parse(b.quota.subscription_period_end)
          : Infinity
        if (aEnd !== bEnd) {
          if (!Number.isFinite(aEnd)) return 1
          if (!Number.isFinite(bEnd)) return -1
          return sortDesc ? bEnd - aEnd : aEnd - bEnd
        }
      }
      const platformOrder = a.platform.localeCompare(b.platform)
      return (sortDesc ? -platformOrder : platformOrder) || b.id - a.id
    })
  const sortOptions = [
    { value: 'platform', label: t('Platform') },
    { value: 'expiry', label: t('Subscription expiry') },
  ]

  return (
    <>
      <SectionPageLayout>
        <SectionPageLayout.Title>
          {t('Subscription operations')}
        </SectionPageLayout.Title>
        {tab === 'coding-plan' && (
          <SectionPageLayout.Actions>
            <Button
              variant='outline'
              size='sm'
              disabled={subscriptions.isFetching}
              onClick={() => void subscriptions.refetch()}
            >
              <RefreshCw className='size-4' aria-hidden='true' />
              {t('Refresh list')}
            </Button>
            <Button size='sm' onClick={create}>
              <Plus className='size-4' aria-hidden='true' />
              {t('Create subscription')}
            </Button>
          </SectionPageLayout.Actions>
        )}
        <SectionPageLayout.Content>
          <Tabs
            value={tab}
            onValueChange={(value) => {
              if (value === 'devin' || value === 'coding-plan') {
                if (value === 'devin') {
                  void queryClient.cancelQueries({
                    queryKey: QUERY_KEY,
                    exact: true,
                  })
                }
                setTab(value)
                setNow(Date.now())
              }
            }}
            className='min-w-0 gap-4'
          >
            <TabsList variant='line' aria-label={t('Subscription operations')}>
              <TabsTrigger value='devin'>{t('Devin')}</TabsTrigger>
              <TabsTrigger value='coding-plan'>{t('Coding Plan')}</TabsTrigger>
            </TabsList>
            <TabsContent value='coding-plan'>
              <DataTableToolbar
                table={table}
                hideViewOptions
                className='mb-3'
                searchPlaceholder={t('Search name or account...')}
                filters={PLATFORM_FILTERS.map((filter) => ({
                  ...filter,
                  title: t('Platform'),
                }))}
                preActions={
                  <SubscriptionSortControl
                    options={sortOptions}
                    value={sortBy}
                    descending={sortDesc}
                    onChange={setSortBy}
                    onToggle={() => setSortDesc(!sortDesc)}
                  />
                }
              />
              {subscriptions.isLoading && (
                <div
                  role='status'
                  aria-label={t('Loading subscriptions')}
                  className='space-y-4'
                >
                  <Skeleton className='h-44 w-full' />
                  <Skeleton className='h-44 w-full' />
                </div>
              )}
              {subscriptions.error && (
                <ErrorState
                  description={t('Failed to load subscriptions')}
                  onRetry={() => void subscriptions.refetch()}
                />
              )}
              {subscriptions.data?.length === 0 && (
                <EmptyState
                  icon={CreditCard}
                  title={t('Connect your first subscription')}
                  description={t(
                    'Add OpenCode Go or sign in to OpenDesign to see all available allowance windows here.'
                  )}
                  action={
                    <Button onClick={create}>
                      <Plus className='size-4' aria-hidden='true' />
                      {t('Create subscription')}
                    </Button>
                  }
                />
              )}
              {subscriptions.data &&
                subscriptions.data.length > 0 &&
                visibleSubscriptions.length === 0 && (
                  <EmptyState
                    title={t('No matching subscriptions')}
                    description={t('Try another name or platform.')}
                    action={
                      <Button
                        variant='outline'
                        onClick={() => {
                          table.resetColumnFilters()
                          table.setGlobalFilter('')
                        }}
                      >
                        {t('Clear filters')}
                      </Button>
                    }
                  />
                )}
              {visibleSubscriptions.length > 0 && (
                <ul
                  className='bg-card divide-y overflow-hidden rounded-lg border'
                  aria-label={t('Subscriptions')}
                >
                  {visibleSubscriptions.map((subscription) => (
                    <SubscriptionRow
                      key={subscription.id}
                      subscription={subscription}
                      now={now}
                      onEdit={edit}
                      onRemove={setRemoving}
                    />
                  ))}
                </ul>
              )}
            </TabsContent>
            <TabsContent value='devin'>
              <DeviMonPage sort={devinSort} onSortChange={setDevinSort} />
            </TabsContent>
          </Tabs>
        </SectionPageLayout.Content>
      </SectionPageLayout>
      <SubscriptionFormDialog
        open={formOpen}
        subscription={editing}
        onOpenChange={setFormOpen}
        onSaved={() =>
          void queryClient.invalidateQueries({ queryKey: QUERY_KEY })
        }
      />
      <ConfirmDialog
        open={removing !== null}
        onOpenChange={(open) => {
          if (!open) setRemoving(null)
        }}
        title={t('Remove subscription')}
        desc={t(
          'Remove this saved connection and quota snapshot? Your subscription on the platform will stay active.'
        )}
        confirmText={t('Remove')}
        destructive
        isLoading={remove.isPending}
        handleConfirm={() => {
          if (removing) remove.mutate(removing.id)
        }}
      />
    </>
  )
}
