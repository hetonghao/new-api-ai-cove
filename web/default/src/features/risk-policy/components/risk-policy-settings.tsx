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
import { ShieldCheck } from 'lucide-react'
import { useEffect, useMemo, useState, type FormEvent } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { ErrorState } from '@/components/error-state'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Skeleton } from '@/components/ui/skeleton'
import { TitledCard } from '@/components/ui/titled-card'
import type { RiskProvider } from '@/features/risk-providers/types'

import { getRiskPolicy, updateRiskPolicy } from '../api'
import {
  createRiskPolicyFormSchema,
  riskPolicyFormValuesToPayload,
  riskPolicyToFormValues,
  type RiskPolicyFormValues,
} from '../lib/risk-policy-form'
import { RiskPolicyFormFields } from './risk-policy-form-fields'

const QUERY_KEY = ['risk', 'policy'] as const
const DEFAULT_VALUES: RiskPolicyFormValues = {
  enabled: false,
  provider_id: '',
  review_mode: 'selective',
  action_mode: 'observe',
}

type RiskPolicySettingsProps = {
  readonly providers: readonly RiskProvider[]
  readonly onSaved: () => void
}

export function RiskPolicySettings(props: RiskPolicySettingsProps) {
  const { t } = useTranslation()
  const [values, setValues] = useState(DEFAULT_VALUES)
  const [isSaving, setIsSaving] = useState(false)
  const validatedProviders = useMemo(
    () => props.providers.filter((provider) => provider.validated_at !== null),
    [props.providers]
  )
  const validatedProviderIds = useMemo(
    () => validatedProviders.map((provider) => provider.id),
    [validatedProviders]
  )

  const policyQuery = useQuery({
    queryKey: QUERY_KEY,
    queryFn: async () => {
      const response = await getRiskPolicy()
      if (!response.success || !response.data) {
        throw new Error(response.message || t('Failed to load risk policy'))
      }
      return response.data
    },
    retry: false,
  })

  useEffect(() => {
    if (policyQuery.data) {
      setValues(riskPolicyToFormValues(policyQuery.data))
    }
  }, [policyQuery.data])

  async function handleSubmit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    const parsed = createRiskPolicyFormSchema(
      validatedProviderIds,
      t
    ).safeParse(values)
    if (!parsed.success) {
      toast.error(parsed.error.issues[0]?.message ?? t('Invalid configuration'))
      return
    }

    setIsSaving(true)
    try {
      const response = await updateRiskPolicy(
        riskPolicyFormValuesToPayload(parsed.data)
      )
      if (!response.success || !response.data) {
        throw new Error(response.message)
      }
      setValues(riskPolicyToFormValues(response.data))
      toast.success(t('Risk policy saved'))
      props.onSaved()
      await policyQuery.refetch()
    } catch (error) {
      toast.error(error instanceof Error ? error.message : t('Request failed'))
    } finally {
      setIsSaving(false)
    }
  }

  let content = <Skeleton className='h-72 rounded-xl' />
  if (policyQuery.isError) {
    content = (
      <ErrorState
        title={t('Failed to load risk policy')}
        description={
          policyQuery.error instanceof Error
            ? policyQuery.error.message
            : t('Request failed')
        }
        onRetry={() => policyQuery.refetch()}
      />
    )
  } else if (!policyQuery.isLoading) {
    content = (
      <form onSubmit={handleSubmit} className='space-y-5'>
        <RiskPolicyFormFields
          values={values}
          validatedProviders={validatedProviders}
          onChange={setValues}
        />
        <div className='flex justify-end'>
          <Button type='submit' disabled={isSaving}>
            {isSaving ? t('Saving...') : t('Save policy')}
          </Button>
        </div>
      </form>
    )
  }

  return (
    <TitledCard
      title={t('Global risk policy')}
      description={t(
        'All enabled risk channels share one provider, review scope, and decision action.'
      )}
      icon={<ShieldCheck className='size-5' />}
      action={
        <Badge variant={policyQuery.data?.enabled ? 'default' : 'outline'}>
          {policyQuery.data?.enabled ? t('Enabled') : t('Disabled')}
        </Badge>
      }
      disableHoverEffect
    >
      {content}
    </TitledCard>
  )
}
