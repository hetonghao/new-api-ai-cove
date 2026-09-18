import { zodResolver } from '@hookform/resolvers/zod'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { isAxiosError } from 'axios'
import { useEffect, useState } from 'react'
import { useForm } from 'react-hook-form'
import { useTranslation } from 'react-i18next'

import { ConfirmDialog } from '@/components/confirm-dialog'
import { ErrorState } from '@/components/error-state'
import { LoadingState } from '@/components/loading-state'
import { Button } from '@/components/ui/button'
import { Form } from '@/components/ui/form'
import { ROLE } from '@/lib/roles'
import {
  getServerErrorMessage,
  requireServerSuccess,
} from '@/lib/server-error-message'
import { useAuthStore } from '@/stores/auth-store'

import {
  mediaPolicyDraftSchema,
  mediaPolicyToDraft,
  validateMediaPolicyDraft,
  type MediaPolicyDraftValues,
  type MediaPolicySnapshot,
} from '../lib/media-policy'
import {
  getMediaModelsPolicy,
  mediaModelsPolicyQueryKey,
  updateMediaModelsPolicy,
} from '../media-api'
import { MediaPolicyFields } from './media-policy-fields'

export function MediaModelsPanel() {
  const { t } = useTranslation()
  const canEdit = useAuthStore(
    (state) => state.auth.user?.role === ROLE.SUPER_ADMIN
  )
  const queryClient = useQueryClient()
  const [baseline, setBaseline] = useState<MediaPolicySnapshot | null>(null)
  const [confirmReload, setConfirmReload] = useState(false)
  const [saveError, setSaveError] = useState<string | null>(null)
  const [conflict, setConflict] = useState(false)
  const form = useForm<MediaPolicyDraftValues>({
    resolver: zodResolver(mediaPolicyDraftSchema),
    defaultValues: { imagePriority: '', videoPriority: '', modelsJson: '[]' },
  })
  const query = useQuery({
    queryKey: mediaModelsPolicyQueryKey,
    enabled: canEdit,
    retry: false,
    meta: { errorToast: false },
    queryFn: async () => {
      const response = requireServerSuccess(await getMediaModelsPolicy())
      if (!response.data) {
        throw new Error(t('Media model policy is unavailable.'))
      }
      return response.data
    },
  })
  useEffect(() => {
    if (query.data && !baseline) {
      setBaseline(query.data)
      form.reset(mediaPolicyToDraft(query.data.policy))
    }
  }, [query.data, baseline, form])

  const mutation = useMutation({
    retry: false,
    meta: { errorToast: false },
    mutationFn: async (values: MediaPolicyDraftValues) => {
      const validation = validateMediaPolicyDraft(values)
      if (!validation.policy || !baseline) {
        throw new Error(
          t('Invalid media policy. Check the highlighted fields.')
        )
      }
      const response = requireServerSuccess(
        await updateMediaModelsPolicy(
          baseline.config_version,
          validation.policy
        )
      )
      if (!response.data) {
        throw new Error(t('Media model policy is unavailable.'))
      }
      return response.data
    },
    onSuccess: (snapshot) => {
      queryClient.setQueryData(mediaModelsPolicyQueryKey, snapshot)
      setBaseline(snapshot)
      form.reset(mediaPolicyToDraft(snapshot.policy))
      setSaveError(null)
      setConflict(false)
    },
    onError: (error) => {
      const isConflict = isAxiosError(error) && error.response?.status === 409
      setConflict(isConflict)
      setSaveError(
        isConflict
          ? t('This policy changed elsewhere. Reload before saving.')
          : getServerErrorMessage(error, t('Failed to save media models.'))
      )
    },
  })

  async function reload() {
    setConfirmReload(false)
    const result = await query.refetch()
    if (result.data && !result.isError) {
      setBaseline(result.data)
      form.reset(mediaPolicyToDraft(result.data.policy))
      setSaveError(null)
      setConflict(false)
    }
  }

  function submit(values: MediaPolicyDraftValues) {
    form.clearErrors()
    const validation = validateMediaPolicyDraft(values)
    if (validation.issues.length) {
      for (const issue of validation.issues) {
        if (issue.field === 'modelsJson') {
          setSaveError(
            t(
              'Invalid media policy. Check model IDs, operations and parameter limits.'
            )
          )
          continue
        }
        form.setError(issue.field, {
          message: t(
            'Invalid media policy. Check model IDs, operations and parameter limits.'
          ),
        })
      }
      return
    }
    setSaveError(null)
    mutation.mutate(values)
  }

  if (!canEdit) {
    return (
      <ErrorState
        description={t('Only super administrators can configure media models.')}
      />
    )
  }
  if (query.isPending) return <LoadingState />
  if (query.isError && !baseline) {
    return (
      <ErrorState
        description={getServerErrorMessage(query.error)}
        onRetry={() => void query.refetch()}
      />
    )
  }
  if (!baseline) return <LoadingState />

  return (
    <div
      className='h-full min-h-0 overflow-y-auto pr-1'
      aria-label={t('Media models')}
    >
      <Form {...form}>
        <form
          onSubmit={form.handleSubmit(submit)}
          className='mx-auto flex max-w-5xl flex-col gap-6 pb-8'
        >
          <p className='text-muted-foreground text-sm'>
            {t(
              'Choose existing image and video models. Capabilities are generated automatically; configure only priority and selection hints.'
            )}
          </p>
          <MediaPolicyFields
            form={form}
            disabled={mutation.isPending || query.isFetching}
          />
          {saveError && (
            <p role='alert' className='text-destructive text-sm'>
              {saveError}
            </p>
          )}
          {query.isError && (
            <p role='alert' className='text-destructive text-sm'>
              {getServerErrorMessage(query.error)}
            </p>
          )}
          {mutation.isSuccess && !form.formState.isDirty && (
            <p role='status' className='text-sm'>
              {t('Media model policy saved.')}
            </p>
          )}
          <div className='flex flex-wrap gap-2'>
            <Button
              type='submit'
              disabled={
                mutation.isPending ||
                query.isFetching ||
                conflict ||
                !form.formState.isDirty
              }
            >
              {mutation.isPending ? t('Saving...') : t('Save')}
            </Button>
            <Button
              type='button'
              variant='outline'
              disabled={mutation.isPending || query.isFetching}
              onClick={() =>
                form.formState.isDirty ? setConfirmReload(true) : void reload()
              }
            >
              {t('Reload')}
            </Button>
          </div>
        </form>
      </Form>
      <ConfirmDialog
        open={confirmReload}
        onOpenChange={setConfirmReload}
        title={t('Discard unsaved changes?')}
        desc={t('Reload the latest media policy and discard this draft.')}
        confirmText={t('Reload')}
        handleConfirm={() => void reload()}
      />
    </div>
  )
}
