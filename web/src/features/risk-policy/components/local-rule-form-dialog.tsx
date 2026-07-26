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
import { useEffect } from 'react'
import { Controller, useForm } from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import {
  Field,
  FieldContent,
  FieldDescription,
  FieldError,
  FieldGroup,
  FieldLabel,
  FieldTitle,
} from '@/components/ui/field'
import { NativeSelect, NativeSelectOption } from '@/components/ui/native-select'
import { Switch } from '@/components/ui/switch'
import { Textarea } from '@/components/ui/textarea'

import { createLocalRiskRule, updateLocalRiskRule } from '../api'
import {
  createLocalRuleFormSchema,
  localRuleFormValuesToPayload,
  localRuleToFormValues,
  type LocalRiskRuleFormValues,
} from '../lib/risk-policy-form'
import type { LocalRiskRule } from '../types'

type LocalRuleFormDialogProps = {
  readonly open: boolean
  readonly rule: LocalRiskRule | null
  readonly onOpenChange: (open: boolean) => void
  readonly onSaved: () => void
}

export function LocalRuleFormDialog(props: LocalRuleFormDialogProps) {
  const { t } = useTranslation()
  const form = useForm<LocalRiskRuleFormValues>({
    resolver: zodResolver(createLocalRuleFormSchema(t)),
    defaultValues: localRuleToFormValues(null),
  })
  const ruleType = form.watch('rule_type')
  const errors = form.formState.errors

  useEffect(() => {
    if (props.open) {
      form.reset(localRuleToFormValues(props.rule))
    }
  }, [form, props.open, props.rule])

  async function handleSubmit(values: LocalRiskRuleFormValues) {
    form.clearErrors('root.server')
    try {
      const payload = localRuleFormValuesToPayload(values)
      const response = props.rule
        ? await updateLocalRiskRule(props.rule.id, payload)
        : await createLocalRiskRule(payload)
      if (!response.success) {
        form.setError('root.server', {
          type: 'server',
          message: response.message || t('Request failed'),
        })
        return
      }
      toast.success(props.rule ? t('Rule updated') : t('Rule created'))
      props.onOpenChange(false)
      props.onSaved()
    } catch (error) {
      form.setError('root.server', {
        type: 'server',
        message: error instanceof Error ? error.message : t('Request failed'),
      })
    }
  }

  return (
    <Dialog open={props.open} onOpenChange={props.onOpenChange}>
      <DialogContent className='max-h-[calc(100dvh-1.5rem)] grid-rows-[auto_minmax(0,1fr)] overflow-hidden sm:max-w-xl'>
        <DialogHeader>
          <DialogTitle>
            {props.rule ? t('Edit local rule') : t('Add local rule')}
          </DialogTitle>
          <DialogDescription>
            {t(
              'Local rules only select content for cloud review. They never reject requests by themselves.'
            )}
          </DialogDescription>
        </DialogHeader>
        <form
          onSubmit={form.handleSubmit(handleSubmit)}
          className='space-y-5 overflow-y-auto pr-1'
        >
          <FieldGroup>
            <Field data-invalid={Boolean(errors.rule_type)}>
              <FieldLabel htmlFor='local-risk-rule-type'>
                {t('Rule type')}
              </FieldLabel>
              <NativeSelect
                id='local-risk-rule-type'
                className='w-full'
                aria-invalid={Boolean(errors.rule_type)}
                {...form.register('rule_type')}
              >
                <NativeSelectOption value='keyword'>
                  {t('Keyword')}
                </NativeSelectOption>
                <NativeSelectOption value='phrase'>
                  {t('Phrase')}
                </NativeSelectOption>
                <NativeSelectOption value='regex'>
                  {t('Go regular expression')}
                </NativeSelectOption>
              </NativeSelect>
              <FieldError errors={[errors.rule_type]} />
            </Field>
            <Field data-invalid={Boolean(errors.pattern)}>
              <FieldLabel htmlFor='local-risk-rule-pattern'>
                {t('Rule pattern')}
              </FieldLabel>
              <Textarea
                id='local-risk-rule-pattern'
                className='min-h-24 font-mono text-sm'
                autoFocus
                aria-invalid={Boolean(errors.pattern)}
                {...form.register('pattern')}
              />
              <FieldDescription>
                {ruleType === 'regex'
                  ? t('Regex syntax is validated by Go RE2 when saved.')
                  : t('Matching is case-insensitive after text normalization.')}
              </FieldDescription>
              <FieldError errors={[errors.pattern]} />
            </Field>
            <Field orientation='horizontal' className='rounded-lg border p-3'>
              <FieldContent>
                <FieldTitle>{t('Rule enabled')}</FieldTitle>
                <FieldDescription>
                  {t(
                    'Disabled rules are kept but do not trigger cloud review.'
                  )}
                </FieldDescription>
              </FieldContent>
              <Controller
                control={form.control}
                name='enabled'
                render={({ field }) => (
                  <Switch
                    checked={field.value}
                    onCheckedChange={field.onChange}
                    aria-label={t('Rule enabled')}
                  />
                )}
              />
            </Field>
          </FieldGroup>
          {errors.root?.server?.message ? (
            <Alert variant='destructive'>
              <AlertTitle>{t('Failed to save')}</AlertTitle>
              <AlertDescription>{errors.root.server.message}</AlertDescription>
            </Alert>
          ) : null}
          <DialogFooter>
            <Button
              type='button'
              variant='outline'
              disabled={form.formState.isSubmitting}
              onClick={() => props.onOpenChange(false)}
            >
              {t('Cancel')}
            </Button>
            <Button type='submit' disabled={form.formState.isSubmitting}>
              {form.formState.isSubmitting ? t('Saving...') : t('Save')}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}
