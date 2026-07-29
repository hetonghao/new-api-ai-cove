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
import { FlaskConical, Pencil, Trash2 } from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Switch } from '@/components/ui/switch'

import type { LocalRiskRule } from '../types'

type LocalRuleCardProps = {
  readonly rule: LocalRiskRule
  readonly isPending: boolean
  readonly onToggle: (rule: LocalRiskRule, enabled: boolean) => void
  readonly onEdit: (rule: LocalRiskRule) => void
  readonly onTest: (rule: LocalRiskRule) => void
  readonly onDelete: (rule: LocalRiskRule) => void
}

export function LocalRuleCard(props: LocalRuleCardProps) {
  const { t } = useTranslation()
  const typeLabel = {
    keyword: t('Keyword'),
    phrase: t('Phrase'),
    regex: t('Go regular expression'),
  }[props.rule.rule_type]
  const actionLabel =
    props.rule.action === 'skip'
      ? t('Skip cloud review')
      : t('Send to cloud review')

  return (
    <article className='flex min-w-0 flex-col gap-3 rounded-lg border p-3 sm:p-4'>
      <div className='flex min-w-0 flex-col gap-3 sm:flex-row sm:items-start sm:justify-between'>
        <div className='min-w-0 space-y-2'>
          <div className='flex flex-wrap items-center gap-2'>
            <Badge variant='secondary'>{typeLabel}</Badge>
            <Badge variant={props.rule.enabled ? 'default' : 'outline'}>
              {props.rule.enabled ? t('Enabled') : t('Disabled')}
            </Badge>
            <Badge variant='outline'>{actionLabel}</Badge>
          </div>
          <code className='bg-muted block max-w-full rounded-md px-2.5 py-2 text-sm [overflow-wrap:anywhere] whitespace-pre-wrap'>
            {props.rule.pattern}
          </code>
        </div>
        <div className='flex shrink-0 items-center gap-2'>
          <span className='text-muted-foreground text-xs'>
            {t('Rule enabled')}
          </span>
          <Switch
            size='sm'
            checked={props.rule.enabled}
            disabled={props.isPending}
            onCheckedChange={(enabled) => props.onToggle(props.rule, enabled)}
            aria-label={t('Toggle rule: {{pattern}}', {
              pattern: props.rule.pattern,
            })}
          />
        </div>
      </div>
      <div className='flex flex-wrap justify-end gap-2'>
        <Button
          size='sm'
          variant='outline'
          onClick={() => props.onTest(props.rule)}
        >
          <FlaskConical className='size-4' />
          {t('Test')}
        </Button>
        <Button
          size='sm'
          variant='outline'
          onClick={() => props.onEdit(props.rule)}
        >
          <Pencil className='size-4' />
          {t('Edit')}
        </Button>
        <Button
          size='icon-sm'
          variant='destructive'
          onClick={() => props.onDelete(props.rule)}
          aria-label={t('Delete rule: {{pattern}}', {
            pattern: props.rule.pattern,
          })}
        >
          <Trash2 className='size-4' />
        </Button>
      </div>
    </article>
  )
}
