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
import { Plus, RefreshCw } from 'lucide-react'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { ConfirmDialog } from '@/components/confirm-dialog'
import { SectionPageLayout } from '@/components/layout'
import { Button } from '@/components/ui/button'
import { LocalRuleManager } from '@/features/risk-policy/components/local-rule-manager'
import { RiskPolicySettings } from '@/features/risk-policy/components/risk-policy-settings'

import {
  activateRiskProvider,
  deleteRiskProvider,
  listRiskProviders,
  validateRiskProvider,
} from './api'
import { RiskProviderFormDialog } from './components/risk-provider-form-dialog'
import {
  RiskProviderList,
  type RiskProviderPendingAction,
} from './components/risk-provider-list'
import type { RiskProvider } from './types'

const QUERY_KEY = ['risk', 'providers'] as const

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
  const [pendingAction, setPendingAction] =
    useState<RiskProviderPendingAction | null>(null)

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
    action: RiskProviderPendingAction
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
            {t('Refresh providers')}
          </Button>
          <Button size='sm' onClick={openCreateDialog}>
            <Plus className='size-4' />
            {t('Add provider')}
          </Button>
        </SectionPageLayout.Actions>
        <SectionPageLayout.Content>
          <div className='space-y-4'>
            <RiskPolicySettings
              providers={providersQuery.data ?? []}
              onSaved={() => void providersQuery.refetch()}
            />
            <LocalRuleManager />
            <RiskProviderList
              providers={providersQuery.data ?? []}
              isLoading={providersQuery.isLoading}
              error={providersQuery.error}
              pendingProviderId={pendingProviderId}
              pendingAction={pendingAction}
              onRetry={() => void providersQuery.refetch()}
              onCreate={openCreateDialog}
              onEdit={openEditDialog}
              onValidate={(provider) =>
                void runProviderAction(provider, 'validate')
              }
              onActivate={(provider) =>
                void runProviderAction(provider, 'activate')
              }
              onDelete={setDeletingProvider}
            />
          </div>
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
