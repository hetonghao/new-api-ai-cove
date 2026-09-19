import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import {
  act,
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
  within,
} from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { AxiosError, AxiosHeaders } from 'axios'
import i18n from 'i18next'
import { afterEach, beforeEach, expect, it, vi } from 'vitest'

import zhCN from '@/i18n/locales/zh.json'
import { api } from '@/lib/api'
import { useAuthStore } from '@/stores/auth-store'

import { MediaModelsPanel } from '../components/media-models-panel'
import {
  parseMediaModelsJson,
  validateMediaPolicyDraft,
  type MediaModelProfile,
  type MediaParameter,
  type MediaPolicy,
  type MediaPolicySnapshot,
} from '../lib/media-policy'
import { MODELS_SECTION_IDS } from '../section-registry'

const clients: QueryClient[] = []
const empty: MediaPolicySnapshot = {
  config_version: 'version-one',
  policy: { version: 1, models: [], image_priority: [], video_priority: [] },
}

function imageProfile(id: string, hint = ''): MediaModelProfile {
  return {
    id,
    type: 'image',
    selection_hint: hint || undefined,
    operations: {
      text_to_image: {
        protocol: 'openai_images',
        path: '/v1/images/generations',
        parameters: {},
        reference: { input: 'none', max_images: 0 },
      },
    },
  }
}

function videoProfile(id: string): MediaModelProfile {
  const parameters: Record<string, MediaParameter> = {
    seconds: { type: 'integer', minimum: 1, maximum: 15 },
    size: {
      type: 'string',
      enum: ['720x1280', '1280x720', '1024x1792', '1792x1024'],
    },
    aspect_ratio: {
      type: 'string',
      enum: ['1:1', '16:9', '9:16', '4:3', '3:4', '3:2', '2:3'],
    },
    resolution: { type: 'string', enum: ['480p', '720p'] },
  }
  return {
    id,
    type: 'video',
    operations: {
      text_to_video: {
        protocol: 'openai_video',
        path: '/v1/videos',
        parameters,
        reference: { input: 'none', max_images: 0 },
      },
      image_to_video: {
        protocol: 'openai_video',
        path: '/v1/videos',
        parameters,
        reference: { input: 'url', max_images: 1 },
      },
    },
  }
}

const imageA = imageProfile('gpt-image-a')
const imageB = imageProfile('gpt-image-b')
const videoV = videoProfile('vid-model-a')
const registered: MediaPolicySnapshot = {
  config_version: 'version-one',
  policy: {
    version: 1,
    models: [imageA, imageB, videoV],
    image_priority: [],
    video_priority: [],
  },
}

function mount(snapshot = empty, options: { loadError?: Error } = {}) {
  const get = vi.spyOn(api, 'get').mockImplementation((url: string) => {
    if (url === '/api/option/media_models') {
      if (options.loadError) return Promise.reject(options.loadError)
      return Promise.resolve({ data: { success: true, data: snapshot } })
    }
    return Promise.reject(new Error(`unexpected GET ${url}`))
  })
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  })
  clients.push(client)
  render(
    <QueryClientProvider client={client}>
      <MediaModelsPanel />
    </QueryClientProvider>
  )
  return get
}

async function choose(comboboxName: string, option: string) {
  await userEvent.click(screen.getByLabelText(comboboxName))
  await userEvent.click(await screen.findByRole('option', { name: option }))
}

function priorityList(name: string) {
  return within(screen.getByRole('list', { name }))
}

function putPolicy(put: { mock: { calls: unknown[][] } }): MediaPolicy {
  return (put.mock.calls[0][1] as { policy: MediaPolicy }).policy
}

beforeEach(() => {
  useAuthStore.getState().auth.setUser({ id: 1, username: 'root', role: 100 })
  i18n.addResourceBundle('zhCN', 'translation', zhCN.translation, true, true)
})

