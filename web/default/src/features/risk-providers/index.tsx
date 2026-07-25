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
import { useQuery } from '@tanstack/react-query'
import { Plus, RefreshCw, ShieldCheck } from 'lucide-react'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { ConfirmDialog } from '@/components/confirm-dialog'
import { ErrorState } from '@/components/error-state'
import { SectionPageLayout } from '@/components/layout'
import { Button } from '@/components/ui/button'
import {
  Empty,
  EmptyContent,
  EmptyDescription,
  EmptyHeader,
  EmptyMedia,
  EmptyTitle,
} from '@/components/ui/empty'
import { Skeleton } from '@/components/ui/skeleton'
import { TitledCard } from '@/components/ui/titled-card'

import {
  activateRiskProvider,
  deleteRiskProvider,
  listRiskProviders,
  validateRiskProvider,
} from './api'
import { RiskProviderCard } from './components/risk-provider-card'
import { RiskProviderFormDialog } from './components/risk-provider-form-dialog'
import type { RiskProvider } from './types'

const QUERY_KEY = ['risk', 'providers'] as const
const SKELETON_KEYS = ['risk-provider-1', 'risk-provider-2'] as const

type PendingAction = 'validate' | 'activate' | 'delete'

function assertNever(action: never): never {
  throw new Error(`Unsupported provider action: ${action}`)
}

export function RiskProviders() {
  const { t } = useTranslation()
  const [formOpen, setFormOpen] = useState(false)
  const [editingProvider, setEditingProvider] = useState<RiskProvider | null>(
    null
  )
  const [deletingProvider, setDeletingProvider] = useState<RiskProvider | null>(
    null
  )
  const [pendingProviderId, setPendingProviderId] = useState<number | null>(
    null
  )
  const [pendingAction, setPendingAction] = useState<PendingAction | null>(null)

  const providersQuery = useQuery({
    queryKey: QUERY_KEY,
    queryFn: async () => {
      const response = await listRiskProviders()
      if (!response.success || !response.data) {
        throw new Error(response.message || t('Failed to load providers'))
      }
      return response.data
    },
    retry: false,
  })

  async function runProviderAction(
    provider: RiskProvider,
    action: PendingAction
  ) {
    setPendingProviderId(provider.id)
    setPendingAction(action)
    try {
      switch (action) {
        case 'validate': {
          const response = await validateRiskProvider(provider.id)
          if (!response.success || !response.data) {
            throw new Error(response.message)
          }
          toast.success(
            t('Connection verified: {{status}}', {
              status: response.data.status,
            })
          )
          break
        }
        case 'activate': {
          const response = await activateRiskProvider(provider.id)
          if (!response.success) throw new Error(response.message)
          toast.success(t('Active provider updated'))
          break
        }
        case 'delete': {
          const response = await deleteRiskProvider(provider.id)
          if (!response.success) throw new Error(response.message)
          toast.success(t('Provider deleted'))
          setDeletingProvider(null)
          break
        }
        default:
          assertNever(action)
      }
      await providersQuery.refetch()
    } catch (error) {
      toast.error(error instanceof Error ? error.message : t('Request failed'))
    } finally {
      setPendingProviderId(null)
      setPendingAction(null)
    }
  }

  function openCreateDialog() {
    setEditingProvider(null)
    setFormOpen(true)
  }

  function openEditDialog(provider: RiskProvider) {
    setEditingProvider(provider)
    setFormOpen(true)
  }

  let content = (
    <div className='grid gap-4 lg:grid-cols-2'>
      {SKELETON_KEYS.map((key) => (
        <Skeleton key={key} className='h-72 rounded-xl' />
      ))}
    </div>
  )

  if (providersQuery.isError) {
    content = (
      <ErrorState
        title={t('Failed to load providers')}
        description={
          providersQuery.error instanceof Error
            ? providersQuery.error.message
            : t('Request failed')
        }
        onRetry={() => providersQuery.refetch()}
      />
    )
  } else if (!providersQuery.isLoading) {
    const providers = providersQuery.data ?? []
    content = providers.length ? (
      <div className='grid gap-4 lg:grid-cols-2'>
        {providers.map((provider) => (
          <RiskProviderCard
            key={provider.id}
            provider={provider}
            pendingAction={
              pendingProviderId === provider.id ? pendingAction : null
            }
            onEdit={openEditDialog}
            onValidate={(selected) =>
              void runProviderAction(selected, 'validate')
            }
            onActivate={(selected) =>
              void runProviderAction(selected, 'activate')
            }
            onDelete={setDeletingProvider}
          />
        ))}
      </div>
    ) : (
      <Empty className='min-h-72 border'>
        <EmptyHeader>
          <EmptyMedia variant='icon'>
            <ShieldCheck />
          </EmptyMedia>
          <EmptyTitle>{t('No cloud review providers')}</EmptyTitle>
          <EmptyDescription>
            {t('Add a provider, test its connection, then set it active.')}
          </EmptyDescription>
        </EmptyHeader>
        <EmptyContent>
          <Button size='sm' onClick={openCreateDialog}>
            <Plus className='size-4' />
            {t('Add provider')}
          </Button>
        </EmptyContent>
      </Empty>
    )
  }

  return (
    <>
      <SectionPageLayout>
        <SectionPageLayout.Title>{t('Risk Center')}</SectionPageLayout.Title>
        <SectionPageLayout.Actions>
          <Button
            size='sm'
            variant='outline'
            onClick={() => providersQuery.refetch()}
            disabled={providersQuery.isFetching}
          >
            <RefreshCw
              className={
                providersQuery.isFetching ? 'size-4 animate-spin' : 'size-4'
              }
            />
            {t('Refresh')}
          </Button>
          <Button size='sm' onClick={openCreateDialog}>
            <Plus className='size-4' />
            {t('Add provider')}
          </Button>
        </SectionPageLayout.Actions>
        <SectionPageLayout.Content>
          <TitledCard
            title={t('Cloud review providers')}
            description={t(
              'Manage encrypted credentials, connection checks, timeouts, and the single active provider.'
            )}
            descriptionClassName='text-pretty'
            icon={<ShieldCheck className='size-5' />}
            disableHoverEffect
          >
            {content}
          </TitledCard>
        </SectionPageLayout.Content>
      </SectionPageLayout>
      <RiskProviderFormDialog
        open={formOpen}
        provider={editingProvider}
        onOpenChange={setFormOpen}
        onSaved={() => void providersQuery.refetch()}
      />
      <ConfirmDialog
        open={deletingProvider !== null}
        onOpenChange={(open) => !open && setDeletingProvider(null)}
        title={t('Delete provider')}
        desc={t(
          'This removes the provider configuration. The encrypted credential cannot be recovered.'
        )}
        destructive
        isLoading={pendingAction === 'delete'}
        handleConfirm={() => {
          if (deletingProvider) {
            void runProviderAction(deletingProvider, 'delete')
          }
        }}
      />
    </>
  )
}
