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
import { useNavigate } from '@tanstack/react-router'
import { isAxiosError } from 'axios'
import { ChevronDown } from 'lucide-react'
import { useEffect, useMemo, useState } from 'react'
import { FormProvider, useForm } from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { ConfirmDialog } from '@/components/confirm-dialog'
import { StatusBadge } from '@/components/status-badge'
import { Button } from '@/components/ui/button'
import { Checkbox } from '@/components/ui/checkbox'
import {
  Collapsible,
  CollapsibleContent,
  CollapsibleTrigger,
} from '@/components/ui/collapsible'
import {
  Field,
  FieldDescription,
  FieldError,
  FieldGroup,
  FieldLabel,
} from '@/components/ui/field'
import { Input } from '@/components/ui/input'
import { NativeSelect, NativeSelectOption } from '@/components/ui/native-select'
import { Switch } from '@/components/ui/switch'
import { Textarea } from '@/components/ui/textarea'
import { formatTimestampToDate } from '@/lib/format'
import { handleServerError } from '@/lib/handle-server-error'

import {
  createQualityCase,
  deleteQualityCase,
  getQualityOutputContract,
  updateQualityCase,
} from '../api'
import {
  defaultQualityCaseValues,
  formValuesToWrite,
  getQualityCaseFormSchema,
  viewToFormValues,
  type QualityCaseFormValues,
} from '../lib/case-form'
import type { QualityCapabilities, QualityCaseView } from '../types'

type CaseEditorProps = {
  qualityCase: QualityCaseView | null
  capabilities: QualityCapabilities | undefined
  onDirtyChange: (dirty: boolean) => void
}

const WEEKDAY_KEYS = ['Mon', 'Tue', 'Wed', 'Thu', 'Fri', 'Sat', 'Sun']

