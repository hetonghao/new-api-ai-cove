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
import { useEffect } from 'react'
import { FormProvider, useForm } from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { FieldError } from '@/components/ui/field'
import { getRiskPolicyChannels } from '@/features/risk-policy/api'

import { createRiskProvider, updateRiskProvider } from '../api'
import {
  formValuesToPayload,
  getRiskProviderFormSchema,
  getRiskProviderServerFormError,
  providerToFormValues,
  RISK_PROVIDER_DEFAULT_VALUES,
} from '../lib/risk-provider-form'
import type { RiskProvider, RiskProviderFormValues } from '../types'
import { RiskProviderFormFields } from './risk-provider-form-fields'

type RiskProviderFormDialogProps = {
  readonly open: boolean
  readonly provider: RiskProvider | null
  readonly onOpenChange: (open: boolean) => void
  readonly onSaved: () => void
}

export function RiskProviderFormDialog(props: RiskProviderFormDialogProps) {
  const { t } = useTranslation()
  const credentialRequired = !props.provider?.has_credential
  const form = useForm<RiskProviderFormValues>({
    resolver: zodResolver(getRiskProviderFormSchema(t, credentialRequired)),
    defaultValues: RISK_PROVIDER_DEFAULT_VALUES,
  })
  const channelsQuery = useQuery({
    queryKey: ['risk', 'provider-form', 'channels'],
    queryFn: getRiskPolicyChannels,
    enabled: props.open,
    retry: false,
  })
  const enabledChannels = (channelsQuery.data ?? []).filter(
    (channel) => channel.status === 1
  )
  const providerType = form.watch('provider_type')

  useEffect(() => {
    if (!props.open) return
    form.reset(
      props.provider
        ? providerToFormValues(props.provider)
        : RISK_PROVIDER_DEFAULT_VALUES
    )
  }, [props.open, props.provider, form])

  async function handleSubmit(values: RiskProviderFormValues) {
    try {
      const payload = formValuesToPayload(values)
      const response = props.provider
        ? await updateRiskProvider(props.provider.id, payload)
        : await createRiskProvider(payload)
      if (!response.success) {
        const formError = getRiskProviderServerFormError(response.message)
        form.setError(formError.name, formError.error)
        toast.error(formError.error.message)
        return
      }
      toast.success(
        props.provider ? t('Provider updated') : t('Provider created')
      )
      props.onOpenChange(false)
      props.onSaved()
    } catch (error) {
      const message =
        error instanceof Error ? error.message : t('Request failed')
      const formError = getRiskProviderServerFormError(message)
      form.setError(formError.name, formError.error)
      toast.error(formError.error.message)
    }
  }

  const isSaving = form.formState.isSubmitting

  return (
    <Dialog open={props.open} onOpenChange={props.onOpenChange}>
      <DialogContent className='max-h-[calc(100dvh-1.5rem)] grid-rows-[auto_minmax(0,1fr)] overflow-hidden sm:max-w-2xl'>
        <DialogHeader>
          <DialogTitle>
            {props.provider ? t('Edit provider') : t('Add provider')}
          </DialogTitle>
          <DialogDescription>
            {providerType === 'platform_internal'
              ? t(
                  'Internal review calls this New API instance through 127.0.0.1.'
                )
              : t(
                  'Credentials are encrypted by the server and are never returned to this page.'
                )}
          </DialogDescription>
        </DialogHeader>
        <FormProvider {...form}>
          <form
            onSubmit={form.handleSubmit(handleSubmit)}
            className='grid min-h-0 grid-rows-[minmax(0,1fr)_auto] gap-4'
          >
            <div className='min-h-0 overflow-y-auto pr-1'>
              <RiskProviderFormFields
                hasCredential={Boolean(props.provider?.has_credential)}
                channels={enabledChannels}
                channelsLoading={channelsQuery.isLoading}
                channelsError={channelsQuery.isError}
              />
              <FieldError errors={[form.formState.errors.root?.server]} />
            </div>
            <DialogFooter>
              <Button
                type='button'
                variant='outline'
                onClick={() => props.onOpenChange(false)}
                disabled={isSaving}
              >
                {t('Cancel')}
              </Button>
              <Button type='submit' disabled={isSaving}>
                {isSaving ? t('Saving...') : t('Save')}
              </Button>
            </DialogFooter>
          </form>
        </FormProvider>
      </DialogContent>
    </Dialog>
  )
}