afterEach(async () => {
  await i18n.changeLanguage('en')
  cleanup()
  clients.splice(0).forEach((client) => client.clear())
  useAuthStore.getState().auth.reset()
  vi.restoreAllMocks()
})

it('registers media as the fourth model section without reordering existing tabs', () => {
  expect(MODELS_SECTION_IDS).toEqual([
    'metadata',
    'vendors',
    'deployments',
    'media',
  ])
})

it('does not request protected endpoints for a non-root administrator', async () => {
  useAuthStore.getState().auth.setUser({ id: 2, username: 'admin', role: 10 })
  const get = mount()
  expect(
    await screen.findByText(
      'Only super administrators can configure media models.'
    )
  ).toBeInTheDocument()
  expect(get).not.toHaveBeenCalled()
})

it('shows no secondary registration, JSON, or template editors', async () => {
  const get = mount(registered)
  await screen.findByLabelText('Add image model')
  expect(get.mock.calls.some(([url]) => url === '/api/models/')).toBe(false)
  expect(screen.queryByText('Media capability registry')).toBeNull()
  expect(screen.queryByText('Templates')).toBeNull()
  expect(screen.queryByLabelText('Model for capability template')).toBeNull()
  expect(screen.queryByText('Registered media models')).toBeNull()
})

it('disables pickers and points at Models setup when no media models exist', async () => {
  mount()
  expect(
    await screen.findByText(
      'No Image models found. Configure models and channels in Models first.'
    )
  ).toBeInTheDocument()
  expect(screen.getByLabelText('Add image model')).toBeDisabled()
  expect(screen.getByLabelText('Add video model')).toBeDisabled()
})

it('offers only generated profiles of the matching media type', async () => {
  mount(registered)
  await screen.findByLabelText('Add image model')
  await userEvent.click(screen.getByLabelText('Add image model'))
  expect(
    await screen.findByRole('option', { name: 'gpt-image-a' })
  ).toBeInTheDocument()
  expect(
    screen.getByRole('option', { name: 'gpt-image-b' })
  ).toBeInTheDocument()
  expect(screen.queryByRole('option', { name: 'vid-model-a' })).toBeNull()
})

it('adds ranked rows without creating a model profile and saves hints', async () => {
  mount(registered)
  const savedPolicy: MediaPolicy = {
    version: 1,
    models: [
      imageA,
      { ...imageB, selection_hint: 'Use for portraits' },
      videoV,
    ],
    image_priority: ['gpt-image-b', 'gpt-image-a'],
    video_priority: [],
  }
  const put = vi.spyOn(api, 'put').mockResolvedValue({
    data: {
      success: true,
      data: { config_version: 'version-two', policy: savedPolicy },
    },
  })
  await screen.findByLabelText('Add image model')
  await choose('Add image model', 'gpt-image-a')
  await choose('Add image model', 'gpt-image-b')
  const list = priorityList('Image model priority')
  expect(list.getByText('1.')).toBeInTheDocument()
  expect(list.getByText('2.')).toBeInTheDocument()
  await userEvent.click(
    screen.getByRole('button', { name: 'Move gpt-image-b up' })
  )
  fireEvent.change(screen.getByLabelText('Selection hint for gpt-image-b'), {
    target: { value: 'Use for portraits' },
  })
  await userEvent.click(screen.getByRole('button', { name: 'Save' }))
  expect(
    await screen.findByText('Media model policy saved.')
  ).toBeInTheDocument()
  expect(put).toHaveBeenCalledWith('/api/option/media_models', {
    expected_version: 'version-one',
    policy: savedPolicy,
  })
})

