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
  node_name: node2,
  instance_id: 'process-2',
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
  node_name: z.string(),
  instance_id: z.string(),
  ids: z.array(z.string()),
})

afterEach(() => {
  cleanup()
  vi.restoreAllMocks()
})

async function openSecondNode(options?: { restartOnClear?: boolean }) {
  let entries = sample.entries
  let instance = sample.instance_id
  let clearCount = 0
  let nodeAvailable = true
  vi.spyOn(api, 'get').mockImplementation(async (url, request) => {
    if (url === '/api/image-compression-cache/config') {
      return {
        data: {
          success: true,
          data: { enabled: true, max_bytes: 67108864, ttl_seconds: 900 },
        },
      }
    }
    if (url === '/api/image-compression-cache/nodes') {
      return {
        data: {
          success: true,
          data: {
            local_node_name: node1,
            nodes: [node1, node2].map((node_name) => ({
              node_name,
              status: 'online',
            })),
          },
        },
      }
    }
    if (url === '/api/image-compression-cache/stats') {
      const nodeQuery = z
        .object({ node_name: z.string() })
        .parse(request?.params)
      if (nodeQuery.node_name === node1) {
        return {
          data: {
            success: true,
            data: {
              ...sample,
              node_name: node1,
              instance_id: 'process-1',
              entry_count: 0,
              entries: [],
            },
          },
        }
      }
      if (nodeQuery.node_name === node2) {
        if (!nodeAvailable) {
          throw Object.assign(new Error('Node unavailable'), {
            response: { status: 503 },
          })
        }
        return {
          data: {
            success: true,
            data: {
              ...sample,
              instance_id: instance,
              entry_count: entries.length,
              entries,
            },
          },
        }
      }
    }
    throw new Error(url)
  })
  const clear = vi.spyOn(api, 'post').mockImplementation(async (url, body) => {
    if (url !== '/api/image-compression-cache/clear') throw new Error(url)
    const target = clearSchema.parse(body)
    if (options?.restartOnClear && clearCount++ === 0) {
      instance = 'replacement-process'
      entries = [{ ...sample.entries[0], id: 'replacement-entry' }]
    }
    if (target.node_name !== node2 || target.instance_id !== instance) {
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
  await screen.findByText('No cached images')
  await user.selectOptions(
    screen.getByRole('combobox', { name: 'Node' }),
    node2
  )
  await screen.findByRole('button', { name: 'Clear cache entry entry-a' })
  return {
    user,
    clear,
    setNodeAvailable: (available: boolean) => {
      nodeAvailable = available
    },
  }
}

test('loads the chosen node and shows memory usage with its live entries', async () => {
  await openSecondNode()
  expect(
    screen.getByRole('progressbar', { name: 'Memory usage' })
  ).toHaveAttribute('aria-valuetext', '4.1 KB / 64 MiB')
  expect(screen.getByText('entry-b')).toBeVisible()
})

test('clears one entry on the chosen instance after confirmation', async () => {
  const { user } = await openSecondNode()
  await user.click(
    screen.getByRole('button', { name: 'Clear cache entry entry-a' })
  )
  const dialog = screen.getByRole('alertdialog')
  expect(dialog).toHaveTextContent(node2)
  await user.click(within(dialog).getByRole('button', { name: 'Clear' }))

  await waitFor(() =>
    expect(screen.queryByText('entry-a')).not.toBeInTheDocument()
  )
  expect(screen.getByText('entry-b')).toBeVisible()
})

test('clears only the selected entries when a bulk clear is confirmed', async () => {
  const { user } = await openSecondNode()
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

test('refreshes after instance replacement without replaying the old clear', async () => {
  const { user, clear } = await openSecondNode({ restartOnClear: true })
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
  const { user, setNodeAvailable } = await openSecondNode()
  setNodeAvailable(false)
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

  setNodeAvailable(true)
  await user.click(screen.getByRole('button', { name: 'Refresh' }))
  await waitFor(() =>
    expect(
      screen.getByRole('button', { name: 'Clear cache entry entry-a' })
    ).toBeEnabled()
  )
})

test('announces the selection and clears it with Escape from the inline toolbar', async () => {
  const { user } = await openSecondNode()
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
