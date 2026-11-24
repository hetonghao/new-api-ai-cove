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
import {
  cleanup,
  render,
  screen,
  waitFor,
  within,
} from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { createInstance } from 'i18next'
import { I18nextProvider } from 'react-i18next'
import { afterEach, expect, test, vi } from 'vitest'
import { z } from 'zod'

import { api } from '@/lib/api'

import { ImageCompressionCache } from '..'

const node1 = 'gateway-1'
const node2 = 'gateway-2'
const sample = {
  status: 'ready',
  cache_id: 'shared-cache-2',
  nodes: [
    { node_name: node1, hits: 3, misses: 3 },
    { node_name: node2, hits: 4, misses: 0 },
  ],
  enabled: true,
  max_bytes: 67108864,
  ttl_seconds: 900,
  used_bytes: 4096,
  entry_count: 3,
  hits: 7,
  misses: 3,
  evictions: 1,
  expirations: 2,
  removals: 0,
  entries: ['entry-a', 'entry-b', 'entry-c'].map((id) => ({
    id,
    user_id: 27,
    original_mime: 'image/png',
    compressed_mime: 'image/webp',
    original_bytes: 4096,
    compressed_bytes: 1365,
    hits: 2,
    created_at: 1731283200,
    last_hit_at: 1731283205,
    expires_at: 1731284100,
  })),
}
const clearSchema = z.object({
  cache_id: z.string(),
  ids: z.array(z.string()),
})

afterEach(() => {
  cleanup()
  vi.restoreAllMocks()
})

async function openSharedStatistics(options?: {
  restartOnClear?: boolean
  unavailable?: boolean
}) {
  let entries = sample.entries
  let instance = sample.cache_id
  let clearCount = 0
  const available = !options?.unavailable
  let requestFails = false
  vi.spyOn(api, 'get').mockImplementation(async (url) => {
    if (url === '/api/image-compression-cache/config') {
      return {
        data: {
          success: true,
          data: { enabled: true, max_bytes: 67108864, ttl_seconds: 900 },
        },
      }
    }
    if (url !== '/api/image-compression-cache/stats') throw new Error(url)
    if (requestFails) {
      throw Object.assign(new Error('Request failed'), {
        response: { status: 503 },
      })
    }
    return {
      data: {
        success: true,
        data: {
          ...sample,
          status: available ? 'ready' : 'unavailable',
          cache_id: available ? instance : undefined,
          entry_count: available ? entries.length : 0,
          entries: available ? entries : [],
        },
      },
    }
  })
  const clear = vi.spyOn(api, 'post').mockImplementation(async (url, body) => {
    if (url !== '/api/image-compression-cache/clear') throw new Error(url)
    const target = clearSchema.parse(body)
    if (options?.restartOnClear && clearCount++ === 0) {
      instance = 'replacement-cache'
      entries = [{ ...sample.entries[0], id: 'replacement-entry' }]
    }
    if (target.cache_id !== instance) {
      throw Object.assign(new Error('Cache instance changed'), {
        response: { status: 409 },
      })
    }
    const previous = entries.length
    entries = entries.filter((entry) => !target.ids.includes(entry.id))
    return {
      data: { success: true, data: { deleted: previous - entries.length } },
    }
  })
  const i18n = createInstance()
  await i18n.init({ lng: 'en', fallbackLng: 'en', resources: { en: {} } })
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  render(
    <I18nextProvider i18n={i18n}>
      <QueryClientProvider client={client}>
        <ImageCompressionCache />
      </QueryClientProvider>
    </I18nextProvider>
  )
  const user = userEvent.setup()
  await user.click(screen.getByRole('tab', { name: 'Statistics' }))
  if (!options?.unavailable) {
    await screen.findByRole('button', { name: 'Clear cache entry entry-a' })
  }
  return {
    user,
    clear,
    setFailed: (fail: boolean) => {
      requestFails = fail
    },
  }
}

test('shows one shared list and reports cache hits per node', async () => {
  await openSharedStatistics()
  expect(
    screen.getByRole('progressbar', { name: 'Shared cache usage' })
  ).toHaveAttribute('aria-valuetext', '4.1 KB / 64 MiB')
  expect(screen.getByText('entry-b')).toBeVisible()
  const nodeRow = screen.getByRole('row', { name: /gateway-2/ })
  expect(within(nodeRow).getByText('100%')).toBeVisible()
})

