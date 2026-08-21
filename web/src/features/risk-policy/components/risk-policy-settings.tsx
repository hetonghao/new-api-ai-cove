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
import { zodResolver } from '@hookform/resolvers/zod'
import { useQuery } from '@tanstack/react-query'
import { ShieldCheck } from 'lucide-react'
import { useEffect, useMemo } from 'react'
import { FormProvider, useForm } from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { ErrorState } from '@/components/error-state'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Skeleton } from '@/components/ui/skeleton'
import { Switch } from '@/components/ui/switch'
import { TitledCard } from '@/components/ui/titled-card'
import type { RiskProvider } from '@/features/risk-providers/types'
import {
  getOptionValue,
  useSystemOptions,
} from '@/features/system-settings/hooks/use-system-options'
import { useUpdateOption } from '@/features/system-settings/hooks/use-update-option'

import {
  getRiskPolicy,
  getRiskPolicyChannels,
  getRiskPolicyModels,
  getRiskPolicyUsers,
  updateRiskPolicy,
} from '../api'
import {
  createRiskPolicyFormSchema,
  riskPolicyFormValuesToPayload,
  riskPolicyToFormValues,
  type RiskPolicyFormValues,
} from '../lib/risk-policy-form'
import { RiskPolicyFormFields } from './risk-policy-form-fields'

const QUERY_KEY = ['risk', 'policy'] as const
const CHANNELS_QUERY_KEY = ['risk', 'policy', 'channels'] as const
const USERS_QUERY_KEY = ['risk', 'policy', 'users'] as const
const MODELS_QUERY_KEY = ['risk', 'policy', 'models'] as const
const SEVERE_RISK_OPTION_KEY = 'SevereRiskAutoQuarantineEnabled'
const SEVERE_RISK_OPTION_DEFAULTS = {
  [SEVERE_RISK_OPTION_KEY]: true,
}
const DEFAULT_VALUES: RiskPolicyFormValues = {
  enabled: false,
  enabled_channels: [],
  excluded_user_ids: [],
  excluded_models: [],
  non_blocking_categories: [],
  review_mode: 'selective',
  action_mode: 'observe',
}

type RiskPolicySettingsProps = {
  readonly providers: readonly RiskProvider[]
  readonly onSaved: () => void
}

