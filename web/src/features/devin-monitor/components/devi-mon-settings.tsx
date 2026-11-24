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
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Controller, useForm } from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { z } from 'zod'

import { ErrorState } from '@/components/error-state'
import { LoadingState } from '@/components/loading-state'
import { Button } from '@/components/ui/button'
import {
  Field,
  FieldDescription,
  FieldError,
  FieldLabel,
} from '@/components/ui/field'
import { Input } from '@/components/ui/input'
import { TitledCard } from '@/components/ui/titled-card'

import {
  deviMonQueryKey,
  deviMonSettingsKey,
  getDeviMonSettings,
  saveDeviMonSettings,
} from '../api'
import type { DeviMonSettings } from '../types'

const settingsFormSchema = z.object({
  interval_minutes: z.number().int().min(15).max(10080),
})
type SettingsForm = z.infer<typeof settingsFormSchema>

export function DeviMonSettingsPanel(props: { readonly onClose: () => void }) {
  const { t } = useTranslation()
  const settings = useQuery({
    queryKey: deviMonSettingsKey,
    queryFn: ({ signal }) => getDeviMonSettings(signal),
    retry: false,
  })
  if (settings.isPending) return <LoadingState />
  if (!settings.data) {
    return (
      <ErrorState
        description={t('Failed to load the monitor settings')}
        onRetry={() => void settings.refetch()}
      />
    )
  }
  return <SettingsFormCard settings={settings.data} onClose={props.onClose} />
}

function SettingsFormCard(props: {
  readonly settings: DeviMonSettings
  readonly onClose: () => void
}) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const form = useForm<SettingsForm>({
    resolver: zodResolver(settingsFormSchema),
    defaultValues: { interval_minutes: props.settings.interval_minutes },
  })
  const save = useMutation({
    mutationFn: (values: DeviMonSettings) => saveDeviMonSettings(values),
    onSuccess: (saved) => {
      queryClient.setQueryData(deviMonSettingsKey, saved)
      void queryClient.invalidateQueries({ queryKey: deviMonQueryKey })
      form.reset({ interval_minutes: saved.interval_minutes })
      toast.success(t('Settings saved'))
    },
  })

  return (
    <TitledCard
      title={t('Collection interval')}
      description={t(
        'The monitor probes every Codex account one at a time with a random gap, so one cycle takes a couple of minutes.'
      )}
      disableHoverEffect
      className='max-w-xl'
    >
      <form
        className='space-y-4'
        onSubmit={form.handleSubmit((values) => save.mutate(values))}
      >
        <Field>
          <FieldLabel htmlFor='devi-mon-interval'>
            {t('Interval (minutes)')}
          </FieldLabel>
          <Controller
            control={form.control}
            name='interval_minutes'
            render={({ field, fieldState }) => (
              <>
                <Input
                  id='devi-mon-interval'
                  type='number'
                  min={15}
                  max={10080}
                  inputMode='numeric'
                  value={field.value}
                  onChange={(event) =>
                    field.onChange(
                      event.target.value === '' ? 0 : Number(event.target.value)
                    )
                  }
                  onBlur={field.onBlur}
                  aria-invalid={fieldState.invalid}
                />
                <FieldDescription>
                  {t('Between 15 minutes and 7 days. Default is 180 minutes.')}
                </FieldDescription>
                <FieldError>{fieldState.error?.message}</FieldError>
              </>
            )}
          />
        </Field>
        <div className='flex items-center gap-2'>
          <Button type='submit' disabled={save.isPending}>
            {save.isPending ? t('Saving...') : t('Save changes')}
          </Button>
          <Button type='button' variant='ghost' onClick={props.onClose}>
            {t('Back to monitor')}
          </Button>
        </div>
      </form>
    </TitledCard>
  )
}