it('keeps the generated profile and its hint after removing only priority', async () => {
  mount(registered)
  const put = vi.spyOn(api, 'put').mockResolvedValue({
    data: { success: true, data: empty },
  })
  await screen.findByLabelText('Add image model')
  await choose('Add image model', 'gpt-image-a')
  fireEvent.change(screen.getByLabelText('Selection hint for gpt-image-a'), {
    target: { value: 'keep me' },
  })
  await userEvent.click(
    screen.getByRole('button', { name: 'Remove gpt-image-a from priority' })
  )
  expect(
    screen.queryByRole('button', {
      name: 'Remove gpt-image-a from priority',
    })
  ).toBeNull()
  await userEvent.click(screen.getByRole('button', { name: 'Save' }))
  await waitFor(() => expect(put).toHaveBeenCalled())
  const policy = putPolicy(put)
  expect(policy.image_priority).toEqual([])
  expect(policy.models).toHaveLength(3)
  expect(policy.models[0].selection_hint).toBe('keep me')
})

it('keeps an unregistered priority row removable', async () => {
  const stale: MediaPolicySnapshot = {
    config_version: 'version-one',
    policy: {
      version: 1,
      models: [imageA],
      image_priority: ['ghost-model'],
      video_priority: [],
    },
  }
  mount(stale)
  const put = vi.spyOn(api, 'put').mockResolvedValue({
    data: { success: true, data: empty },
  })
  expect(
    await screen.findByText('Model is no longer registered.')
  ).toBeInTheDocument()
  await userEvent.click(
    screen.getByRole('button', { name: 'Remove ghost-model from priority' })
  )
  expect(screen.queryByText('Model is no longer registered.')).toBeNull()
  await userEvent.click(screen.getByRole('button', { name: 'Save' }))
  await waitFor(() => expect(put).toHaveBeenCalled())
  expect(putPolicy(put).image_priority).toEqual([])
})

it('offers no hint editor on a type-mismatched priority row but keeps removal', async () => {
  const mismatched: MediaPolicySnapshot = {
    config_version: 'version-one',
    policy: {
      version: 1,
      models: [videoV],
      image_priority: ['vid-model-a'],
      video_priority: [],
    },
  }
  mount(mismatched)
  const put = vi.spyOn(api, 'put').mockResolvedValue({
    data: { success: true, data: empty },
  })
  const list = await screen.findByRole('list', {
    name: 'Image model priority',
  })
  expect(
    within(list).getByText('Model is no longer registered.')
  ).toBeInTheDocument()
  expect(
    within(list).queryByLabelText('Selection hint for vid-model-a')
  ).toBeNull()
  await userEvent.click(
    screen.getByRole('button', { name: 'Remove vid-model-a from priority' })
  )
  await userEvent.click(screen.getByRole('button', { name: 'Save' }))
  await waitFor(() => expect(put).toHaveBeenCalled())
  const policy = putPolicy(put)
  expect(policy.image_priority).toEqual([])
  expect(policy.models).toHaveLength(1)
  expect(policy.models[0].id).toBe('vid-model-a')
})

it('flags over-limit hints and allows saving once corrected', async () => {
  mount(registered)
  const put = vi.spyOn(api, 'put').mockResolvedValue({
    data: { success: true, data: empty },
  })
  await screen.findByLabelText('Add image model')
  await choose('Add image model', 'gpt-image-a')
  fireEvent.change(screen.getByLabelText('Selection hint for gpt-image-a'), {
    target: { value: 'x'.repeat(2001) },
  })
  expect(
    await screen.findByText('Selection hint exceeds the 2000 character limit.')
  ).toBeInTheDocument()
  expect(
    screen.getByLabelText('Selection hint for gpt-image-a')
  ).toHaveAttribute('aria-invalid', 'true')
  await userEvent.click(screen.getByRole('button', { name: 'Save' }))
  expect(
    await screen.findByText(
      'Invalid media policy. Check model IDs, operations and parameter limits.'
    )
  ).toBeInTheDocument()
  expect(put).not.toHaveBeenCalled()
  fireEvent.change(screen.getByLabelText('Selection hint for gpt-image-a'), {
    target: { value: 'short' },
  })
  await userEvent.click(screen.getByRole('button', { name: 'Save' }))
  await waitFor(() => expect(put).toHaveBeenCalled())
})

