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
import {
  Field,
  FieldContent,
  FieldDescription,
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
import type { LocalRiskRule, LocalRiskRuleType } from '../types'

type LocalRuleFormDialogProps = {
  readonly open: boolean
  readonly rule: LocalRiskRule | null
  readonly onOpenChange: (open: boolean) => void
  readonly onSaved: () => void
}

export function LocalRuleFormDialog(props: LocalRuleFormDialogProps) {
  const { t } = useTranslation()
  const [values, setValues] = useState<LocalRiskRuleFormValues>(
    localRuleToFormValues(null)
  )
  const [isSaving, setIsSaving] = useState(false)

  useEffect(() => {
    if (props.open) setValues(localRuleToFormValues(props.rule))
  }, [props.open, props.rule])

  function handleRuleTypeChange(value: string) {
    let ruleType: LocalRiskRuleType = 'keyword'
    if (value === 'phrase') ruleType = 'phrase'
    else if (value === 'regex') ruleType = 'regex'
    setValues((current) => ({ ...current, rule_type: ruleType }))
  }

  async function handleSubmit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    const parsed = createLocalRuleFormSchema(t).safeParse(values)
    if (!parsed.success) {
      toast.error(parsed.error.issues[0]?.message ?? t('Invalid rule'))
      return
    }

    setIsSaving(true)
    try {
      const payload = localRuleFormValuesToPayload(parsed.data)
      const response = props.rule
        ? await updateLocalRiskRule(props.rule.id, payload)
        : await createLocalRiskRule(payload)
      if (!response.success) throw new Error(response.message)
      toast.success(props.rule ? t('Rule updated') : t('Rule created'))
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
          onSubmit={handleSubmit}
          className='space-y-5 overflow-y-auto pr-1'
        >
          <FieldGroup>
            <Field>
              <FieldLabel htmlFor='local-risk-rule-type'>
                {t('Rule type')}
              </FieldLabel>
              <NativeSelect
                id='local-risk-rule-type'
                className='w-full'
                value={values.rule_type}
                onChange={(event) => handleRuleTypeChange(event.target.value)}
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
            </Field>
            <Field>
              <FieldLabel htmlFor='local-risk-rule-pattern'>
                {t('Rule pattern')}
              </FieldLabel>
              <Textarea
                id='local-risk-rule-pattern'
                value={values.pattern}
                className='min-h-24 font-mono text-sm'
                autoFocus
                onChange={(event) =>
                  setValues((current) => ({
                    ...current,
                    pattern: event.target.value,
                  }))
                }
              />
              <FieldDescription>
                {values.rule_type === 'regex'
                  ? t('Regex syntax is validated by Go RE2 when saved.')
                  : t('Matching is case-insensitive after text normalization.')}
              </FieldDescription>
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
              <Switch
                checked={values.enabled}
                onCheckedChange={(enabled) =>
                  setValues((current) => ({ ...current, enabled }))
                }
                aria-label={t('Rule enabled')}
              />
            </Field>
          </FieldGroup>
          <DialogFooter>
            <Button
              type='button'
              variant='outline'
              disabled={isSaving}
              onClick={() => props.onOpenChange(false)}
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