test('clears one shared entry after confirmation', async () => {
  const { user } = await openSharedStatistics()
  await user.click(
    screen.getByRole('button', { name: 'Clear cache entry entry-a' })
  )
  const dialog = screen.getByRole('alertdialog')
  expect(dialog).toHaveTextContent('shared cache entries')
  await user.click(within(dialog).getByRole('button', { name: 'Clear' }))

  await waitFor(() =>
    expect(screen.queryByText('entry-a')).not.toBeInTheDocument()
  )
  expect(screen.getByText('entry-b')).toBeVisible()
})

test('clears only the selected entries when a bulk clear is confirmed', async () => {
  const { user } = await openSharedStatistics()
  for (const id of ['entry-a', 'entry-b']) {
    const row = screen.getByText(id).closest('tr')
    if (!row) throw new Error('Missing cache entry row')
    await user.click(within(row).getByRole('checkbox'))
  }
  const toolbar = screen.getByRole('toolbar', {
    name: 'Bulk actions for 2 selected records',
  })
  await user.click(
    within(toolbar).getByRole('button', { name: 'Clear selected' })
  )
  await user.click(
    within(screen.getByRole('alertdialog')).getByRole('button', {
      name: 'Clear',
    })
  )

  await waitFor(() =>
    expect(screen.queryByText('entry-a')).not.toBeInTheDocument()
  )
  expect(screen.queryByText('entry-b')).not.toBeInTheDocument()
  expect(screen.getByText('entry-c')).toBeVisible()
})

test('refreshes after shared cache replacement without replaying the old clear', async () => {
  const { user, clear } = await openSharedStatistics({ restartOnClear: true })
  await user.click(
    screen.getByRole('button', { name: 'Clear cache entry entry-a' })
  )
  await user.click(
    within(screen.getByRole('alertdialog')).getByRole('button', {
      name: 'Clear',
    })
  )

  expect(await screen.findByText('replacement-entry')).toBeVisible()
  expect(screen.queryByRole('alertdialog')).not.toBeInTheDocument()
  expect(clear).toHaveBeenCalledTimes(1)
})

test('keeps the most recent snapshot but disables clearing when refresh fails', async () => {
  const { user, setFailed } = await openSharedStatistics()
  setFailed(true)
  await user.click(screen.getByRole('button', { name: 'Refresh' }))

  expect(
    await screen.findByText(
      'Statistics are out of date. Refresh before clearing entries.'
    )
  ).toBeVisible()
  expect(screen.getByText('entry-a')).toBeVisible()
  expect(
    screen.getByRole('button', { name: 'Clear cache entry entry-a' })
  ).toBeDisabled()

  setFailed(false)
  await user.click(screen.getByRole('button', { name: 'Refresh' }))
  await waitFor(() =>
    expect(
      screen.getByRole('button', { name: 'Clear cache entry entry-a' })
    ).toBeEnabled()
  )
})

test('announces the selection and clears it with Escape from the inline toolbar', async () => {
  const { user } = await openSharedStatistics()
  const row = screen.getByText('entry-a').closest('tr')
  if (!row) throw new Error('Missing cache entry row')
  const checkbox = within(row).getByRole('checkbox')
  await user.click(checkbox)
  const toolbar = screen.getByRole('toolbar', {
    name: 'Bulk actions for 1 selected records',
  })
  expect(await screen.findByRole('status')).toHaveTextContent(
    '1 records selected. Bulk actions are available.'
  )
  within(toolbar).getByRole('button', { name: 'Clear selection' }).focus()
  await user.keyboard('{End}')
  expect(
    within(toolbar).getByRole('button', { name: 'Clear selected' })
  ).toHaveFocus()
  await user.keyboard('{Escape}')

  expect(checkbox).not.toBeChecked()
  expect(toolbar).not.toBeInTheDocument()
})

test('warns when Redis is unavailable instead of presenting an empty cache', async () => {
  await openSharedStatistics({ unavailable: true })
  expect(await screen.findByRole('status')).toHaveTextContent(
    'Shared image cache Redis is unavailable. Requests are compressed without caching.'
  )
  expect(screen.queryByText('No cached images')).not.toBeInTheDocument()
  expect(screen.queryByRole('progressbar')).not.toBeInTheDocument()
  expect(
    screen.queryByRole('button', { name: 'Clear selected' })
  ).not.toBeInTheDocument()
})
