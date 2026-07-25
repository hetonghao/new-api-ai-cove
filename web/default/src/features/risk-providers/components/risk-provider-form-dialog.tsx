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
import { useEffect, useState, type FormEvent } from 'react'
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

import { createRiskProvider, updateRiskProvider } from '../api'
import {
  formValuesToPayload,
  getRiskProviderFormSchema,
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
  const [values, setValues] = useState<RiskProviderFormValues>(
    RISK_PROVIDER_DEFAULT_VALUES
  )
  const [isSaving, setIsSaving] = useState(false)

  useEffect(() => {
    if (!props.open) return
    setValues(
      props.provider
        ? providerToFormValues(props.provider)
        : RISK_PROVIDER_DEFAULT_VALUES
    )
  }, [props.open, props.provider])

  async function handleSubmit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    const credentialRequired = !props.provider?.has_credential
    const parsed = getRiskProviderFormSchema(t, credentialRequired).safeParse(
      values
    )
    if (!parsed.success) {
      toast.error(parsed.error.issues[0]?.message ?? t('Invalid configuration'))
      return
    }

    setIsSaving(true)
    try {
      const payload = formValuesToPayload(parsed.data)
      const response = props.provider
        ? await updateRiskProvider(props.provider.id, payload)
        : await createRiskProvider(payload)
      if (!response.success) {
        throw new Error(response.message)
      }
      toast.success(
        props.provider ? t('Provider updated') : t('Provider created')
      )
      props.onOpenChange(false)
      props.onSaved()
    } catch (error) {
      toast.error(error instanceof Error ? error.message : t('Request failed'))
    } finally {
      setIsSaving(false)
    }
  }

  return (
    <Dialog open={props.open} onOpenChange={props.onOpenChange}>
      <DialogContent className='max-h-[calc(100dvh-1.5rem)] grid-rows-[auto_minmax(0,1fr)] overflow-hidden sm:max-w-2xl'>
        <DialogHeader>
          <DialogTitle>
            {props.provider ? t('Edit provider') : t('Add provider')}
          </DialogTitle>
          <DialogDescription>
            {t(
              'Credentials are encrypted by the server and are never returned to this page.'
            )}
          </DialogDescription>
        </DialogHeader>
        <form
          onSubmit={handleSubmit}
          className='grid min-h-0 grid-rows-[minmax(0,1fr)_auto] gap-4'
        >
          <div className='min-h-0 overflow-y-auto pr-1'>
            <RiskProviderFormFields
              values={values}
              hasCredential={Boolean(props.provider?.has_credential)}
              onChange={setValues}
            />
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
      </DialogContent>
    </Dialog>
  )
}
