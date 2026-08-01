/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as published by
the Free Software Foundation, either version 3 of the License, or
(at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.
*/
import type { ColumnDef } from '@tanstack/react-table'
import {
  Pencil,
  Plus,
  PlugZap,
  RefreshCw,
  ShieldCheck,
  Trash2,
} from 'lucide-react'
import { useMemo } from 'react'
import { useTranslation } from 'react-i18next'

import {
  DATA_TABLE_VIEW_MODES,
  DataTablePage,
  DataTableViewModeToggle,
  MobileCardList,
  useDataTable,
  useDataTableViewMode,
} from '@/components/data-table'
import { ErrorState } from '@/components/error-state'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Skeleton } from '@/components/ui/skeleton'
import { Switch } from '@/components/ui/switch'
import { TitledCard } from '@/components/ui/titled-card'
import { formatNumber } from '@/lib/format'

import type { RiskProvider } from '../types'
import { RiskProviderCard } from './risk-provider-card'

const VIEW_MODE_STORAGE_KEY = 'risk-providers-view-mode'
const SKELETON_KEYS = ['risk-provider-1', 'risk-provider-2'] as const

export type RiskProviderPendingAction = 'validate' | 'delete' | 'activate'

type RiskProviderListProps = {
  readonly providers: readonly RiskProvider[]
  readonly isLoading: boolean
  readonly error: unknown
  readonly pendingProviderId: number | null
  readonly pendingAction: RiskProviderPendingAction | null
  readonly onRetry: () => void
  readonly onRefresh: () => void
  readonly isRefreshing: boolean
  readonly onCreate: () => void
  readonly onEdit: (provider: RiskProvider) => void
  readonly onValidate: (provider: RiskProvider) => void
  readonly onDelete: (provider: RiskProvider) => void
  readonly onToggleActive: (provider: RiskProvider, active: boolean) => void
}

function currentStatusLabel(
  provider: RiskProvider,
  t: (key: string) => string
) {
  if (provider.current_status === 'circuit_open') return t('Circuit open')
  if (provider.current_status === 'daily_exhausted') {
    return t('Daily quota exhausted')
  }
  return t('Normal')
}

function currentStatusVariant(provider: RiskProvider) {
  if (provider.current_status === 'circuit_open') return 'warning' as const
  if (provider.current_status === 'daily_exhausted') {
    return 'destructive' as const
  }
  return 'secondary' as const
}

function ProviderActions(props: {
  readonly provider: RiskProvider
  readonly pendingAction: RiskProviderPendingAction | null
  readonly onEdit: (provider: RiskProvider) => void
  readonly onValidate: (provider: RiskProvider) => void
  readonly onDelete: (provider: RiskProvider) => void
}) {
  const { t } = useTranslation()
  const { provider } = props
  return (
    <div className='flex flex-wrap gap-1.5'>
      <Button
        size='sm'
        variant='ghost'
        onClick={() => props.onEdit(provider)}
        aria-label={t('Edit')}
      >
        <Pencil className='size-4' />
      </Button>
      <Button
        size='sm'
        variant='ghost'
        disabled={props.pendingAction !== null}
        onClick={() => props.onValidate(provider)}
        aria-label={t('Test connection')}
      >
        <PlugZap className='size-4' />
      </Button>
      <Button
        size='sm'
        variant='ghost'
        className='text-destructive hover:text-destructive'
        disabled={props.pendingAction !== null}
        onClick={() => props.onDelete(provider)}
        aria-label={t('Delete')}
      >
        <Trash2 className='size-4' />
      </Button>
    </div>
  )
}

