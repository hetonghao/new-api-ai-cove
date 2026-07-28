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
import { Database } from 'lucide-react'
import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { ErrorState } from '@/components/error-state'
import { Button } from '@/components/ui/button'
import {
  Field,
  FieldDescription,
  FieldError,
  FieldLabel,
} from '@/components/ui/field'
import { Input } from '@/components/ui/input'
import { NativeSelect, NativeSelectOption } from '@/components/ui/native-select'
import { Skeleton } from '@/components/ui/skeleton'
import { TitledCard } from '@/components/ui/titled-card'

import { getRiskRecordGovernance, updateRiskRecordGovernance } from '../api'
import {
  riskContentSaveScopeSchema,
  riskRecordRetentionDaysSchema,
  type RiskContentSaveScope,
} from '../types'

const QUERY_KEY = ['risk', 'records', 'settings'] as const

export function RiskRecordGovernanceSettings() {
  const { t } = useTranslation()
  const [contentSaveScope, setContentSaveScope] =
    useState<RiskContentSaveScope>('all')
  const [retentionDaysDraft, setRetentionDaysDraft] = useState('30')
  const [saving, setSaving] = useState(false)
  const settingsQuery = useQuery({
    queryKey: QUERY_KEY,
    queryFn: async () => {
      const response = await getRiskRecordGovernance()
      if (!response.success || !response.data) {
        throw new Error(
          response.message || t('Failed to load risk record settings')
        )
      }
      return response.data
    },
    retry: false,
  })

  useEffect(() => {
    if (settingsQuery.data) {
      setContentSaveScope(settingsQuery.data.content_save_scope)
      setRetentionDaysDraft(String(settingsQuery.data.retention_days))
    }
  }, [settingsQuery.data])

  const retentionDaysResult = riskRecordRetentionDaysSchema.safeParse(
    Number(retentionDaysDraft)
  )
  const retentionDays = retentionDaysResult.success
    ? retentionDaysResult.data
    : null

  async function saveSettings() {
    if (!settingsQuery.data || retentionDays === null) return
    setSaving(true)
    try {
      const response = await updateRiskRecordGovernance({
        ...settingsQuery.data,
        content_save_scope: contentSaveScope,
        retention_days: retentionDays,
      })
      if (!response.success || !response.data) {
        throw new Error(response.message || t('Request failed'))
      }
      setContentSaveScope(response.data.content_save_scope)
      setRetentionDaysDraft(String(response.data.retention_days))
      toast.success(t('Risk record settings saved'))
      await settingsQuery.refetch()
    } catch (error) {
      toast.error(error instanceof Error ? error.message : t('Request failed'))
    } finally {
      setSaving(false)
    }
  }

  let content = <Skeleton className='h-32 rounded-lg' />
  if (settingsQuery.isError) {
    content = (
      <ErrorState
        title={t('Failed to load risk record settings')}
        description={
          settingsQuery.error instanceof Error
            ? settingsQuery.error.message
            : t('Request failed')
        }
        onRetry={() => void settingsQuery.refetch()}
      />
    )
  } else if (!settingsQuery.isLoading) {
    content = (
      <div className='space-y-4'>
        <Field>
          <FieldLabel htmlFor='risk-content-save-scope'>
            {t('Risk content storage')}
          </FieldLabel>
          <NativeSelect
            id='risk-content-save-scope'
            className='w-full sm:max-w-sm'
            value={contentSaveScope}
            onChange={(event) => {
              const parsed = riskContentSaveScopeSchema.safeParse(
                event.target.value
              )
              if (parsed.success) setContentSaveScope(parsed.data)
            }}
          >
            <NativeSelectOption value='all'>
              {t('Save all content')}
            </NativeSelectOption>
            <NativeSelectOption value='unsafe'>
              {t('Save unsafe content only')}
            </NativeSelectOption>
            <NativeSelectOption value='none'>
              {t('Do not save content')}
            </NativeSelectOption>
          </NativeSelect>
          <FieldDescription>
            {t(
              'Controls whether the redacted detection content is retained in risk records.'
            )}
          </FieldDescription>
        </Field>
        <Field data-invalid={!retentionDaysResult.success}>
          <FieldLabel htmlFor='risk-record-retention-days'>
            {t('Retain last N days')}
          </FieldLabel>
          <Input
            id='risk-record-retention-days'
            className='sm:max-w-32'
            type='number'
            min={1}
            max={180}
            step={1}
            value={retentionDaysDraft}
            aria-invalid={!retentionDaysResult.success}
            onChange={(event) => setRetentionDaysDraft(event.target.value)}
          />
          <FieldDescription>
            {t(
              'Risk records older than this are deleted by the daily cleanup task.'
            )}
          </FieldDescription>
          {!retentionDaysResult.success && (
            <FieldError>
              {t('Retention days must be between {{min}} and {{max}}', {
                min: 1,
                max: 180,
              })}
            </FieldError>
          )}
        </Field>
        <div className='flex justify-end'>
          <Button
            type='button'
            disabled={
              saving ||
              retentionDays === null ||
              (contentSaveScope === settingsQuery.data?.content_save_scope &&
                retentionDays === settingsQuery.data?.retention_days)
            }
            onClick={() => void saveSettings()}
          >
            {saving ? t('Saving...') : t('Save settings')}
          </Button>
        </div>
      </div>
    )
  }

  return (
    <TitledCard
      title={t('Risk record settings')}
      description={t('Manage risk record storage and automatic cleanup.')}
      icon={<Database className='size-5' />}
      disableHoverEffect
    >
      {content}
    </TitledCard>
  )
}
