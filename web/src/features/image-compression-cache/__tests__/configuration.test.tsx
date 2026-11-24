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
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { cleanup, render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { createInstance } from 'i18next'
import { I18nextProvider } from 'react-i18next'
import { afterEach, expect, test, vi } from 'vitest'
import { z } from 'zod'

import { resources } from '@/i18n/config'
import { convertDetectedLanguage } from '@/i18n/languages'
import { api } from '@/lib/api'

import { ImageCompressionCache } from '..'

afterEach(() => {
  cleanup()
  vi.restoreAllMocks()
})

async function prepareConfiguration(rejection?: string) {
  let saved = { enabled: false, max_bytes: 67108864, ttl_seconds: 900 }
  vi.spyOn(api, 'get').mockImplementation(async (url) => {
    if (url !== '/api/image-compression-cache/config') throw new Error(url)
    return { data: { success: true, data: saved } }
  })
  const saveRequest = vi
    .spyOn(api, 'put')
    .mockImplementation(async (url, config) => {
      if (url !== '/api/image-compression-cache/config') throw new Error(url)
      if (rejection === 'Save unavailable') {
        return { data: { success: false, message: rejection } }
      }
      if (rejection) {
        throw Object.assign(new Error('Config rejected'), {
          response: {
            status: 400,
            data: {
              success: false,
              message_key: rejection,
              message: rejection,
            },
          },
        })
      }
      saved = z
        .object({
          enabled: z.boolean(),
          max_bytes: z.number(),
          ttl_seconds: z.number(),
        })
        .parse(config)
      return { data: { success: true, data: saved } }
    })
  const i18n = createInstance()
  await i18n.init({ lng: 'en', fallbackLng: 'en', resources: { en: {} } })
  const editor = () => (
    <I18nextProvider i18n={i18n}>
      <QueryClientProvider client={new QueryClient()}>
        <ImageCompressionCache />
      </QueryClientProvider>
    </I18nextProvider>
  )
  return {
    editor,
    user: userEvent.setup(),
    saveRequest,
    savedCapacity: () => saved.max_bytes,
  }
}

test('saves the edited per-instance capacity and reads it back after reopening', async () => {
  const { editor, user, savedCapacity } = await prepareConfiguration()
  const view = render(editor())
  const capacity = await screen.findByRole('spinbutton', {
    name: 'Cache capacity (MiB)',
  })
  expect(capacity).toHaveValue(64)

  await user.clear(capacity)
  await user.type(capacity, '32')
  const save = screen.getByRole('button', { name: 'Save Changes' })
  await user.click(save)

  await waitFor(() => expect(save).toBeDisabled())
  view.unmount()
  render(editor())
  expect(
    await screen.findByRole('spinbutton', { name: 'Cache capacity (MiB)' })
  ).toHaveValue(32)
  expect(savedCapacity()).toBe(33554432)
})

test('keeps the edited value and offers retry when saving is rejected', async () => {
  const { editor, user } = await prepareConfiguration('Save unavailable')
  render(editor())
  const capacity = await screen.findByRole('spinbutton', {
    name: 'Cache capacity (MiB)',
  })
  await user.clear(capacity)
  await user.type(capacity, '32')
  await user.click(screen.getByRole('button', { name: 'Save Changes' }))

  expect(await screen.findByText('Save unavailable')).toBeVisible()
  expect(capacity).toHaveValue(32)
  expect(screen.getByRole('button', { name: 'Save Changes' })).toBeEnabled()
})

test('does not submit a lifetime shorter than the supported minimum', async () => {
  const { editor, user, saveRequest } = await prepareConfiguration()
  render(editor())
  const lifetime = await screen.findByRole('spinbutton', {
    name: 'Cache lifetime (seconds)',
  })
  await user.clear(lifetime)
  await user.type(lifetime, '59')
  await user.click(screen.getByRole('button', { name: 'Save Changes' }))

  expect(
    await screen.findByText('Enter a whole number from 60 to 3,600.')
  ).toBeVisible()
  expect(lifetime).toHaveAttribute('aria-invalid', 'true')
  expect(saveRequest).not.toHaveBeenCalled()
})

test.each([
  [
    'Cache capacity (MiB)',
    'Image compression cache capacity must be between 1 and 256 MiB',
    '32',
  ],
  [
    'Cache lifetime (seconds)',
    'Image compression cache lifetime must be between 60 and 3600 seconds',
    '120',
  ],
])(
  'associates a server validation error with %s',
  async (name, rejection, value) => {
    const { editor, user } = await prepareConfiguration(rejection)
    render(editor())
    const field = await screen.findByRole('spinbutton', { name })
    await user.clear(field)
    await user.type(field, value)
    await user.click(screen.getByRole('button', { name: 'Save Changes' }))

    await waitFor(() => expect(field).toHaveAttribute('aria-invalid', 'true'))
    expect(field).toHaveAccessibleDescription(new RegExp(rejection))
  }
)

test.each([
  ['zh-CN', '配置', '统计'],
  ['zh-TW', '設定', '統計'],
])(
  'shows the cache-specific tab labels for %s',
  async (language, configuration, statistics) => {
    vi.spyOn(api, 'get').mockResolvedValue({
      data: {
        success: true,
        data: { enabled: false, max_bytes: 67108864, ttl_seconds: 900 },
      },
    })
    const i18n = createInstance()
    await i18n.init({
      lng: convertDetectedLanguage(language),
      fallbackLng: 'en',
      resources,
      load: 'currentOnly',
      nsSeparator: false,
    })
    render(
      <I18nextProvider i18n={i18n}>
        <QueryClientProvider client={new QueryClient()}>
          <ImageCompressionCache />
        </QueryClientProvider>
      </I18nextProvider>
    )
    expect(screen.getByRole('tab', { name: configuration })).toHaveAttribute(
      'aria-selected',
      'true'
    )
    expect(screen.getByRole('tab', { name: statistics })).toHaveAttribute(
      'aria-selected',
      'false'
    )
    await screen.findByRole('spinbutton', { name: /MiB/ })
  }
)