export function RiskProviderList(props: RiskProviderListProps) {
  const { t } = useTranslation()
  const providers = useMemo(() => [...props.providers], [props.providers])
  const [viewMode, setViewMode] = useDataTableViewMode({
    storageKey: VIEW_MODE_STORAGE_KEY,
    defaultMode: DATA_TABLE_VIEW_MODES.TABLE,
  })

  const columns = useMemo<ColumnDef<RiskProvider, unknown>[]>(
    () => [
      {
        id: 'provider',
        header: t('Provider'),
        accessorKey: 'name',
        meta: { mobileTitle: true },
        cell: ({ row }) => (
          <div className='max-w-full min-w-0'>
            <div className='font-medium'>{row.original.name}</div>
            <div className='text-muted-foreground mt-0.5 text-xs'>
              {row.original.provider_type === 'cloudflare'
                ? 'Cloudflare Workers AI'
                : t('Platform internal model')}
            </div>
          </div>
        ),
      },
      {
        id: 'priority',
        header: t('Priority'),
        accessorKey: 'priority',
        meta: { mobileOrder: 10 },
        cell: ({ row }) => (
          <Badge variant='outline'>{row.original.priority}</Badge>
        ),
      },
      {
        id: 'active',
        header: t('Enabled status'),
        accessorKey: 'active',
        meta: { mobileOrder: 20 },
        cell: ({ row }) => (
          <div className='flex min-w-0 items-center gap-2'>
            <Switch
              checked={row.original.active}
              disabled={
                props.pendingProviderId === row.original.id ||
                row.original.validated_at === null
              }
              onCheckedChange={(active) =>
                props.onToggleActive(row.original, active)
              }
              aria-label={
                row.original.active
                  ? t('Disable provider')
                  : t('Enable provider')
              }
              size='sm'
            />
            <span className='text-xs'>
              {row.original.active ? t('Enabled') : t('Disabled')}
            </span>
          </div>
        ),
      },
      {
        id: 'current_status',
        header: t('Current status'),
        accessorKey: 'current_status',
        meta: { mobileBadge: true },
        cell: ({ row }) => (
          <Badge variant={currentStatusVariant(row.original)}>
            {currentStatusLabel(row.original, t)}
          </Badge>
        ),
      },
      {
        id: 'neurons',
        header: t('Daily Neurons'),
        meta: { mobileOrder: 30 },
        cell: ({ row }) =>
          row.original.provider_type === 'cloudflare' ? (
            <div className='max-w-full min-w-0'>
              <div className='tabular-nums'>
                {formatNumber(row.original.daily_neurons_used)} /{' '}
                {formatNumber(row.original.daily_neurons_limit)}
              </div>
              <div className='text-muted-foreground text-xs tabular-nums'>
                {formatNumber(row.original.daily_neurons_remaining)}{' '}
                {t('remaining')}
              </div>
            </div>
          ) : (
            t('Not applicable')
          ),
      },
      {
        id: 'model',
        header: t('Model'),
        accessorKey: 'model',
        meta: { mobileHidden: true },
        cell: ({ row }) => (
          <span
            className='block max-w-full min-w-0 truncate font-mono text-xs'
            title={row.original.model}
          >
            {row.original.model}
          </span>
        ),
      },
      {
        id: 'actions',
        header: t('Actions'),
        enableSorting: false,
        cell: ({ row }) => (
          <ProviderActions
            provider={row.original}
            pendingAction={
              props.pendingProviderId === row.original.id
                ? props.pendingAction
                : null
            }
            onEdit={props.onEdit}
            onValidate={props.onValidate}
            onDelete={props.onDelete}
          />
        ),
      },
    ],
    [props, t]
  )

  const { table } = useDataTable({
    data: providers,
    columns,
    pageCount: 1,
    initialPagination: { pageIndex: 0, pageSize: 1000 },
    withFilteredRowModel: false,
    withPaginationRowModel: true,
    withSortedRowModel: false,
    withFacetedRowModel: false,
  })

  let content = (
    <div className='grid gap-4 lg:grid-cols-2'>
      {SKELETON_KEYS.map((key) => (
        <Skeleton key={key} className='h-72 rounded-xl' />
      ))}
    </div>
  )

  if (props.error) {
    content = (
      <ErrorState
        title={t('Failed to load providers')}
        description={
          props.error instanceof Error
            ? props.error.message
            : t('Request failed')
        }
        onRetry={props.onRetry}
      />
    )
  } else if (!props.isLoading) {
    content = (
      <DataTablePage
        table={table}
        columns={columns}
        isLoading={false}
        isFetching={props.isRefreshing}
        emptyTitle={t('No cloud review providers')}
        emptyDescription={t(
          'Add a provider, test its connection, then enable it from this list.'
        )}
        emptyIcon={<ShieldCheck />}
        emptyAction={
          <Button size='sm' onClick={props.onCreate}>
            <Plus className='size-4' />
            {t('Add provider')}
          </Button>
        }
        toolbar={
          <div className='flex justify-end'>
            <DataTableViewModeToggle value={viewMode} onChange={setViewMode} />
          </div>
        }
        mobile={
          viewMode === DATA_TABLE_VIEW_MODES.TABLE ? (
            <MobileCardList
              table={table}
              isLoading={false}
              emptyTitle={t('No cloud review providers')}
              emptyDescription={t(
                'Add a provider, test its connection, then enable it from this list.'
              )}
            />
          ) : undefined
        }
        enableCardView
        viewMode={viewMode}
        onViewModeChange={setViewMode}
        viewModeStorageKey={VIEW_MODE_STORAGE_KEY}
        defaultViewMode={DATA_TABLE_VIEW_MODES.TABLE}
        renderCard={(row) => (
          <RiskProviderCard
            provider={row.original}
            pendingAction={
              props.pendingProviderId === row.original.id
                ? props.pendingAction
                : null
            }
            onEdit={props.onEdit}
            onValidate={props.onValidate}
            onDelete={props.onDelete}
            onToggleActive={props.onToggleActive}
          />
        )}
        cardGridClassName='grid grid-cols-1 gap-3 lg:grid-cols-2'
        showPagination={false}
        fixedHeight={false}
      />
    )
  }

  return (
    <TitledCard
      title={t('Cloud review providers')}
      description={t(
        'Verified and manually enabled providers are selected globally by priority, then rotated within the same priority.'
      )}
      descriptionClassName='text-pretty'
      icon={<ShieldCheck className='size-5' />}
      action={
        <div className='flex flex-wrap items-center justify-end gap-2'>
          <Button
            size='sm'
            variant='outline'
            onClick={props.onRefresh}
            disabled={props.isRefreshing}
          >
            <RefreshCw
              className={props.isRefreshing ? 'size-4 animate-spin' : 'size-4'}
            />
            {t('Refresh providers')}
          </Button>
          <Button size='sm' onClick={props.onCreate}>
            <Plus className='size-4' />
            {t('Add provider')}
          </Button>
        </div>
      }
      disableHoverEffect
    >
      {content}
    </TitledCard>
  )
}
