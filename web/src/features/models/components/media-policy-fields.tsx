import { useMemo } from 'react'
import type { UseFormReturn } from 'react-hook-form'
import { useTranslation } from 'react-i18next'

import { FormField, FormItem, FormMessage } from '@/components/ui/form'

import {
  parseMediaModelsJson,
  prioritiesFromText,
  prioritiesToText,
  type MediaPolicyDraftValues,
} from '../lib/media-policy'
import { MediaPriorityEditor } from './media-priority-editor'

export function MediaPolicyFields(props: {
  form: UseFormReturn<MediaPolicyDraftValues>
  disabled: boolean
}) {
  const { t } = useTranslation()
  const modelsJson = props.form.watch('modelsJson')
  const parsed = useMemo(() => parseMediaModelsJson(modelsJson), [modelsJson])

  function updateSelectionHint(id: string, hint: string) {
    if (props.disabled) return
    const current = parseMediaModelsJson(props.form.getValues('modelsJson'))
    if (!current.models) return
    const models = current.models.map((profile) =>
      profile.id === id ? { ...profile, selection_hint: hint } : profile
    )
    props.form.setValue('modelsJson', JSON.stringify(models, null, 2), {
      shouldDirty: true,
      shouldValidate: true,
    })
  }

  return (
    <>
      <div className='space-y-1'>
        <p className='text-muted-foreground text-sm'>
          {t(
            'Choose registered models in priority order. Only the first eligible model becomes the default.'
          )}
        </p>
        <p className='text-muted-foreground text-xs'>
          {t(
            'Selection hints are public model descriptions, not default recommendation labels.'
          )}
        </p>
      </div>
      <div className='flex flex-col gap-5'>
        <FormField
          control={props.form.control}
          name='imagePriority'
          render={({ field }) => (
            <FormItem>
              <h3 className='text-sm font-medium'>
                {t('Image model priority')}
              </h3>
              <MediaPriorityEditor
                type='image'
                models={parsed.models ?? []}
                value={prioritiesFromText(field.value)}
                disabled={props.disabled || !parsed.models}
                onChange={(ids) => field.onChange(prioritiesToText(ids))}
                onHintChange={updateSelectionHint}
              />
              <FormMessage />
            </FormItem>
          )}
        />
        <FormField
          control={props.form.control}
          name='videoPriority'
          render={({ field }) => (
            <FormItem>
              <h3 className='text-sm font-medium'>
                {t('Video model priority')}
              </h3>
              <MediaPriorityEditor
                type='video'
                models={parsed.models ?? []}
                value={prioritiesFromText(field.value)}
                disabled={props.disabled || !parsed.models}
                onChange={(ids) => field.onChange(prioritiesToText(ids))}
                onHintChange={updateSelectionHint}
              />
              <FormMessage />
            </FormItem>
          )}
        />
      </div>
    </>
  )
}
