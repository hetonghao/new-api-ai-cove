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
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { isAxiosError } from 'axios'
import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { Dialog } from '@/components/dialog'
import { Button } from '@/components/ui/button'
import { Checkbox } from '@/components/ui/checkbox'
import {
  Field,
  FieldDescription,
  FieldError,
  FieldGroup,
  FieldLabel,
} from '@/components/ui/field'
import { Input } from '@/components/ui/input'
import { Skeleton } from '@/components/ui/skeleton'
import { Switch } from '@/components/ui/switch'
import { getApiKeys } from '@/features/keys/api'
import { handleServerError } from '@/lib/handle-server-error'

import { getQualitySettings, updateQualitySettings } from '../api'
import type { QualitySettingsConfig } from '../types'

type SettingsDialogProps = {
  open: boolean
  onOpenChange: (open: boolean) => void
}

type SettingsForm = QualitySettingsConfig & { extraTokenId: string }

const EMPTY_FORM: SettingsForm = {
  enabled: false,
  token_ids: [],
  daily_limit: 200,
  concurrency: 2,
  retention_enabled: false,
  artifact_days: 30,
  metadata_days: 90,
  extraTokenId: '',
}

export function SettingsDialog(props: SettingsDialogProps) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const [form, setForm] = useState<SettingsForm>(EMPTY_FORM)
  const [version, setVersion] = useState(0)

  const settingsQuery = useQuery({
    queryKey: ['model-quality', 'settings'],
    queryFn: async () => {
      const response = await getQualitySettings()
      if (!response.success || !response.data) {
        throw new Error(response.message)
      }
      return response.data
    },
    enabled: props.open,
    retry: false,
  })
  useEffect(() => {
    const data = settingsQuery.data
    if (!data) return
    setForm({ ...data.config, extraTokenId: '' })
    setVersion(data.version)
  }, [settingsQuery.data])

  const tokensQuery = useQuery({
    queryKey: ['model-quality', 'settings-tokens'],
    queryFn: async () => {
      const response = await getApiKeys({ p: 1, size: 100 })
      if (!response.success || !response.data) {
        throw new Error(response.message ?? 'failed')
      }
      return response.data.items
    },
    enabled: props.open,
    retry: false,
  })

  const saveMutation = useMutation({
    mutationFn: () =>
      updateQualitySettings({
        config: {
          enabled: form.enabled,
          token_ids: form.token_ids,
          daily_limit: form.daily_limit,
          concurrency: form.concurrency,
          retention_enabled: form.retention_enabled,
          artifact_days: form.artifact_days,
          metadata_days: form.metadata_days,
        },
        expected_version: version,
      }),
    retry: false,
    onSuccess: (response) => {
      if (!response.success) {
        toast.error(response.message || t('Request failed'))
        return
      }
      toast.success(t('Settings saved'))
      void queryClient.invalidateQueries({ queryKey: ['model-quality'] })
      void settingsQuery.refetch()
    },
    onError: (error) => {
      if (isAxiosError(error) && error.response?.status === 409) {
        toast.error(t('Settings were modified elsewhere, reloading'))
        void settingsQuery.refetch()
        return
      }
      handleServerError(error, t('Failed to save settings'))
    },
  })

  const toggleToken = (id: number, checked: boolean) => {
    setForm((prev) => ({
      ...prev,
      token_ids: checked
        ? [...new Set([...prev.token_ids, id])]
        : prev.token_ids.filter((entry) => entry !== id),
    }))
  }
  const addTokenId = () => {
    const id = Number(form.extraTokenId)
    if (!Number.isInteger(id) || id <= 0) {
      toast.error(t('Enter a positive token ID'))
      return
    }
    toggleToken(id, true)
    setForm((prev) => ({ ...prev, extraTokenId: '' }))
  }

  const numberField = (
    id: string,
    label: string,
    value: number,
    min: number,
    max: number,
    apply: (value: number) => void
  ) => (
    <Field>
      <FieldLabel htmlFor={id}>{label}</FieldLabel>
      <Input
        id={id}
        type='number'
        min={min}
        max={max}
        value={Number.isNaN(value) ? '' : value}
        onChange={(event) => apply(event.target.valueAsNumber)}
      />
    </Field>
  )

  return (
    <Dialog
      open={props.open}
      onOpenChange={props.onOpenChange}
      title={t('Execution settings')}
      description={t(
        'Only approved, finite-quota administrator tokens may execute quality runs.'
      )}
      footer={
        <div className='flex justify-end gap-2'>
          <Button
            size='sm'
            onClick={() => saveMutation.mutate()}
            disabled={saveMutation.isPending || settingsQuery.isPending}
          >
            {t('Save')}
          </Button>
        </div>
      }
    >
      {settingsBody()}
    </Dialog>
  )

  function settingsBody() {
    if (settingsQuery.isPending) {
      return (
        <div className='space-y-3'>
          <Skeleton className='h-8 w-full' />
          <Skeleton className='h-24 w-full' />
        </div>
      )
    }
    if (settingsQuery.isError) {
      return (
        <div className='text-muted-foreground text-sm'>
          {t('Failed to load settings')}
          <Button
            size='sm'
            variant='outline'
            className='ml-2'
            onClick={() => void settingsQuery.refetch()}
          >
            {t('Retry')}
          </Button>
        </div>
      )
    }
    return (
        <FieldGroup className='space-y-4'>
          <Field>
            <FieldLabel htmlFor='mqs-enabled'>{t('Enabled')}</FieldLabel>
            <div className='flex items-center gap-2'>
              <Switch
                id='mqs-enabled'
                checked={form.enabled}
                onCheckedChange={(checked) =>
                  setForm((prev) => ({ ...prev, enabled: checked === true }))
                }
              />
              {!form.enabled && (
                <span className='text-muted-foreground text-xs'>
                  {t('Execution is disabled; scheduled and manual runs will not start.')}
                </span>
              )}
            </div>
          </Field>
          <Field>
            <FieldLabel>{t('Approved execution tokens')}</FieldLabel>
            <div className='grid gap-1 sm:grid-cols-2'>
              {(tokensQuery.data ?? []).map((token) => (
                <label
                  key={token.id}
                  className='flex items-center gap-2 text-sm'
                >
                  <Checkbox
                    checked={form.token_ids.includes(token.id)}
                    onCheckedChange={(checked) =>
                      toggleToken(token.id, checked === true)
                    }
                  />
                  <span className='truncate'>
                    {token.name} (#{token.id})
                  </span>
                </label>
              ))}
              {form.token_ids
                .filter(
                  (id) =>
                    !(tokensQuery.data ?? []).some((token) => token.id === id)
                )
                .map((id) => (
                  <label key={id} className='flex items-center gap-2 text-sm'>
                    <Checkbox
                      checked
                      onCheckedChange={(checked) =>
                        toggleToken(id, checked === true)
                      }
                    />
                    <span className='text-muted-foreground truncate'>
                      #{id}
                    </span>
                  </label>
                ))}
            </div>
            <div className='mt-2 flex items-center gap-2'>
              <Input
                type='number'
                min={1}
                value={form.extraTokenId}
                onChange={(event) =>
                  setForm((prev) => ({
                    ...prev,
                    extraTokenId: event.target.value,
                  }))
                }
                placeholder={t('Token ID')}
                aria-label={t('Token ID')}
                className='w-36'
              />
              <Button
                size='sm'
                variant='outline'
                type='button'
                onClick={addTokenId}
              >
                {t('Add token ID')}
              </Button>
            </div>
            <FieldDescription>
              {t(
                'Token secrets are never displayed; approval stores only token IDs.'
              )}
            </FieldDescription>
          </Field>
          <div className='grid gap-4 sm:grid-cols-2'>
            {numberField(
              'mqs-daily-limit',
              t('Global daily sample limit'),
              form.daily_limit,
              1,
              10000,
              (value) => setForm((prev) => ({ ...prev, daily_limit: value }))
            )}
            {numberField(
              'mqs-concurrency',
              t('Concurrency'),
              form.concurrency,
              1,
              2,
              (value) => setForm((prev) => ({ ...prev, concurrency: value }))
            )}
          </div>
          <Field>
            <FieldLabel htmlFor='mqs-retention'>
              {t('Retention cleanup')}
            </FieldLabel>
            <div className='flex items-center gap-2'>
              <Switch
                id='mqs-retention'
                checked={form.retention_enabled}
                onCheckedChange={(checked) =>
                  setForm((prev) => ({
                    ...prev,
                    retention_enabled: checked === true,
                  }))
                }
              />
            </div>
          </Field>
          <div className='grid gap-4 sm:grid-cols-2'>
            {numberField(
              'mqs-artifact-days',
              t('Artifact retention (days)'),
              form.artifact_days,
              1,
              90,
              (value) => setForm((prev) => ({ ...prev, artifact_days: value }))
            )}
            {numberField(
              'mqs-metadata-days',
              t('Metadata retention (days)'),
              form.metadata_days,
              1,
              365,
              (value) => setForm((prev) => ({ ...prev, metadata_days: value }))
            )}
          </div>
          {form.metadata_days < form.artifact_days && (
            <FieldError
              errors={[
                {
                  message: t(
                    'Metadata retention must be at least artifact retention'
                  ),
                },
              ]}
            />
          )}
        </FieldGroup>
    )
  }
}
