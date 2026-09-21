import {
  ArrowDown01Icon,
  ArrowUp01Icon,
  Cancel01Icon,
} from '@hugeicons/core-free-icons'
import { HugeiconsIcon } from '@hugeicons/react'
import type { TFunction } from 'i18next'
import { ChevronDown } from 'lucide-react'
import { useId, useMemo, useState, type KeyboardEvent } from 'react'
import { useTranslation } from 'react-i18next'

import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import {
  Collapsible,
  CollapsibleContent,
  CollapsibleTrigger,
} from '@/components/ui/collapsible'
import { Combobox } from '@/components/ui/combobox'
import { Label } from '@/components/ui/label'
import { Textarea } from '@/components/ui/textarea'

import type {
  MediaModelProfile,
  MediaModelType,
  MediaOperation,
  MediaParameter,
  MediaReference,
} from '../lib/media-policy'

export const MEDIA_SELECTION_HINT_MAX = 2000

function describeMediaParameter(
  name: string,
  parameter: MediaParameter
): string {
  if (parameter.type === 'integer') {
    return `${name}: ${parameter.minimum}–${parameter.maximum}`
  }
  if (parameter.enum && parameter.enum.length > 0) {
    return `${name}: ${parameter.enum.join(' | ')}`
  }
  return `${name}: ${parameter.type}`
}

function describeMediaParameters(
  t: TFunction,
  operation: MediaOperation
): string {
  const entries = Object.entries(operation.parameters)
  if (entries.length === 0) return t('None')
  return entries
    .map(([name, parameter]) => describeMediaParameter(name, parameter))
    .join(', ')
}

function describeMediaReference(
  t: TFunction,
  reference: MediaReference
): string {
  if (reference.input === 'none') return t('Not accepted')
  return t('{{mode}} input, max {{count}}', {
    mode: reference.input,
    count: reference.max_images,
  })
}

export function MediaCapabilityDetails(props: {
  type: MediaModelType
  models: MediaModelProfile[]
}) {
  const { t } = useTranslation()
  const [open, setOpen] = useState(false)
  const typeModels = useMemo(
    () => props.models.filter((profile) => profile.type === props.type),
    [props.models, props.type]
  )
  if (typeModels.length === 0) return null
  return (
    <Collapsible
      open={open}
      onOpenChange={setOpen}
      className='rounded-lg border'
    >
      <CollapsibleTrigger
        render={
          <Button
            type='button'
            variant='ghost'
            className='h-auto w-full justify-between rounded-lg px-3 py-2.5 text-sm'
          />
        }
      >
        <span>
          <span className='block text-left font-medium'>
            {t('Generated capabilities')}
          </span>
          <span className='text-muted-foreground block text-left text-xs font-normal'>
            {t(
              'Read-only. Resolved from model endpoints and channel routing support.'
            )}
          </span>
        </span>
        <ChevronDown
          className={open ? 'size-4 rotate-180' : 'size-4'}
          aria-hidden='true'
        />
      </CollapsibleTrigger>
      <CollapsibleContent className='border-t px-3 py-2.5'>
        <ul className='grid gap-x-6 gap-y-4 md:grid-cols-2'>
          {typeModels.map((profile) => (
            <li key={profile.id} className='min-w-0 space-y-1'>
              <p className='font-mono text-xs font-medium break-all'>
                {profile.id}
              </p>
              <ul className='space-y-1.5'>
                {Object.entries(profile.operations).map(([name, operation]) => (
                  <li key={name} className='break-words border-border border-l-2 pl-2'>
                    <p className='text-xs'>
                      <span className='font-medium'>{name}</span>{' '}
                      <span className='text-muted-foreground'>
                        {operation.protocol} · {operation.path}
                      </span>
                    </p>
                    <p className='text-muted-foreground text-xs'>
                      {t('Parameters')}:{' '}
                      {describeMediaParameters(t, operation)}
                    </p>
                    <p className='text-muted-foreground text-xs'>
                      {t('Reference images')}:{' '}
                      {describeMediaReference(t, operation.reference)}
                    </p>
                  </li>
                ))}
              </ul>
            </li>
          ))}
        </ul>
      </CollapsibleContent>
    </Collapsible>
  )
}

export function MediaSelectionHintField(props: {
  id: string
  model: string
  value: string
  disabled: boolean
  onChange: (hint: string) => void
}) {
  const { t } = useTranslation()
  const overLimit = [...props.value].length > MEDIA_SELECTION_HINT_MAX
  return (
    <div className='min-w-0 space-y-1'>
      <Label htmlFor={props.id} className='sr-only'>
        {t('Selection hint for {{model}}', { model: props.model })}
      </Label>
      <Textarea
        id={props.id}
        rows={1}
        value={props.value}
        disabled={props.disabled}
        placeholder={t('Selection hint for {{model}}', {
          model: props.model,
        })}
        aria-invalid={overLimit || undefined}
        aria-describedby={overLimit ? `${props.id}-error` : undefined}
        className='[field-sizing:fixed] h-8 max-h-8 min-h-8 resize-none py-1'
        onChange={(event) => props.onChange(event.target.value)}
      />
      {overLimit && (
        <p
          id={`${props.id}-error`}
          role='alert'
          className='text-destructive text-xs'
        >
          {t('Selection hint exceeds the {{limit}} character limit.', {
            limit: MEDIA_SELECTION_HINT_MAX,
          })}
        </p>
      )}
    </div>
  )
}

