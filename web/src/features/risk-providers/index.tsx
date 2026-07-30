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
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { ConfirmDialog } from '@/components/confirm-dialog'
import { SectionPageLayout } from '@/components/layout'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { LocalRuleManager } from '@/features/risk-policy/components/local-rule-manager'
import { RiskPolicySettings } from '@/features/risk-policy/components/risk-policy-settings'
import { RiskRecordGovernanceSettings } from '@/features/risk-records/components/risk-record-governance-settings'
import { RiskRecordList } from '@/features/risk-records/components/risk-record-list'

import {
  deleteRiskProvider,
  listRiskProviders,
  validateRiskProvider,
} from './api'
import { RiskProviderFormDialog } from './components/risk-provider-form-dialog'
import {
  RiskProviderList,
  type RiskProviderPendingAction,
} from './components/risk-provider-list'
import { RiskProviderValidationDialog } from './components/risk-provider-validation-dialog'
import type { RiskProvider } from './types'

const QUERY_KEY = ['risk', 'providers'] as const

type RiskCenterTab = 'records' | 'configuration'
type RiskProviderAction =
  | { readonly kind: 'validate'; readonly text: string }
  | { readonly kind: 'delete' }

function assertNever(action: never): never {
  throw new Error(`Unsupported provider action: ${action}`)
}

export function RiskProviders() {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const [activeTab, setActiveTab] = useState<RiskCenterTab>('records')
  const [formOpen, setFormOpen] = useState(false)
  const [editingProvider, setEditingProvider] = useState<RiskProvider | null>(
    null
  )
  const [deletingProvider, setDeletingProvider] = useState<RiskProvider | null>(
    null
  )
  const [validatingProvider, setValidatingProvider] =
    useState<RiskProvider | null>(null)
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
    action: RiskProviderAction
  ): Promise<boolean> {
    setPendingProviderId(provider.id)
    setPendingAction(action.kind)
    try {
      switch (action.kind) {
        case 'validate': {
          const response = await validateRiskProvider(provider.id, {
            text: action.text,
          })
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
      return true
    } catch (error) {
      toast.error(error instanceof Error ? error.message : t('Request failed'))
      return false
    } finally {
      if (action.kind === 'validate') {
        await queryClient.invalidateQueries({ queryKey: ['risk', 'records'] })
      }
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
      <SectionPageLayout fixedContent>
        <SectionPageLayout.Title>{t('Risk Center')}</SectionPageLayout.Title>
        <SectionPageLayout.Content>
          <Tabs
            className='h-full min-h-0 min-w-0 overflow-hidden'
            value={activeTab}
            onValueChange={(value) => {
              if (value === 'configuration' || value === 'records') {
                setActiveTab(value)
              }
            }}
          >
            <TabsList>
              <TabsTrigger value='records'>{t('Risk records')}</TabsTrigger>
              <TabsTrigger value='configuration'>
                {t('Risk Configuration')}
              </TabsTrigger>
            </TabsList>
            <TabsContent
              value='configuration'
              className='mt-2 min-h-0 min-w-0 overflow-x-hidden overflow-y-auto'
            >
              <div className='max-w-full min-w-0 space-y-4 pb-1'>
                <RiskPolicySettings
                  providers={providersQuery.data ?? []}
                  onSaved={() => void providersQuery.refetch()}
                />
                <RiskRecordGovernanceSettings />
                <LocalRuleManager />
                <RiskProviderList
                  providers={providersQuery.data ?? []}
                  isLoading={providersQuery.isLoading}
                  error={providersQuery.error}
                  pendingProviderId={pendingProviderId}
                  pendingAction={pendingAction}
                  onRetry={() => void providersQuery.refetch()}
                  onRefresh={() => void providersQuery.refetch()}
                  isRefreshing={providersQuery.isFetching}
                  onCreate={openCreateDialog}
                  onEdit={openEditDialog}
                  onValidate={setValidatingProvider}
                  onDelete={setDeletingProvider}
                />
              </div>
            </TabsContent>
            <TabsContent value='records' className='mt-2 min-h-0'>
              <RiskRecordList providers={providersQuery.data ?? []} />
            </TabsContent>
          </Tabs>
        </SectionPageLayout.Content>
      </SectionPageLayout>
      <RiskProviderFormDialog
        open={formOpen}
        provider={editingProvider}
        onOpenChange={setFormOpen}
        onSaved={() => void providersQuery.refetch()}
      />
      <RiskProviderValidationDialog
        open={validatingProvider !== null}
        provider={validatingProvider}
        pending={pendingAction === 'validate'}
        onOpenChange={(open) => !open && setValidatingProvider(null)}
        onSubmit={(text) =>
          validatingProvider
            ? runProviderAction(validatingProvider, { kind: 'validate', text })
            : Promise.resolve(false)
        }
      />
      <ConfirmDialog
        open={deletingProvider !== null}
        onOpenChange={(open) => !open && setDeletingProvider(null)}
        title={t('Delete provider')}
        desc={t(
          'This removes the provider configuration and disables its system-managed token when present.'
        )}
        destructive
        isLoading={pendingAction === 'delete'}
        handleConfirm={() => {
          if (deletingProvider) {
            void runProviderAction(deletingProvider, { kind: 'delete' })
          }
        }}
      />
    </>
  )
}
