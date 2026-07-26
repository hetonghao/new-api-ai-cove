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

import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { Field, FieldDescription, FieldLabel } from '@/components/ui/field'
import { Textarea } from '@/components/ui/textarea'

import type { RiskProvider } from '../types'

export const RISK_PROVIDER_VALIDATION_DEFAULT_TEXT =
  'AI Cove provider connection test'

type RiskProviderValidationDialogProps = {
  readonly open: boolean
  readonly provider: RiskProvider | null
  readonly pending: boolean
  readonly onOpenChange: (open: boolean) => void
  readonly onSubmit: (text: string) => Promise<boolean>
}

export function RiskProviderValidationDialog(
  props: RiskProviderValidationDialogProps
) {
  const { t } = useTranslation()
  const [text, setText] = useState(RISK_PROVIDER_VALIDATION_DEFAULT_TEXT)

  useEffect(() => {
    if (props.open) setText(RISK_PROVIDER_VALIDATION_DEFAULT_TEXT)
  }, [props.open, props.provider?.id])

  const trimmedText = text.trim()
  const textLength = [...trimmedText].length
  const valid = textLength >= 1 && textLength <= 4000

  async function handleSubmit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    if (!valid || props.pending) return
    const succeeded = await props.onSubmit(trimmedText)
    if (succeeded) props.onOpenChange(false)
  }

  return (
    <Dialog open={props.open} onOpenChange={props.onOpenChange}>
      <DialogContent className='max-h-[calc(100dvh-1.5rem)] overflow-x-hidden overflow-y-auto sm:max-w-xl'>
        <DialogHeader>
          <DialogTitle>{t('Test provider connection')}</DialogTitle>
          <DialogDescription className='break-words'>
            {props.provider?.name}
          </DialogDescription>
        </DialogHeader>
        <form className='min-w-0 space-y-4' onSubmit={handleSubmit}>
          <Field data-invalid={!valid}>
            <FieldLabel htmlFor='risk-provider-validation-text'>
              {t('Test content')}
            </FieldLabel>
            <Textarea
              id='risk-provider-validation-text'
              className='min-h-36 resize-y'
              value={text}
              autoFocus
              aria-invalid={!valid}
              onChange={(event) => setText(event.target.value)}
            />
            <FieldDescription>
              {t(
                'Enter 1 to 4000 characters to send to the cloud review provider.'
              )}
            </FieldDescription>
          </Field>
          <DialogFooter>
            <Button
              type='button'
              variant='outline'
              disabled={props.pending}
              onClick={() => props.onOpenChange(false)}
            >
              {t('Cancel')}
            </Button>
            <Button type='submit' disabled={!valid || props.pending}>
              {props.pending ? t('Testing...') : t('Run test')}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}