it('preserves the draft after a conflict and requires confirmation before reloading', async () => {
  mount(registered)
  const response = {
    status: 409,
    statusText: 'Conflict',
    headers: new AxiosHeaders(),
    config: { headers: new AxiosHeaders() },
    data: { code: 'media_policy_conflict' },
  }
  vi.spyOn(api, 'put').mockRejectedValue(
    new AxiosError('conflict', undefined, response.config, undefined, response)
  )
  await screen.findByLabelText('Add image model')
  await choose('Add image model', 'gpt-image-a')
  await userEvent.click(screen.getByRole('button', { name: 'Save' }))
  expect(
    await screen.findByText(
      'This policy changed elsewhere. Reload before saving.'
    )
  ).toBeInTheDocument()
  expect(screen.getByRole('button', { name: 'Save' })).toBeDisabled()
  await userEvent.click(screen.getByRole('button', { name: 'Reload' }))
  expect(
    await screen.findByText('Discard unsaved changes?')
  ).toBeInTheDocument()
  await userEvent.click(screen.getByRole('button', { name: 'Cancel' }))
  expect(
    priorityList('Image model priority').getByText('gpt-image-a')
  ).toBeInTheDocument()
})

it('disables priority controls while a save is pending', async () => {
  mount(registered)
  let resolvePut: (value: unknown) => void = () => {}
  vi.spyOn(api, 'put').mockImplementation(
    () => new Promise((resolve) => (resolvePut = resolve))
  )
  await screen.findByLabelText('Add image model')
  await choose('Add image model', 'gpt-image-a')
  await userEvent.click(screen.getByRole('button', { name: 'Save' }))
  expect(
    await screen.findByRole('button', { name: 'Saving...' })
  ).toBeDisabled()
  expect(screen.getByLabelText('Add image model')).toBeDisabled()
  expect(
    screen.getByRole('button', { name: 'Remove gpt-image-a from priority' })
  ).toBeDisabled()
  expect(screen.getByLabelText('Selection hint for gpt-image-a')).toBeDisabled()
  resolvePut({ data: { success: true, data: empty } })
  expect(
    await screen.findByText('Media model policy saved.')
  ).toBeInTheDocument()
})

it('selects options from the keyboard without submitting the form', async () => {
  mount(registered)
  const put = vi.spyOn(api, 'put').mockResolvedValue({
    data: { success: true, data: empty },
  })
  const user = userEvent.setup()
  await screen.findByLabelText('Add image model')
  await user.click(screen.getByLabelText('Add image model'))
  await user.type(screen.getByLabelText('Add image model'), 'gpt-image-a')
  await user.keyboard('{ArrowDown}{Enter}')
  expect(
    within(
      await screen.findByRole('list', { name: 'Image model priority' })
    ).getByText('gpt-image-a')
  ).toBeInTheDocument()
  expect(put).not.toHaveBeenCalled()
  await user.click(screen.getByRole('button', { name: 'Save' }))
  await waitFor(() => expect(put).toHaveBeenCalled())
})

it('stacks priority sections vertically and keeps model and hint on one desktop row', async () => {
  mount(registered)
  await screen.findByLabelText('Add image model')
  await choose('Add image model', 'gpt-image-a')
  const list = screen.getByRole('list', { name: 'Image model priority' })
  expect(list).toHaveClass('divide-y')
  const row = list.querySelector('li')
  expect(row).toHaveClass('grid')
  expect(row?.className).toContain('sm:grid-cols-[')
  const hint = screen.getByLabelText('Selection hint for gpt-image-a')
  expect(hint.closest('div')?.parentElement).toHaveClass(
    'sm:col-start-3',
    'sm:row-start-1'
  )
})

it('truncates long model identifiers inside priority rows', async () => {
  const longId = 'a-very-long-image-model-identifier-that-must-wrap-0123456789'
  mount({
    config_version: 'version-one',
    policy: {
      version: 1,
      models: [imageProfile(longId)],
      image_priority: [],
      video_priority: [],
    },
  })
  await screen.findByLabelText('Add image model')
  await choose('Add image model', longId)
  expect(priorityList('Image model priority').getByText(longId)).toHaveClass(
    'truncate'
  )
})