export function RiskPolicySettings(props: RiskPolicySettingsProps) {
  const { t } = useTranslation()
  const systemOptionsQuery = useSystemOptions()
  const updateOption = useUpdateOption()
  const severeRiskOptions = getOptionValue(
    systemOptionsQuery.data?.data,
    SEVERE_RISK_OPTION_DEFAULTS
  )
  const validatedProviders = useMemo(
    () => props.providers.filter((provider) => provider.validated_at !== null),
    [props.providers]
  )
  const form = useForm<RiskPolicyFormValues>({
    resolver: zodResolver(createRiskPolicyFormSchema(t)),
    defaultValues: DEFAULT_VALUES,
  })

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
  const channelsQuery = useQuery({
    queryKey: CHANNELS_QUERY_KEY,
    queryFn: getRiskPolicyChannels,
    retry: false,
  })
  const usersQuery = useQuery({
    queryKey: USERS_QUERY_KEY,
    queryFn: getRiskPolicyUsers,
    retry: false,
  })
  const modelsQuery = useQuery({
    queryKey: MODELS_QUERY_KEY,
    queryFn: getRiskPolicyModels,
    retry: false,
  })

  useEffect(() => {
    if (policyQuery.data) {
      form.reset(riskPolicyToFormValues(policyQuery.data))
    }
  }, [form, policyQuery.data])

  async function handleSubmit(values: RiskPolicyFormValues) {
    form.clearErrors('root.server')
    try {
      const response = await updateRiskPolicy(
        riskPolicyFormValuesToPayload(values)
      )
      if (!response.success || !response.data) {
        form.setError('root.server', {
          type: 'server',
          message: response.message || t('Request failed'),
        })
        return
      }
      form.reset(riskPolicyToFormValues(response.data))
      toast.success(t('Risk policy saved'))
      props.onSaved()
      await policyQuery.refetch()
    } catch (error) {
      form.setError('root.server', {
        type: 'server',
        message: error instanceof Error ? error.message : t('Request failed'),
      })
    }
  }

  const queryError =
    policyQuery.error ??
    channelsQuery.error ??
    usersQuery.error ??
    modelsQuery.error
  let queryErrorTitle = t('Failed to load enabled models')
  if (usersQuery.isError) {
    queryErrorTitle = t('Failed to load users')
  }
  if (channelsQuery.isError) {
    queryErrorTitle = t('Failed to load channels')
  }
  if (policyQuery.isError) {
    queryErrorTitle = t('Failed to load risk policy')
  }
  let content = <Skeleton className='h-72 rounded-xl' />
  if (
    policyQuery.isError ||
    channelsQuery.isError ||
    usersQuery.isError ||
    modelsQuery.isError
  ) {
    content = (
      <ErrorState
        title={queryErrorTitle}
        description={
          queryError instanceof Error ? queryError.message : t('Request failed')
        }
        onRetry={() => {
          void policyQuery.refetch()
          void channelsQuery.refetch()
          void usersQuery.refetch()
          void modelsQuery.refetch()
        }}
      />
    )
  } else if (
    !policyQuery.isLoading &&
    !channelsQuery.isLoading &&
    !usersQuery.isLoading &&
    !modelsQuery.isLoading
  ) {
    content = (
      <FormProvider {...form}>
        <form onSubmit={form.handleSubmit(handleSubmit)} className='space-y-5'>
          <RiskPolicyFormFields
            availableProviderCount={
              validatedProviders.filter((provider) => provider.active).length
            }
            channels={channelsQuery.data ?? []}
            users={usersQuery.data ?? []}
            models={modelsQuery.data ?? []}
          />
          {form.formState.errors.root?.server?.message ? (
            <Alert variant='destructive'>
              <AlertTitle>{t('Failed to save')}</AlertTitle>
              <AlertDescription>
                {form.formState.errors.root.server.message}
              </AlertDescription>
            </Alert>
          ) : null}
          <div className='flex justify-end'>
            <Button type='submit' disabled={form.formState.isSubmitting}>
              {form.formState.isSubmitting ? t('Saving...') : t('Save policy')}
            </Button>
          </div>
        </form>
      </FormProvider>
    )
  }

  return (
    <TitledCard
      title={t('Global risk policy')}
      description={t(
        'All enabled risk channels share one review scope and decision action.'
      )}
      descriptionClassName='text-balance'
      icon={<ShieldCheck className='size-5' />}
      action={
        <Badge variant={policyQuery.data?.enabled ? 'default' : 'outline'}>
          {policyQuery.data?.enabled ? t('Enabled') : t('Disabled')}
        </Badge>
      }
      disableHoverEffect
    >
      <div className='space-y-5'>
        <div className='bg-muted/20 flex items-center justify-between gap-4 rounded-xl border px-3 py-2.5'>
          <div className='min-w-0 space-y-0.5'>
            <label
              className='text-sm leading-snug font-medium'
              htmlFor='severe-risk-auto-quarantine'
            >
              {t('Enable severe risk auto-quarantine')}
            </label>
            <p className='break-keep text-muted-foreground text-pretty text-xs'>
              {t(
                'Automatically disable the affected user and upstream channel when invalid_prompt is confirmed.'
              )}
            </p>
          </div>
          <Switch
            id='severe-risk-auto-quarantine'
            checked={severeRiskOptions[SEVERE_RISK_OPTION_KEY]}
            disabled={systemOptionsQuery.isLoading || updateOption.isPending}
            onCheckedChange={(checked) => {
              updateOption.mutate({
                key: SEVERE_RISK_OPTION_KEY,
                value: checked,
              })
            }}
          />
        </div>
        {content}
      </div>
    </TitledCard>
  )
}
