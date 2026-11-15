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
import { CreditCard, Plus, RefreshCw } from 'lucide-react'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { ConfirmDialog } from '@/components/confirm-dialog'
import { EmptyState } from '@/components/empty-state'
import { ErrorState } from '@/components/error-state'
import { SectionPageLayout } from '@/components/layout'
import { Button } from '@/components/ui/button'
import { Skeleton } from '@/components/ui/skeleton'

import { deleteUpstreamSubscription, listUpstreamSubscriptions } from './api'
import { SubscriptionFormDialog } from './components/subscription-form-dialog'
import { SubscriptionRow } from './components/subscription-row'
import type { UpstreamSubscription } from './types'

const QUERY_KEY = ['upstream-subscriptions'] as const

export function UpstreamSubscriptions() {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const [formOpen, setFormOpen] = useState(false)
  const [editing, setEditing] = useState<UpstreamSubscription | null>(null)
  const [removing, setRemoving] = useState<UpstreamSubscription | null>(null)
  const subscriptions = useQuery({
    queryKey: QUERY_KEY,
    queryFn: ({ signal }) => listUpstreamSubscriptions(signal),
    refetchInterval: 15_000,
    retry: false,
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

  return (
    <>
      <SectionPageLayout>
        <SectionPageLayout.Title>
          {t('Subscription operations')}
        </SectionPageLayout.Title>
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
        <SectionPageLayout.Content>
          <p className='text-muted-foreground mb-4 text-sm leading-relaxed'>
            {t(
              'Quota refresh intervals are configurable and default to 10 minutes. Each bar uses its own window limit.'
            )}
          </p>
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
          {subscriptions.data && subscriptions.data.length > 0 && (
            <ul
              className='bg-card divide-y overflow-hidden rounded-lg border'
              aria-label={t('Subscriptions')}
            >
              {subscriptions.data.map((subscription) => (
                <SubscriptionRow
                  key={subscription.id}
                  subscription={subscription}
                  onEdit={edit}
                  onRemove={setRemoving}
                />
              ))}
            </ul>
          )}
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
