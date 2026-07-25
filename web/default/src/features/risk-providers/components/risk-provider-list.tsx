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
import { Plus, ShieldCheck } from 'lucide-react'
import { useTranslation } from 'react-i18next'

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

import type { RiskProvider } from '../types'
import { RiskProviderCard } from './risk-provider-card'

const SKELETON_KEYS = ['risk-provider-1', 'risk-provider-2'] as const

export type RiskProviderPendingAction = 'validate' | 'activate' | 'delete'

type RiskProviderListProps = {
  readonly providers: readonly RiskProvider[]
  readonly isLoading: boolean
  readonly error: unknown
  readonly pendingProviderId: number | null
  readonly pendingAction: RiskProviderPendingAction | null
  readonly onRetry: () => void
  readonly onCreate: () => void
  readonly onEdit: (provider: RiskProvider) => void
  readonly onValidate: (provider: RiskProvider) => void
  readonly onActivate: (provider: RiskProvider) => void
  readonly onDelete: (provider: RiskProvider) => void
}

export function RiskProviderList(props: RiskProviderListProps) {
  const { t } = useTranslation()
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
    content = props.providers.length ? (
      <div className='grid gap-4 lg:grid-cols-2'>
        {props.providers.map((provider) => (
          <RiskProviderCard
            key={provider.id}
            provider={provider}
            pendingAction={
              props.pendingProviderId === provider.id
                ? props.pendingAction
                : null
            }
            onEdit={props.onEdit}
            onValidate={props.onValidate}
            onActivate={props.onActivate}
            onDelete={props.onDelete}
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
          <Button size='sm' onClick={props.onCreate}>
            <Plus className='size-4' />
            {t('Add provider')}
          </Button>
        </EmptyContent>
      </Empty>
    )
  }

  return (
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
  )
}
