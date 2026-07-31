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
import { useEffect, useState } from 'react'
import { useForm } from 'react-hook-form'
import { useTranslation } from 'react-i18next'

import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Badge } from '@/components/ui/badge'
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
  FieldDescription,
  FieldError,
  FieldLabel,
} from '@/components/ui/field'
import { Textarea } from '@/components/ui/textarea'

import { testLocalRiskRule } from '../api'
import {
  createLocalRuleTestFormSchema,
  localRuleTestFormValuesToPayload,
  localRuleTestToFormValues,
  type LocalRiskRuleTestFormValues,
} from '../lib/risk-policy-form'
import type { LocalRiskRule, LocalRiskRuleTestResult } from '../types'

type LocalRuleTestDialogProps = {
  readonly open: boolean
  readonly rule: LocalRiskRule | null
  readonly onOpenChange: (open: boolean) => void
}

export function LocalRuleTestDialog(props: LocalRuleTestDialogProps) {
  const { t } = useTranslation()
  const [result, setResult] = useState<LocalRiskRuleTestResult | null>(null)
  const [testedText, setTestedText] = useState<string | null>(null)
  const form = useForm<LocalRiskRuleTestFormValues>({
    resolver: zodResolver(createLocalRuleTestFormSchema(t)),
    defaultValues: localRuleTestToFormValues(null),
  })
  const errors = form.formState.errors
  const isRegexRule = props.rule?.rule_type === 'regex'

  useEffect(() => {
    if (!props.open) return
    form.reset(localRuleTestToFormValues(props.rule))
    setResult(null)
    setTestedText(null)
  }, [form, props.open, props.rule])

  async function handleSubmit(values: LocalRiskRuleTestFormValues) {
    if (!props.rule) return

    form.clearErrors('root.server')
    setResult(null)
    setTestedText(null)
    try {
      const response = await testLocalRiskRule(
        localRuleTestFormValuesToPayload(values)
      )
      if (!response.success || !response.data) {
        form.setError('root.server', {
          type: 'server',
          message: response.message || t('Request failed'),
        })
        return
      }
      setResult(response.data)
      setTestedText(values.text)
    } catch (error) {
      form.setError('root.server', {
        type: 'server',
        message: error instanceof Error ? error.message : t('Request failed'),
      })
    }
  }

  return (
    <Dialog open={props.open} onOpenChange={props.onOpenChange}>
      <DialogContent className='max-h-[calc(100dvh-1.5rem)] grid-rows-[auto_minmax(0,1fr)_auto] overflow-hidden sm:max-w-xl'>
        <DialogHeader>
          <DialogTitle>{t('Test local rule')}</DialogTitle>
          <DialogDescription className='break-all'>
            {props.rule?.pattern}
          </DialogDescription>
        </DialogHeader>
        <form
          id='local-risk-rule-test-form'
          onSubmit={form.handleSubmit(handleSubmit)}
          className='space-y-4 overflow-y-auto pr-1'
        >
          <Field data-invalid={Boolean(errors.text)}>
            <FieldLabel htmlFor='local-risk-rule-test-text'>
              {t('Test text')}
            </FieldLabel>
            <Textarea
              id='local-risk-rule-test-text'
              className='min-h-32'
              autoFocus
              aria-invalid={Boolean(errors.text)}
              {...form.register('text')}
            />
            <FieldDescription>
              {t(
                isRegexRule
                  ? 'Keywords and phrases use normalized text; Go regular expressions use the text as entered.'
                  : 'The server applies Unicode normalization, lowercase conversion, trimming, and whitespace collapse.'
              )}
            </FieldDescription>
            <FieldError errors={[errors.text]} />
          </Field>
          {errors.root?.server?.message ? (
            <Alert variant='destructive'>
              <AlertTitle>{t('Request failed')}</AlertTitle>
              <AlertDescription>{errors.root.server.message}</AlertDescription>
            </Alert>
          ) : null}
          {result ? (
            <Alert>
              <AlertTitle className='flex items-center gap-2'>
                {t('Test result')}
                <Badge variant={result.matched ? 'default' : 'outline'}>
                  {result.matched ? t('Matched') : t('Not matched')}
                </Badge>
                <Badge variant='outline'>
                  {result.action === 'skip'
                    ? t('Skip cloud review')
                    : t('Send to cloud review')}
                </Badge>
              </AlertTitle>
              <AlertDescription>
                <span className='block font-medium'>
                  {t(isRegexRule ? 'Test text' : 'Normalized text')}
                </span>
                <code className='mt-1 block break-words whitespace-pre-wrap'>
                  {isRegexRule ? testedText : result.normalized_text}
                </code>
              </AlertDescription>
            </Alert>
          ) : null}
        </form>
        <DialogFooter>
          <Button
            type='button'
            variant='outline'
            onClick={() => props.onOpenChange(false)}
          >
            {t('Close')}
          </Button>
          <Button
            type='submit'
            form='local-risk-rule-test-form'
            disabled={form.formState.isSubmitting}
          >
            {form.formState.isSubmitting ? t('Testing...') : t('Run test')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