export function MediaPriorityEditor(props: {
  type: MediaModelType
  models: MediaModelProfile[]
  value: string[]
  disabled: boolean
  onChange: (ids: string[]) => void
  onHintChange: (id: string, hint: string) => void
}) {
  const { t } = useTranslation()
  const comboboxId = useId()
  const modelsByID = useMemo(
    () => new Map(props.models.map((profile) => [profile.id, profile])),
    [props.models]
  )
  const options = useMemo(
    () =>
      props.models
        .filter(
          (profile) =>
            profile.type === props.type && !props.value.includes(profile.id)
        )
        .map((profile) => ({ value: profile.id, label: profile.id })),
    [props.models, props.type, props.value]
  )
  const addLabel =
    props.type === 'image' ? t('Add image model') : t('Add video model')
  const hasTypeModels = props.models.some(
    (profile) => profile.type === props.type
  )

  function select(id: string | null) {
    if (!id || props.disabled) return
    const profile = modelsByID.get(id)
    if (!profile || profile.type !== props.type) return
    if (props.value.includes(id)) return
    props.onChange([...props.value, id])
  }

  function move(index: number, delta: number) {
    const next = [...props.value]
    const target = index + delta
    if (target < 0 || target >= next.length) return
    const [entry] = next.splice(index, 1)
    next.splice(target, 0, entry)
    props.onChange(next)
  }

  function emptyText(): string {
    if (!hasTypeModels) {
      return t(
        'No {{type}} models found. Configure models and channels in Models first.',
        { type: props.type === 'image' ? t('Image') : t('Video') }
      )
    }
    if (props.type === 'image') return t('No image models prioritized.')
    return t('No video models prioritized.')
  }

  return (
    <div className='space-y-2'>
      <div className='flex items-center gap-2'>
        <Label htmlFor={comboboxId} className='sr-only'>
          {addLabel}
        </Label>
        <div className='w-full max-w-md min-w-0'>
          <Combobox
            id={comboboxId}
            options={options}
            value={null}
            onValueChange={select}
            disabled={props.disabled || options.length === 0}
            searchPlaceholder={t('Search registered models')}
            emptyText={t('No registered models available')}
            onKeyDown={(event: KeyboardEvent<HTMLInputElement>) => {
              if (event.key === 'Enter') event.preventDefault()
            }}
          />
        </div>
      </div>
      {props.value.length === 0 && (
        <p className='text-muted-foreground text-sm'>{emptyText()}</p>
      )}
      {props.value.length > 0 && (
        <ol
          className='divide-y rounded-md border'
          aria-label={
            props.type === 'image'
              ? t('Image model priority')
              : t('Video model priority')
          }
        >
          {props.value.map((id, index) => {
            const profile = modelsByID.get(id)
            const usableProfile =
              profile?.type === props.type ? profile : undefined
            return (
              <li
                key={id}
                className='grid min-w-0 grid-cols-[1.25rem_minmax(0,1fr)_auto] items-center gap-2 px-3 py-2 sm:grid-cols-[1.25rem_minmax(8rem,14rem)_minmax(0,1fr)_auto]'
              >
                <span className='text-muted-foreground text-sm tabular-nums'>
                  {index + 1}.
                </span>
                <Badge
                  variant='secondary'
                  className='block min-w-0 truncate'
                  title={id}
                >
                  {id}
                </Badge>
                <div className='flex shrink-0 items-center justify-end gap-1 sm:col-start-4 sm:row-start-1'>
                  <Button
                    type='button'
                    variant='ghost'
                    size='icon'
                    className='size-7'
                    disabled={props.disabled || index === 0}
                    aria-label={t('Move {{model}} up', { model: id })}
                    onClick={() => move(index, -1)}
                  >
                    <HugeiconsIcon
                      icon={ArrowUp01Icon}
                      className='size-4'
                      aria-hidden='true'
                    />
                  </Button>
                  <Button
                    type='button'
                    variant='ghost'
                    size='icon'
                    className='size-7'
                    disabled={
                      props.disabled || index === props.value.length - 1
                    }
                    aria-label={t('Move {{model}} down', { model: id })}
                    onClick={() => move(index, 1)}
                  >
                    <HugeiconsIcon
                      icon={ArrowDown01Icon}
                      className='size-4'
                      aria-hidden='true'
                    />
                  </Button>
                  <Button
                    type='button'
                    variant='ghost'
                    size='icon'
                    className='size-7'
                    disabled={props.disabled}
                    aria-label={t('Remove {{model}} from priority', {
                      model: id,
                    })}
                    onClick={() =>
                      props.onChange(
                        props.value.filter((entry) => entry !== id)
                      )
                    }
                  >
                    <HugeiconsIcon
                      icon={Cancel01Icon}
                      className='size-4'
                      aria-hidden='true'
                    />
                  </Button>
                </div>
                <div className='col-span-3 min-w-0 space-y-1 sm:col-span-1 sm:col-start-3 sm:row-start-1'>
                  {usableProfile ? (
                    <MediaSelectionHintField
                      id={`${comboboxId}-hint-${index}`}
                      model={id}
                      value={usableProfile.selection_hint ?? ''}
                      disabled={props.disabled}
                      onChange={(hint) => props.onHintChange(id, hint)}
                    />
                  ) : (
                    <p role='alert' className='text-destructive text-xs'>
                      {t('Model is no longer registered.')}
                    </p>
                  )}
                </div>
              </li>
            )
          })}
        </ol>
      )}
      <MediaCapabilityDetails type={props.type} models={props.models} />
    </div>
  )
}