export function CaseEditor(props: CaseEditorProps) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const navigate = useNavigate({ from: '/model-quality/' })
  const capabilities = props.capabilities
  const [deleteOpen, setDeleteOpen] = useState(false)

  const defaultValues = useMemo(
    () =>
      props.qualityCase
        ? viewToFormValues(props.qualityCase)
        : defaultQualityCaseValues({
            token_id: capabilities?.tokens[0]?.id ?? 0,
            group: capabilities?.tokens[0]?.group ?? '',
          }),
    // eslint-disable-next-line react-hooks/exhaustive-deps
    [props.qualityCase?.id, props.qualityCase?.edit_version]
  )

  const form = useForm<QualityCaseFormValues>({
    resolver: zodResolver(getQualityCaseFormSchema(t)),
    defaultValues,
  })
  const errors = form.formState.errors

  useEffect(() => {
    form.reset(defaultValues)
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [defaultValues])

  useEffect(() => {
    props.onDirtyChange(form.formState.isDirty)
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [form.formState.isDirty])

  const invalidate = () => {
    void queryClient.invalidateQueries({ queryKey: ['model-quality', 'cases'] })
  }

  const saveMutation = useMutation({
    mutationFn: (values: QualityCaseFormValues) => {
      const body = formValuesToWrite(
        values,
        props.qualityCase?.edit_version ?? 0
      )
      return props.qualityCase
        ? updateQualityCase(props.qualityCase.id, body)
        : createQualityCase(body)
    },
    retry: false,
    onSuccess: (response) => {
      if (!response.success || !response.data) {
        toast.error(response.message || t('Request failed'))
        return
      }
      const saved = response.data
      toast.success(t('Case saved'))
      form.reset(viewToFormValues(saved))
      invalidate()
    },
    onError: (error) => {
      if (isAxiosError(error) && error.response?.status === 409) {
        toast.error(t('Case was modified elsewhere, reloading'))
        invalidate()
        return
      }
      handleServerError(error, t('Failed to save the case'))
    },
  })

  const duplicateMutation = useMutation({
    mutationFn: () => {
      if (!props.qualityCase) throw new Error('no case to duplicate')
      const body = formValuesToWrite(viewToFormValues(props.qualityCase), 0)
      body.name = `${body.name} (copy)`
      return createQualityCase(body)
    },
    retry: false,
    onSuccess: (response) => {
      if (!response.success) {
        toast.error(response.message || t('Request failed'))
        return
      }
      toast.success(t('Case duplicated'))
      invalidate()
    },
    onError: (error) => {
      handleServerError(error, t('Failed to duplicate the case'))
    },
  })

  const deleteMutation = useMutation({
    mutationFn: () => {
      const qualityCase = props.qualityCase
      if (!qualityCase) throw new Error('no case to delete')
      return deleteQualityCase(qualityCase.id, qualityCase.edit_version)
    },
    retry: false,
    onSuccess: (response) => {
      if (!response.success) {
        toast.error(response.message || t('Request failed'))
        return
      }
      toast.success(t('Case deleted'))
      setDeleteOpen(false)
      invalidate()
      void navigate({
        search: (prev) => ({ ...prev, case: undefined }),
        replace: true,
      })
    },
    onError: (error) => {
      if (isAxiosError(error) && error.response?.status === 409) {
        toast.error(t('Case was modified elsewhere, reloading'))
        invalidate()
        return
      }
      handleServerError(error, t('Failed to delete the case'))
    },
  })

  const mode = form.watch('mode')
  const model = form.watch('model')
  const channelIds = form.watch('channel_ids')
  const scheduleEnabled = form.watch('schedule_enabled')
  const scheduleKind = form.watch('schedule_kind')
  const samplesPerTarget = form.watch('samples_per_target')
  const outputType = form.watch('output_type')
  const protocol = form.watch('protocol')
  const [contractOpen, setContractOpen] = useState(false)
  const contractQuery = useQuery({
    queryKey: ['model-quality', 'output-contract'],
    queryFn: getQualityOutputContract,
    staleTime: Infinity,
    enabled: outputType === 'svg',
  })
  const maxOutputTokens = form.watch('max_output_tokens')
  const timeoutSeconds = form.watch('timeout_seconds')
  const canOperate = capabilities?.can_operate ?? false

  const [advancedOpen, setAdvancedOpen] = useState(false)
  const advancedHasError = Boolean(
    errors.instruction_role ||
    errors.protocol ||
    errors.reasoning_effort ||
    errors.max_output_tokens ||
    errors.temperature ||
    errors.top_p ||
    errors.samples_per_target ||
    errors.timeout_seconds ||
    errors.daily_limit
  )
  useEffect(() => {
    if (advancedHasError) setAdvancedOpen(true)
  }, [advancedHasError])

  const modelOptions = useMemo(() => {
    const models = new Set<string>()
    capabilities?.channels.forEach((channel) =>
      channel.models.forEach((entry) => models.add(entry))
    )
    return [...models]
  }, [capabilities])

  const runCount =
    (mode === 'channel' ? Math.max(channelIds.length, 1) : 1) *
    (Number.isFinite(samplesPerTarget) ? samplesPerTarget : 0)

  const toggleChannel = (id: number, checked: boolean) => {
    const current = form.getValues('channel_ids')
    const next = checked
      ? [...current, id]
      : current.filter((entry) => entry !== id)
    form.setValue('channel_ids', next, { shouldDirty: true })
  }
  const toggleWeekday = (day: number, checked: boolean) => {
    const current = form.getValues('weekdays')
    const next = checked
      ? [...current, day].sort((a, b) => a - b)
      : current.filter((entry) => entry !== day)
    form.setValue('weekdays', next, { shouldDirty: true })
  }

  const tokenField = form.register('token_id', { valueAsNumber: true })

  return (
    <FormProvider {...form}>
      <form
        onSubmit={form.handleSubmit((values) => saveMutation.mutate(values))}
        className='space-y-4 rounded-lg border p-3 sm:p-4'
      >
        <fieldset disabled={!canOperate} className='space-y-4'>
          {form.formState.isDirty && (
            <StatusBadge
              variant='warning'
              copyable={false}
              label={t('Unsaved changes')}
            />
          )}

          <FieldGroup className='grid gap-4 sm:grid-cols-2'>
            <Field data-invalid={Boolean(errors.name)}>
              <FieldLabel htmlFor='mq-name'>{t('Name')}</FieldLabel>
              <Input
                id='mq-name'
                aria-invalid={Boolean(errors.name)}
                {...form.register('name')}
              />
              <FieldError errors={[errors.name]} />
            </Field>
            <Field>
              <FieldLabel htmlFor='mq-enabled'>{t('Enabled')}</FieldLabel>
              <div className='flex items-center gap-2'>
                <Switch
                  id='mq-enabled'
                  checked={form.watch('enabled')}
                  onCheckedChange={(checked) =>
                    form.setValue('enabled', checked === true, {
                      shouldDirty: true,
                    })
                  }
                />
              </div>
              <FieldDescription>
                {t(
                  'Enabled cases appear as tabs in the test panel; scheduling is separate.'
                )}
              </FieldDescription>
            </Field>
            <Field
              className='sm:col-span-2'
              data-invalid={Boolean(errors.description)}
            >
              <FieldLabel htmlFor='mq-description'>
                {t('Description')}
              </FieldLabel>
              <Textarea
                id='mq-description'
                rows={2}
                aria-invalid={Boolean(errors.description)}
                {...form.register('description')}
              />
              <FieldError errors={[errors.description]} />
            </Field>
          </FieldGroup>

          <FieldGroup className='grid gap-4 sm:grid-cols-2'>
            <Field data-invalid={Boolean(errors.output_type)}>
              <FieldLabel htmlFor='mq-output-type'>
                {t('Output type')}
              </FieldLabel>
              <NativeSelect
                id='mq-output-type'
                className='w-full'
                {...form.register('output_type')}
              >
                <NativeSelectOption value='svg'>SVG</NativeSelectOption>
                <NativeSelectOption value='text'>
                  {t('Text')}
                </NativeSelectOption>
              </NativeSelect>
              <FieldError errors={[errors.output_type]} />
            </Field>
            <Field data-invalid={Boolean(errors.mode)}>
              <FieldLabel htmlFor='mq-mode'>{t('Route mode')}</FieldLabel>
              <NativeSelect
                id='mq-mode'
                className='w-full'
                {...form.register('mode', {
                  onChange: (event) => {
                    if (event.target.value === 'route') {
                      form.setValue('channel_ids', [], { shouldDirty: true })
                    }
                  },
                })}
              >
                <NativeSelectOption value='route'>
                  {t('Normal routing')}
                </NativeSelectOption>
                <NativeSelectOption value='channel'>
                  {t('Pinned channel')}
                </NativeSelectOption>
              </NativeSelect>
              <FieldError errors={[errors.mode]} />
            </Field>
            <Field data-invalid={Boolean(errors.model)}>
              <FieldLabel htmlFor='mq-model'>{t('Model')}</FieldLabel>
              <Input
                id='mq-model'
                list='mq-model-options'
                aria-invalid={Boolean(errors.model)}
                {...form.register('model')}
              />
              <datalist id='mq-model-options'>
                {modelOptions.map((entry) => (
                  <option key={entry} value={entry} />
                ))}
              </datalist>
              <FieldError errors={[errors.model]} />
            </Field>
            <Field data-invalid={Boolean(errors.token_id)}>
              <FieldLabel htmlFor='mq-token'>{t('Execution token')}</FieldLabel>
              <NativeSelect
                id='mq-token'
                className='w-full'
                aria-invalid={Boolean(errors.token_id)}
                {...tokenField}
                onChange={(event) => {
                  tokenField.onChange(event)
                  const token = capabilities?.tokens.find(
                    (entry) => entry.id === Number(event.target.value)
                  )
                  if (token && form.getValues('group') === '') {
                    form.setValue('group', token.group, { shouldDirty: true })
                  }
                }}
              >
                <NativeSelectOption value={0}>
                  {t('Select a token')}
                </NativeSelectOption>
                {(capabilities?.tokens ?? []).map((token) => (
                  <NativeSelectOption key={token.id} value={token.id}>
                    {token.name} (#{token.id})
                  </NativeSelectOption>
                ))}
              </NativeSelect>
              {(capabilities?.tokens.length ?? 0) === 0 && (
                <FieldDescription>
                  {t(
                    'No approved execution token. Ask a root user to approve one in execution settings.'
                  )}
                </FieldDescription>
              )}
              <FieldError errors={[errors.token_id]} />
            </Field>
            <Field data-invalid={Boolean(errors.group)}>
              <FieldLabel htmlFor='mq-group'>{t('Group')}</FieldLabel>
              <Input
                id='mq-group'
                aria-invalid={Boolean(errors.group)}
                {...form.register('group')}
              />
              <FieldError errors={[errors.group]} />
            </Field>
            {mode === 'channel' && (
              <Field
                className='sm:col-span-2'
                data-invalid={Boolean(errors.channel_ids)}
              >
                <FieldLabel>{t('Target channels')}</FieldLabel>
                <div className='grid gap-1 sm:grid-cols-2'>
                  {(capabilities?.channels ?? []).map((channel) => {
                    const supports = channel.models.includes(model)
                    return (
                      <label
                        key={channel.id}
                        className='flex items-center gap-2 text-sm'
                      >
                        <Checkbox
                          checked={channelIds.includes(channel.id)}
                          onCheckedChange={(checked) =>
                            toggleChannel(channel.id, checked === true)
                          }
                        />
                        <span className='truncate' title={channel.name}>
                          {channel.name}
                        </span>
                        {!supports && (
                          <span className='text-muted-foreground text-xs'>
                            {t('Model not listed')}
                          </span>
                        )}
                      </label>
                    )
                  })}
                </div>
                <FieldError errors={[errors.channel_ids]} />
              </Field>
            )}
          </FieldGroup>

          <FieldGroup className='grid gap-4 sm:grid-cols-2'>
            <Field
              className='sm:col-span-2'
              data-invalid={Boolean(errors.prompt)}
            >
              <FieldLabel htmlFor='mq-prompt'>
                {t('Prompt (user message)')}
              </FieldLabel>
              <Textarea
                id='mq-prompt'
                rows={4}
                aria-invalid={Boolean(errors.prompt)}
                {...form.register('prompt')}
              />
              <FieldDescription>
                {t('Sent as the user message of every sample request.')}
              </FieldDescription>
              <FieldError errors={[errors.prompt]} />
            </Field>
            <Field
              className='sm:col-span-2'
              data-invalid={Boolean(errors.instruction)}
            >
              <FieldLabel htmlFor='mq-instruction'>
                {t('System instruction (optional)')}
              </FieldLabel>
              <Textarea
                id='mq-instruction'
                rows={3}
                aria-invalid={Boolean(errors.instruction)}
                {...form.register('instruction')}
              />
              <FieldDescription>
                {outputType === 'svg'
                  ? t(
                      'Sent before the prompt after the built-in SVG output contract (well-formed XML, single root, viewBox, no markdown fences). Leave empty to send only the contract.'
                    )
                  : t(
                      'Sent before the prompt as a system or developer message. Leave empty to send only the prompt.'
                    )}
              </FieldDescription>
              <FieldError errors={[errors.instruction]} />
              {outputType === 'svg' && (
                <Collapsible
                  open={contractOpen}
                  onOpenChange={setContractOpen}
                  className='mt-1'
                >
                  <CollapsibleTrigger
                    render={
                      <Button type='button' variant='ghost' size='sm' />
                    }
                    className='group -ml-2'
                  >
                    <ChevronDown
                      className='size-3 transition-transform group-data-[panel-open]:rotate-180'
                      aria-hidden='true'
                    />
                    {t('View built-in SVG contract')}
                  </CollapsibleTrigger>
                  <CollapsibleContent>
                    <pre className='bg-muted text-muted-foreground mt-1 max-h-64 overflow-auto rounded-md p-3 text-xs whitespace-pre-wrap'>
                      {contractQuery.data?.data?.contract ?? ''}
                    </pre>
                  </CollapsibleContent>
                </Collapsible>
              )}
            </Field>
          </FieldGroup>

          <Collapsible open={advancedOpen} onOpenChange={setAdvancedOpen}>
            <CollapsibleTrigger
              render={<Button type='button' variant='ghost' size='sm' />}
              className='group -ml-2'
            >
              <ChevronDown
                className='size-3 transition-transform group-data-[panel-open]:rotate-180'
                aria-hidden='true'
              />
              {t('Advanced settings')}
              <span className='text-muted-foreground ml-2 text-xs'>
                {`${protocol} · ${maxOutputTokens ? `${maxOutputTokens} tokens` : t('Unlimited')} · ${timeoutSeconds}s`}
              </span>
            </CollapsibleTrigger>
            <CollapsibleContent className='space-y-4 pt-3'>
              <FieldGroup className='grid gap-4 sm:grid-cols-2'>
                <Field data-invalid={Boolean(errors.instruction_role)}>
                  <FieldLabel htmlFor='mq-instruction-role'>
                    {t('Instruction role')}
                  </FieldLabel>
                  <NativeSelect
                    id='mq-instruction-role'
                    className='w-full'
                    {...form.register('instruction_role')}
                  >
                    <NativeSelectOption value='system'>
                      system
                    </NativeSelectOption>
                    <NativeSelectOption value='developer'>
                      developer
                    </NativeSelectOption>
                  </NativeSelect>
                  <FieldError errors={[errors.instruction_role]} />
                </Field>
                <Field data-invalid={Boolean(errors.protocol)}>
                  <FieldLabel htmlFor='mq-protocol'>{t('Protocol')}</FieldLabel>
                  <NativeSelect
                    id='mq-protocol'
                    className='w-full'
                    {...form.register('protocol')}
                  >
                    <NativeSelectOption value='responses'>
                      Responses
                    </NativeSelectOption>
                    <NativeSelectOption value='chat'>
                      Chat Completions
                    </NativeSelectOption>
                  </NativeSelect>
                  <FieldError errors={[errors.protocol]} />
                </Field>
                <Field data-invalid={Boolean(errors.reasoning_effort)}>
                  <FieldLabel htmlFor='mq-effort'>
                    {t('Reasoning effort')}
                  </FieldLabel>
                  <NativeSelect
                    id='mq-effort'
                    className='w-full'
                    {...form.register('reasoning_effort')}
                  >
                    <NativeSelectOption value=''>
                      {t('Upstream default')}
                    </NativeSelectOption>
                    {(
                      [
                        'none',
                        'minimal',
                        'low',
                        'medium',
                        'high',
                        'xhigh',
                        'max',
                      ] as const
                    ).map((effort) => (
                      <NativeSelectOption key={effort} value={effort}>
                        {effort}
                      </NativeSelectOption>
                    ))}
                  </NativeSelect>
                  <FieldError errors={[errors.reasoning_effort]} />
                </Field>
                <Field data-invalid={Boolean(errors.max_output_tokens)}>
                  <FieldLabel htmlFor='mq-max-tokens'>
                    {t('Max output tokens')}
                  </FieldLabel>
                  <Input
                    id='mq-max-tokens'
                    type='number'
                    min={1}
                    max={131072}
                    aria-invalid={Boolean(errors.max_output_tokens)}
                    {...form.register('max_output_tokens', {
                      setValueAs: (value) =>
                        value === '' ||
                        value == null ||
                        Number.isNaN(Number(value))
                          ? undefined
                          : Number(value),
                    })}
                  />
                  <FieldDescription>
                    {t('Leave empty for no upstream output limit.')}
                  </FieldDescription>
                  <FieldError errors={[errors.max_output_tokens]} />
                </Field>
                <Field data-invalid={Boolean(errors.temperature)}>
                  <FieldLabel htmlFor='mq-temperature'>
                    {t('Temperature')}
                  </FieldLabel>
                  <Input
                    id='mq-temperature'
                    type='number'
                    step='0.1'
                    min={0}
                    max={2}
                    aria-invalid={Boolean(errors.temperature)}
                    {...form.register('temperature', {
                      setValueAs: (value) =>
                        value === '' ||
                        value == null ||
                        Number.isNaN(Number(value))
                          ? undefined
                          : Number(value),
                    })}
                  />
                  <FieldError errors={[errors.temperature]} />
                </Field>
                <Field data-invalid={Boolean(errors.top_p)}>
                  <FieldLabel htmlFor='mq-topp'>{t('Top P')}</FieldLabel>
                  <Input
                    id='mq-topp'
                    type='number'
                    step='0.05'
                    min={0}
                    max={1}
                    aria-invalid={Boolean(errors.top_p)}
                    {...form.register('top_p', {
                      setValueAs: (value) =>
                        value === '' ||
                        value == null ||
                        Number.isNaN(Number(value))
                          ? undefined
                          : Number(value),
                    })}
                  />
                  <FieldError errors={[errors.top_p]} />
                </Field>
              </FieldGroup>

              <FieldGroup className='grid gap-4 sm:grid-cols-3'>
                <Field data-invalid={Boolean(errors.samples_per_target)}>
                  <FieldLabel htmlFor='mq-samples'>
                    {t('Samples per target')}
                  </FieldLabel>
                  <Input
                    id='mq-samples'
                    type='number'
                    min={1}
                    max={10}
                    aria-invalid={Boolean(errors.samples_per_target)}
                    {...form.register('samples_per_target', {
                      valueAsNumber: true,
                    })}
                  />
                  <FieldError errors={[errors.samples_per_target]} />
                </Field>
                <Field data-invalid={Boolean(errors.timeout_seconds)}>
                  <FieldLabel htmlFor='mq-timeout'>
                    {t('Timeout (seconds)')}
                  </FieldLabel>
                  <Input
                    id='mq-timeout'
                    type='number'
                    min={10}
                    max={600}
                    aria-invalid={Boolean(errors.timeout_seconds)}
                    {...form.register('timeout_seconds', {
                      valueAsNumber: true,
                    })}
                  />
                  <FieldError errors={[errors.timeout_seconds]} />
                </Field>
                <Field data-invalid={Boolean(errors.daily_limit)}>
                  <FieldLabel htmlFor='mq-daily-limit'>
                    {t('Daily sample limit')}
                  </FieldLabel>
                  <Input
                    id='mq-daily-limit'
                    type='number'
                    min={1}
                    max={10000}
                    aria-invalid={Boolean(errors.daily_limit)}
                    {...form.register('daily_limit', {
                      setValueAs: (value) =>
                        value === '' ||
                        value == null ||
                        Number.isNaN(Number(value))
                          ? undefined
                          : Number(value),
                    })}
                  />
                  <FieldDescription>
                    {t('Leave empty for no case daily cap.')}
                  </FieldDescription>
                  <FieldError errors={[errors.daily_limit]} />
                </Field>
                <FieldDescription className='sm:col-span-3'>
                  {t('Each run: {{count}} samples', { count: runCount })}
                </FieldDescription>
              </FieldGroup>
            </CollapsibleContent>
          </Collapsible>

          <FieldGroup className='grid gap-4 sm:grid-cols-3'>
            <Field>
              <FieldLabel htmlFor='mq-schedule-enabled'>
                {t('Schedule')}
              </FieldLabel>
              <div className='flex items-center gap-2'>
                <Switch
                  id='mq-schedule-enabled'
                  checked={scheduleEnabled}
                  onCheckedChange={(checked) =>
                    form.setValue('schedule_enabled', checked === true, {
                      shouldDirty: true,
                    })
                  }
                />
              </div>
            </Field>
            {scheduleEnabled && (
              <>
                <Field data-invalid={Boolean(errors.schedule_kind)}>
                  <FieldLabel htmlFor='mq-schedule-kind'>
                    {t('Schedule kind')}
                  </FieldLabel>
                  <NativeSelect
                    id='mq-schedule-kind'
                    className='w-full'
                    {...form.register('schedule_kind')}
                  >
                    <NativeSelectOption value='interval'>
                      {t('Interval')}
                    </NativeSelectOption>
                    <NativeSelectOption value='daily'>
                      {t('Daily')}
                    </NativeSelectOption>
                    <NativeSelectOption value='weekly'>
                      {t('Weekly')}
                    </NativeSelectOption>
                  </NativeSelect>
                  <FieldError errors={[errors.schedule_kind]} />
                </Field>
                {scheduleKind === 'interval' && (
                  <Field data-invalid={Boolean(errors.interval_minutes)}>
                    <FieldLabel htmlFor='mq-interval'>
                      {t('Interval (minutes)')}
                    </FieldLabel>
                    <Input
                      id='mq-interval'
                      type='number'
                      min={1}
                      max={10080}
                      aria-invalid={Boolean(errors.interval_minutes)}
                      {...form.register('interval_minutes', {
                        valueAsNumber: true,
                      })}
                    />
                    <FieldError errors={[errors.interval_minutes]} />
                  </Field>
                )}
                {scheduleKind !== 'interval' && (
                  <Field data-invalid={Boolean(errors.schedule_time)}>
                    <FieldLabel htmlFor='mq-time'>{t('Time')}</FieldLabel>
                    <Input
                      id='mq-time'
                      type='time'
                      step={60}
                      aria-invalid={Boolean(errors.schedule_time)}
                      {...form.register('schedule_time')}
                    />
                    <FieldError errors={[errors.schedule_time]} />
                  </Field>
                )}
                {scheduleKind === 'weekly' && (
                  <Field
                    className='sm:col-span-3'
                    data-invalid={Boolean(errors.weekdays)}
                  >
                    <FieldLabel>{t('Weekdays')}</FieldLabel>
                    <div className='flex flex-wrap gap-3'>
                      {WEEKDAY_KEYS.map((key, index) => {
                        const day = index + 1
                        return (
                          <label
                            key={key}
                            className='flex items-center gap-1 text-sm'
                          >
                            <Checkbox
                              checked={form.watch('weekdays').includes(day)}
                              onCheckedChange={(checked) =>
                                toggleWeekday(day, checked === true)
                              }
                            />
                            {t(key)}
                          </label>
                        )
                      })}
                    </div>
                    <FieldError errors={[errors.weekdays]} />
                  </Field>
                )}
                <Field data-invalid={Boolean(errors.timezone)}>
                  <FieldLabel htmlFor='mq-timezone'>{t('Timezone')}</FieldLabel>
                  <Input
                    id='mq-timezone'
                    aria-invalid={Boolean(errors.timezone)}
                    {...form.register('timezone')}
                  />
                  <FieldError errors={[errors.timezone]} />
                </Field>
              </>
            )}
            {props.qualityCase && (
              <FieldDescription className='sm:col-span-3'>
                {t('Next run')}:{' '}
                {props.qualityCase.next_run_at > 0
                  ? formatTimestampToDate(
                      props.qualityCase.next_run_at,
                      'milliseconds'
                    )
                  : '—'}
                {props.qualityCase.last_schedule_reason !== '' && (
                  <span className='text-muted-foreground'>
                    {' · '}
                    {props.qualityCase.last_schedule_reason}
                  </span>
                )}
              </FieldDescription>
            )}
          </FieldGroup>
        </fieldset>

        <div className='flex flex-wrap items-center gap-2 border-t pt-3'>
          <Button
            type='submit'
            size='sm'
            disabled={!canOperate || saveMutation.isPending}
          >
            {t('Save')}
          </Button>
          <Button
            type='button'
            size='sm'
            variant='ghost'
            disabled={!form.formState.isDirty}
            onClick={() => form.reset(defaultValues)}
          >
            {t('Discard')}
          </Button>
          {props.qualityCase && (
            <>
              <Button
                type='button'
                size='sm'
                variant='outline'
                disabled={!canOperate || duplicateMutation.isPending}
                onClick={() => duplicateMutation.mutate()}
              >
                {t('Duplicate')}
              </Button>
              <Button
                type='button'
                size='sm'
                variant='outline'
                className='text-destructive'
                disabled={!canOperate || props.qualityCase.active_run_id !== ''}
                title={
                  props.qualityCase.active_run_id !== ''
                    ? t('Stop the active run first')
                    : undefined
                }
                onClick={() => setDeleteOpen(true)}
              >
                {t('Delete')}
              </Button>
            </>
          )}
        </div>
      </form>
      <ConfirmDialog
        open={deleteOpen}
        onOpenChange={setDeleteOpen}
        title={t('Delete case')}
        desc={t(
          'The case is removed from the list; its runs and samples remain in Batch Records.'
        )}
        destructive
        isLoading={deleteMutation.isPending}
        handleConfirm={() => deleteMutation.mutate()}
      />
    </FormProvider>
  )
}
