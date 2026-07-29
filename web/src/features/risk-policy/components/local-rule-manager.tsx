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
import { ListFilter, Plus } from 'lucide-react'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { ConfirmDialog } from '@/components/confirm-dialog'
import { ErrorState } from '@/components/error-state'
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
  deleteLocalRiskRule,
  listLocalRiskRules,
  updateLocalRiskRule,
} from '../api'
import type { LocalRiskRule } from '../types'
import { LocalRuleCard } from './local-rule-card'
import { LocalRuleFormDialog } from './local-rule-form-dialog'
import { LocalRuleTestDialog } from './local-rule-test-dialog'

const QUERY_KEY = ['risk', 'rules'] as const
const SKELETON_KEYS = ['risk-rule-1', 'risk-rule-2'] as const

export function LocalRuleManager() {
  const { t } = useTranslation()
  const [formOpen, setFormOpen] = useState(false)
  const [editingRule, setEditingRule] = useState<LocalRiskRule | null>(null)
  const [testingRule, setTestingRule] = useState<LocalRiskRule | null>(null)
  const [deletingRule, setDeletingRule] = useState<LocalRiskRule | null>(null)
  const [pendingRuleId, setPendingRuleId] = useState<number | null>(null)

  const rulesQuery = useQuery({
    queryKey: QUERY_KEY,
    queryFn: async () => {
      const response = await listLocalRiskRules()
      if (!response.success || !response.data) {
        throw new Error(response.message || t('Failed to load local rules'))
      }
      return response.data
    },
    retry: false,
  })

  function openCreateDialog() {
    setEditingRule(null)
    setFormOpen(true)
  }

  function openEditDialog(rule: LocalRiskRule) {
    setEditingRule(rule)
    setFormOpen(true)
  }

  async function handleToggle(rule: LocalRiskRule, enabled: boolean) {
    setPendingRuleId(rule.id)
    try {
      const response = await updateLocalRiskRule(rule.id, {
        rule_type: rule.rule_type,
        pattern: rule.pattern,
        action: rule.action,
        enabled,
      })
      if (!response.success) throw new Error(response.message)
      toast.success(enabled ? t('Rule enabled') : t('Rule disabled'))
      await rulesQuery.refetch()
    } catch (error) {
      toast.error(error instanceof Error ? error.message : t('Request failed'))
    } finally {
      setPendingRuleId(null)
    }
  }

  async function handleDelete() {
    if (!deletingRule) return
    setPendingRuleId(deletingRule.id)
    try {
      const response = await deleteLocalRiskRule(deletingRule.id)
      if (!response.success) throw new Error(response.message)
      toast.success(t('Rule deleted'))
      setDeletingRule(null)
      await rulesQuery.refetch()
    } catch (error) {
      toast.error(error instanceof Error ? error.message : t('Request failed'))
    } finally {
      setPendingRuleId(null)
    }
  }

  let content = (
    <div className='grid gap-3 lg:grid-cols-2'>
      {SKELETON_KEYS.map((key) => (
        <Skeleton key={key} className='h-40 rounded-xl' />
      ))}
    </div>
  )
  if (rulesQuery.isError) {
    content = (
      <ErrorState
        title={t('Failed to load local rules')}
        description={
          rulesQuery.error instanceof Error
            ? rulesQuery.error.message
            : t('Request failed')
        }
        onRetry={() => rulesQuery.refetch()}
      />
    )
  } else if (!rulesQuery.isLoading) {
    const rules = rulesQuery.data ?? []
    content = rules.length ? (
      <div className='grid gap-3 lg:grid-cols-2'>
        {rules.map((rule) => (
          <LocalRuleCard
            key={rule.id}
            rule={rule}
            isPending={pendingRuleId === rule.id}
            onToggle={(selected, enabled) =>
              void handleToggle(selected, enabled)
            }
            onEdit={openEditDialog}
            onTest={setTestingRule}
            onDelete={setDeletingRule}
          />
        ))}
      </div>
    ) : (
      <Empty className='min-h-60 border'>
        <EmptyHeader>
          <EmptyMedia variant='icon'>
            <ListFilter />
          </EmptyMedia>
          <EmptyTitle>{t('No local risk rules')}</EmptyTitle>
          <EmptyDescription>
            {t(
              'Add keywords, phrases, or Go regular expressions that send matching content to or skip it from cloud review.'
            )}
          </EmptyDescription>
        </EmptyHeader>
        <EmptyContent>
          <Button size='sm' onClick={openCreateDialog}>
            <Plus className='size-4' />
            {t('Add local rule')}
          </Button>
        </EmptyContent>
      </Empty>
    )
  }

  return (
    <>
      <TitledCard
        title={t('Local risk trigger rules')}
        description={t(
          'Rules run on normalized new user text and decide whether cloud review is sent or skipped.'
        )}
        icon={<ListFilter className='size-5' />}
        action={
          <Button size='sm' onClick={openCreateDialog}>
            <Plus className='size-4' />
            {t('Add local rule')}
          </Button>
        }
        disableHoverEffect
      >
        {content}
      </TitledCard>
      <LocalRuleFormDialog
        open={formOpen}
        rule={editingRule}
        onOpenChange={setFormOpen}
        onSaved={() => void rulesQuery.refetch()}
      />
      <LocalRuleTestDialog
        open={testingRule !== null}
        rule={testingRule}
        onOpenChange={(open) => !open && setTestingRule(null)}
      />
      <ConfirmDialog
        open={deletingRule !== null}
        onOpenChange={(open) => !open && setDeletingRule(null)}
        title={t('Delete local rule')}
        desc={t('This permanently removes the local risk trigger rule.')}
        destructive
        isLoading={pendingRuleId === deletingRule?.id}
        handleConfirm={() => void handleDelete()}
      />
    </>
  )
}
