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
import { Field, FieldDescription, FieldLabel } from '@/components/ui/field'
import { Textarea } from '@/components/ui/textarea'

import { testLocalRiskRule } from '../api'
import {
  createLocalRuleTestFormSchema,
  localRuleTestFormValuesToPayload,
} from '../lib/risk-policy-form'
import type { LocalRiskRule, LocalRiskRuleTestResult } from '../types'

type LocalRuleTestDialogProps = {
  readonly open: boolean
  readonly rule: LocalRiskRule | null
  readonly onOpenChange: (open: boolean) => void
}

export function LocalRuleTestDialog(props: LocalRuleTestDialogProps) {
  const { t } = useTranslation()
  const [text, setText] = useState('')
  const [result, setResult] = useState<LocalRiskRuleTestResult | null>(null)
  const [isTesting, setIsTesting] = useState(false)

  useEffect(() => {
    if (!props.open) return
    setText('')
    setResult(null)
  }, [props.open, props.rule])

  async function handleSubmit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    if (!props.rule) return

    const parsed = createLocalRuleTestFormSchema(t).safeParse({
      rule_type: props.rule.rule_type,
      pattern: props.rule.pattern,
      text,
    })
    if (!parsed.success) {
      toast.error(parsed.error.issues[0]?.message ?? t('Invalid test input'))
      return
    }

    setIsTesting(true)
    try {
      const response = await testLocalRiskRule(
        localRuleTestFormValuesToPayload(parsed.data)
      )
      if (!response.success || !response.data) {
        throw new Error(response.message)
      }
      setResult(response.data)
    } catch (error) {
      toast.error(error instanceof Error ? error.message : t('Request failed'))
    } finally {
      setIsTesting(false)
    }
  }

  return (
    <Dialog open={props.open} onOpenChange={props.onOpenChange}>
      <DialogContent className='max-h-[calc(100dvh-1.5rem)] grid-rows-[auto_minmax(0,1fr)] overflow-hidden sm:max-w-xl'>
        <DialogHeader>
          <DialogTitle>{t('Test local rule')}</DialogTitle>
          <DialogDescription className='break-all'>
            {props.rule?.pattern}
          </DialogDescription>
        </DialogHeader>
        <form
          onSubmit={handleSubmit}
          className='space-y-4 overflow-y-auto pr-1'
        >
          <Field>
            <FieldLabel htmlFor='local-risk-rule-test-text'>
              {t('Test text')}
            </FieldLabel>
            <Textarea
              id='local-risk-rule-test-text'
              value={text}
              className='min-h-32'
              autoFocus
              onChange={(event) => setText(event.target.value)}
            />
            <FieldDescription>
              {t(
                'The server applies Unicode normalization, lowercase conversion, trimming, and whitespace collapse.'
              )}
            </FieldDescription>
          </Field>
          {result ? (
            <Alert>
              <AlertTitle className='flex items-center gap-2'>
                {t('Test result')}
                <Badge variant={result.matched ? 'default' : 'outline'}>
                  {result.matched ? t('Matched') : t('Not matched')}
                </Badge>
              </AlertTitle>
              <AlertDescription>
                <span className='block font-medium'>
                  {t('Normalized text')}
                </span>
                <code className='mt-1 block break-words whitespace-pre-wrap'>
                  {result.normalized_text}
                </code>
              </AlertDescription>
            </Alert>
          ) : null}
          <DialogFooter>
            <Button
              type='button'
              variant='outline'
              onClick={() => props.onOpenChange(false)}
            >
              {t('Close')}
            </Button>
            <Button type='submit' disabled={isTesting}>
              {isTesting ? t('Testing...') : t('Run test')}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}
