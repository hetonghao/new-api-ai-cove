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
import { Switch } from '@/components/ui/switch'
import { TitledCard } from '@/components/ui/titled-card'
import { getServerErrorMessage } from '@/lib/server-error-message'

import {
  configKey,
  getCacheConfig,
  MIB,
  saveCacheConfig,
  statsKey,
  type CacheConfig,
} from './api'
import {
  cacheFormSchema,
  getCacheConfigErrorField,
  type CacheForm,
} from './lib/configuration'

function formValues(config: CacheConfig): CacheForm {
  return {
    enabled: config.enabled,
    capacity: config.max_bytes / MIB,
    ttl: config.ttl_seconds,
  }
}

export function CacheConfiguration() {
  const settings = useQuery({
    queryKey: configKey,
    queryFn: getCacheConfig,
    meta: { errorToast: false },
  })
  if (settings.isPending) return <LoadingState />
  if (!settings.data) {
    return (
      <ErrorState
        description={getServerErrorMessage(settings.error)}
        onRetry={() => void settings.refetch()}
      />
    )
  }
  return <CacheConfigForm config={settings.data} />
}

function CacheConfigForm(props: { readonly config: CacheConfig }) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const form = useForm<CacheForm>({
    resolver: zodResolver(cacheFormSchema),
    defaultValues: formValues(props.config),
  })
  const save = useMutation({
    mutationFn: saveCacheConfig,
    onSuccess: (config) => {
      queryClient.setQueryData(configKey, config)
      void queryClient.invalidateQueries({ queryKey: statsKey })
      form.reset(formValues(config))
      toast.success(t('Settings saved'))
    },
    onError: (error) => {
      form.setError(getCacheConfigErrorField(error), {
        type: 'server',
        message: getServerErrorMessage(error),
      })
    },
    meta: { errorToast: false },
  })
  const errors = form.formState.errors
  return (
    <TitledCard
      title={t('Cache configuration')}
      description={t(
        'Compressed images are reused from memory on the same node until they expire.'
      )}
      disableHoverEffect
    >
      <form
        className='space-y-4'
        noValidate
        onSubmit={form.handleSubmit((values) =>
          save.mutate({
            enabled: values.enabled,
            max_bytes: Math.round(values.capacity * MIB),
            ttl_seconds: values.ttl,
          })
        )}
      >
        <Field orientation='horizontal' className='justify-between'>
          <FieldLabel htmlFor='compression-cache-enabled'>
            {t('Enable image compression cache')}
          </FieldLabel>
          <Controller
            control={form.control}
            name='enabled'
            render={({ field }) => (
              <Switch
                id='compression-cache-enabled'
                name={field.name}
                ref={field.ref}
                checked={field.value}
                onBlur={field.onBlur}
                onCheckedChange={field.onChange}
                disabled={save.isPending}
              />
            )}
          />
        </Field>
        <div className='grid gap-4 sm:grid-cols-2'>
          <Field data-invalid={Boolean(errors.capacity)}>
            <FieldLabel htmlFor='compression-cache-capacity'>
              {t('Cache capacity (MiB)')}
            </FieldLabel>
            <Input
              id='compression-cache-capacity'
              type='number'
              min={1}
              max={256}
              step='any'
              aria-invalid={Boolean(errors.capacity)}
              aria-describedby={
                errors.capacity
                  ? 'compression-cache-capacity-hint compression-cache-capacity-error'
                  : 'compression-cache-capacity-hint'
              }
              disabled={save.isPending}
              {...form.register('capacity', { valueAsNumber: true })}
            />
            <FieldDescription id='compression-cache-capacity-hint'>
              {t('1 to 256 MiB per node.')}
            </FieldDescription>
            {errors.capacity && (
              <FieldError id='compression-cache-capacity-error'>
                {errors.capacity.type === 'server'
                  ? errors.capacity.message
                  : t('Enter a number from 1 to 256.')}
              </FieldError>
            )}
          </Field>
          <Field data-invalid={Boolean(errors.ttl)}>
            <FieldLabel htmlFor='compression-cache-ttl'>
              {t('Cache lifetime (seconds)')}
            </FieldLabel>
            <Input
              id='compression-cache-ttl'
              type='number'
              min={60}
              max={3600}
              aria-invalid={Boolean(errors.ttl)}
              aria-describedby={
                errors.ttl
                  ? 'compression-cache-ttl-hint compression-cache-ttl-error'
                  : 'compression-cache-ttl-hint'
              }
              disabled={save.isPending}
              {...form.register('ttl', { valueAsNumber: true })}
            />
            <FieldDescription id='compression-cache-ttl-hint'>
              {t('60 to 3,600 seconds from insertion.')}
            </FieldDescription>
            {errors.ttl && (
              <FieldError id='compression-cache-ttl-error'>
                {errors.ttl.type === 'server'
                  ? errors.ttl.message
                  : t('Enter a whole number from 60 to 3,600.')}
              </FieldError>
            )}
          </Field>
        </div>
        {errors.root?.message && <FieldError>{errors.root.message}</FieldError>}
        <div className='flex justify-end'>
          <Button
            type='submit'
            disabled={!form.formState.isDirty || save.isPending}
            aria-busy={save.isPending}
          >
            {t('Save Changes')}
          </Button>
        </div>
      </form>
    </TitledCard>
  )
}