it('renders localized empty states without raw media type text', async () => {
  mount()
  await screen.findByLabelText('Add image model')
  await act(async () => {
    await i18n.changeLanguage('zhCN')
  })
  expect(
    await screen.findByText(/没有可用的图片模型。请先在/)
  ).toBeInTheDocument()
  expect(screen.getByText(/没有可用的视频模型。请先在/)).toBeInTheDocument()
  expect(screen.queryByText(/No (Image|Video) models found/)).toBeNull()
})

it('shows generated capabilities in a read-only collapsible per media type', async () => {
  mount(registered)
  await screen.findByLabelText('Add image model')
  const triggers = screen.getAllByRole('button', {
    name: /Generated capabilities/,
  })
  expect(triggers).toHaveLength(2)
  expect(screen.queryByText('image_to_video')).toBeNull()
  await userEvent.click(triggers[1])
  expect(await screen.findByText('image_to_video')).toBeInTheDocument()
  expect(screen.getAllByText(/seconds: 1–15/)).toHaveLength(2)
  expect(screen.getAllByText(/resolution: 480p \| 720p/)).toHaveLength(2)
  expect(screen.getByText(/url input, max 1/)).toBeInTheDocument()
  await userEvent.click(triggers[0])
  expect((await screen.findAllByText('text_to_image')).length).toBeGreaterThan(
    0
  )
  expect(screen.getAllByText(/Not accepted/).length).toBeGreaterThan(0)
})

it('hides capability details when no media models of that type exist', async () => {
  mount(empty)
  await screen.findByLabelText('Add image model')
  expect(
    screen.queryByRole('button', { name: /Generated capabilities/ })
  ).toBeNull()
})

it('shows a policy load error and can retry', async () => {
  mount(empty, { loadError: new Error('offline') })
  await waitFor(() =>
    expect(screen.getByRole('button', { name: 'Retry' })).toBeInTheDocument()
  )
  vi.mocked(api.get).mockResolvedValue({
    data: { success: true, data: empty },
  })
  await userEvent.click(screen.getByRole('button', { name: 'Retry' }))
  expect(await screen.findByLabelText('Add image model')).toBeInTheDocument()
})

it('validates malformed registries, priorities and hints at the form boundary', () => {
  const draft = { imagePriority: '', videoPriority: '', modelsJson: 'not JSON' }
  expect(validateMediaPolicyDraft(draft).issues[0].code).toBe('invalid_json')
  draft.modelsJson = JSON.stringify([imageA])
  draft.imagePriority = 'gpt-image-a\ngpt-image-a'
  expect(validateMediaPolicyDraft(draft).issues[0].code).toBe(
    'duplicate_priority'
  )
  draft.imagePriority = 'ghost-model'
  expect(validateMediaPolicyDraft(draft).issues[0].code).toBe(
    'unknown_priority'
  )
  draft.imagePriority = ''
  draft.videoPriority = 'gpt-image-a'
  expect(validateMediaPolicyDraft(draft).issues[0].code).toBe(
    'priority_type_mismatch'
  )
  draft.videoPriority = ''
  draft.modelsJson = JSON.stringify([{ ...imageA, selection_hint: 5 }])
  expect(validateMediaPolicyDraft(draft).issues[0].code).toBe(
    'malformed_profile'
  )
  const overlong = JSON.stringify([
    { ...imageA, selection_hint: 'x'.repeat(2001) },
  ])
  const parsed = parseMediaModelsJson(overlong)
  expect(parsed.models).toHaveLength(1)
  expect(parsed.issue?.code).toBe('selection_hint_too_long')
  draft.modelsJson = overlong
  expect(validateMediaPolicyDraft(draft).issues[0].code).toBe(
    'selection_hint_too_long'
  )
})
