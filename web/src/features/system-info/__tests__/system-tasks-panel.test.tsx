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
  act,
  cleanup,
  render,
  screen,
  waitFor,
  within,
} from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import i18next from 'i18next'
import { Toaster } from 'sonner'
import { afterEach, expect, it, vi } from 'vitest'

import fr from '@/i18n/locales/fr.json'
import ja from '@/i18n/locales/ja.json'
import ru from '@/i18n/locales/ru.json'
import viLocale from '@/i18n/locales/vi.json'
import zhTW from '@/i18n/locales/zh-TW.json'
import zhCN from '@/i18n/locales/zh.json'
import { api, type ApiRequestConfig } from '@/lib/api'

import { SystemTasksPanel } from '../components/system-tasks-panel'

const task = {
  id: 1,
  task_id: 'history-task',
  type: 'model_update',
  status: 'succeeded',
  created_at: 100,
  updated_at: 200,
  locked_by: 'history-runner',
}

function renderPanel() {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  })
  render(
    <QueryClientProvider client={client}>
      <SystemTasksPanel />
      <Toaster />
    </QueryClientProvider>
  )
  return client
}

afterEach(() => {
  cleanup()
  vi.restoreAllMocks()
})

it('filters history on the server and resets pagination without hiding active tasks', async () => {
  const get = vi
    .spyOn(api, 'get')
    .mockImplementation(async (_url, config?: ApiRequestConfig) => {
      if (config?.params?.scope === 'active') {
        return {
          data: {
            success: true,
            data: [
              {
                ...task,
                task_id: 'active',
                status: 'running',
                locked_by: 'active-runner',
              },
            ],
            total: 1,
          },
        }
      }
      return { data: { success: true, data: [task], total: 21 } }
    })
  const client = renderPanel()
  await screen.findByRole('combobox', { name: 'Type' })
  await userEvent.click(
    await screen.findByRole('button', { name: 'Go to next page' })
  )
  await waitFor(() =>
    expect(get).toHaveBeenCalledWith(
      '/api/system-task/list',
      expect.objectContaining({
        params: expect.objectContaining({ scope: 'history', offset: 20 }),
      })
    )
  )
  await userEvent.selectOptions(
    screen.getByRole('combobox', { name: 'Type' }),
    'model_update'
  )
  await userEvent.selectOptions(
    screen.getByRole('combobox', { name: 'Status' }),
    'failed'
  )
  await waitFor(() =>
    expect(get).toHaveBeenLastCalledWith(
      '/api/system-task/list',
      expect.objectContaining({
        params: expect.objectContaining({
          scope: 'history',
          type: 'model_update',
          status: 'failed',
          offset: 0,
        }),
      })
    )
  )
  expect(screen.getByText('active-runner')).toBeVisible()
  client.clear()
})

it('keeps filters available and disables cleanup when history is empty', async () => {
  vi.spyOn(api, 'get').mockResolvedValue({
    data: { success: true, data: [], total: 0 },
  })
  const client = renderPanel()
  expect(await screen.findByText('No historical system tasks.')).toBeVisible()
  expect(screen.getByRole('combobox', { name: 'Type' })).toBeEnabled()
  expect(
    screen.getByRole('button', { name: 'Clean task history' })
  ).toBeDisabled()
  client.clear()
})

it('requires confirmation and cleans all matching history pages using the selected filters', async () => {
  vi.spyOn(api, 'get').mockImplementation(
    async (_url, config?: ApiRequestConfig) => ({
      data: {
        success: true,
        data: config?.params?.scope === 'active' ? [] : [task],
        total: 21,
      },
    })
  )
  const remove = vi
    .spyOn(api, 'delete')
    .mockResolvedValue({ data: { success: true, data: { deleted_count: 20 } } })
  const client = renderPanel()
  await userEvent.selectOptions(
    await screen.findByRole('combobox', { name: 'Type' }),
    'model_update'
  )
  await waitFor(() =>
    expect(
      screen.getByRole('button', { name: 'Clean task history' })
    ).toBeEnabled()
  )
  await userEvent.click(
    screen.getByRole('button', { name: 'Clean task history' })
  )
  const dialog = screen.getByRole('alertdialog')
  expect(within(dialog).getByText(/latest run of each task type/)).toBeVisible()
  expect(remove).not.toHaveBeenCalled()
  await userEvent.click(within(dialog).getByRole('button', { name: 'Cancel' }))
  expect(remove).not.toHaveBeenCalled()
  await userEvent.click(
    screen.getByRole('button', { name: 'Clean task history' })
  )
  await userEvent.click(
    within(screen.getByRole('alertdialog')).getByRole('button', {
      name: 'Delete',
    })
  )
  await waitFor(() =>
    expect(remove).toHaveBeenCalledWith('/api/system-task/history', {
      params: { type: 'model_update', status: '' },
    })
  )
  await waitFor(() =>
    expect(screen.queryByRole('alertdialog')).not.toBeInTheDocument()
  )
  client.clear()
})

it('retains the confirmation and shows the server error when cleanup fails', async () => {
  vi.spyOn(api, 'get').mockResolvedValue({
    data: { success: true, data: [task], total: 1 },
  })
  let finishCleanup!: (response: {
    data: { success: boolean; message: string }
  }) => void
  const response = new Promise<{ data: { success: boolean; message: string } }>(
    (resolve) => {
      finishCleanup = resolve
    }
  )
  vi.spyOn(api, 'delete').mockReturnValue(response)
  const client = renderPanel()
  await waitFor(() =>
    expect(
      screen.getByRole('button', { name: 'Clean task history' })
    ).toBeEnabled()
  )
  await userEvent.click(
    screen.getByRole('button', { name: 'Clean task history' })
  )
  const dialog = screen.getByRole('alertdialog')
  await userEvent.click(within(dialog).getByRole('button', { name: 'Delete' }))
  expect(
    within(dialog).getByRole('button', { name: 'Deleting...' })
  ).toBeDisabled()
  expect(within(dialog).getByRole('button', { name: 'Cancel' })).toBeDisabled()
  finishCleanup({
    data: { success: false, message: 'History cleanup unavailable' },
  })
  expect(await screen.findByText('History cleanup unavailable')).toBeVisible()
  expect(within(dialog).getByRole('button', { name: 'Delete' })).toBeEnabled()
  client.clear()
})

it('shows history query failures without hiding active tasks and allows retry', async () => {
  let historyFailed = true
  vi.spyOn(api, 'get').mockImplementation(
    async (_url, config?: ApiRequestConfig) => {
      if (config?.params?.scope === 'history' && historyFailed) {
        return { data: { success: false, message: 'History unavailable' } }
      }
      return {
        data: {
          success: true,
          data: [
            {
              ...task,
              locked_by:
                config?.params?.scope === 'active'
                  ? 'active-runner'
                  : 'history-runner',
            },
          ],
          total: 1,
        },
      }
    }
  )
  const client = renderPanel()
  expect(await screen.findByText('History unavailable')).toBeVisible()
  expect(screen.getByText('active-runner')).toBeVisible()
  expect(
    screen.getByRole('button', { name: 'Clean task history' })
  ).toBeDisabled()
  historyFailed = false
  await userEvent.click(
    screen.getByRole('button', { name: /retry|try again/i })
  )
  expect(await screen.findByText('history-runner')).toBeVisible()
  client.clear()
})

it('refreshes history when the last running task finishes', async () => {
  let finished = false
  vi.spyOn(api, 'get').mockImplementation(
    async (_url, config?: ApiRequestConfig) => {
      const active = config?.params?.scope === 'active'
      const data =
        active === finished
          ? []
          : [{ ...task, status: finished ? 'succeeded' : 'running' }]
      return { data: { success: true, data, total: data.length } }
    }
  )
  const client = renderPanel()
  await screen.findByText('No historical system tasks.')
  finished = true
  await userEvent.click(screen.getByRole('button', { name: 'Refresh' }))
  expect(await screen.findByText('No active system tasks.')).toBeVisible()
  await waitFor(() =>
    expect(
      screen.queryByText('No historical system tasks.')
    ).not.toBeInTheDocument()
  )
  expect(screen.getByText('history-runner')).toBeVisible()
  client.clear()
})

it('translates model quality schedules in the history list and filter when the language changes', async () => {
  vi.spyOn(api, 'get').mockImplementation(
    async (_url, config?: ApiRequestConfig) => {
      const data =
        config?.params?.scope === 'history'
          ? [
              {
                ...task,
                type: 'model_quality_schedule',
                locked_by: 'quality-runner',
              },
            ]
          : []
      return { data: { success: true, data, total: data.length } }
    }
  )
  const translations = [
    ['zhCN', zhCN.translation, '模型质量巡检调度'],
    ['zhTW', zhTW.translation, '模型質量巡檢排程'],
    [
      'fr',
      fr.translation,
      'Planification de l’inspection de la qualité des modèles',
    ],
    ['ru', ru.translation, 'Планирование проверки качества моделей'],
    ['ja', ja.translation, 'モデル品質検査のスケジュール'],
    ['vi', viLocale.translation, 'Lập lịch kiểm tra chất lượng mô hình'],
  ] as const
  let client: QueryClient | undefined
  try {
    for (const [language, resource] of translations) {
      i18next.addResourceBundle(language, 'translation', resource)
    }
    await i18next.changeLanguage('zhCN')
    client = renderPanel()
    const row = await screen.findByRole('row', { name: /quality-runner/ })
    for (const [language, , label] of translations) {
      await act(() => i18next.changeLanguage(language))
      expect(within(row).getByText(label)).toBeVisible()
      expect(screen.getByRole('option', { name: label })).toBeInTheDocument()
    }
    await act(() => i18next.changeLanguage('en'))
    expect(
      within(row).getByText('Model quality inspection scheduling')
    ).toBeVisible()
    expect(
      screen.getByRole('option', {
        name: 'Model quality inspection scheduling',
      })
    ).toBeInTheDocument()
  } finally {
    client?.clear()
    cleanup()
    await i18next.changeLanguage('en')
    for (const [language] of translations) {
      i18next.removeResourceBundle(language, 'translation')
    }
  }
})

const qualityTask = 'model_quality_schedule'
const quotaTask = 'upstream_subscription_quota'

it.each([
  [qualityTask, 'succeeded', undefined, 100],
  [quotaTask, 'succeeded', { checked: 1 }, 100],
  [qualityTask, 'succeeded', { progress: null }, 100],
  [qualityTask, 'pending', undefined, null],
  [qualityTask, 'running', undefined, null],
  [qualityTask, 'failed', undefined, null],
  [quotaTask, 'pending', undefined, null],
  [quotaTask, 'running', undefined, null],
  [quotaTask, 'failed', undefined, null],
  ['model_update', 'succeeded', undefined, null],
  ['future_task', 'succeeded', undefined, null],
  [qualityTask, 'succeeded', { progress: 'unknown' }, null],
  [qualityTask, 'succeeded', { progress: 0 }, 100],
  [quotaTask, 'succeeded', { progress: 42 }, 100],
  [qualityTask, 'running', { progress: 37 }, 37],
  ['model_update', 'succeeded', { progress: 75 }, 75],
])(
  'shows progress for %s when %s with state %j: expected %s',
  async (type, status, state, expected) => {
    const scope =
      status === 'pending' || status === 'running' ? 'active' : 'history'
    vi.spyOn(api, 'get').mockImplementation(
      async (_url, config?: ApiRequestConfig) => {
        const data =
          config?.params?.scope === scope
            ? [{ ...task, type, status, state }]
            : []
        return { data: { success: true, data, total: data.length } }
      }
    )
    const client = renderPanel()
    const row = await screen.findByRole('row', { name: /history-runner/ })
    const progressCell = within(row).getAllByRole('cell')[2]
    expect(
      within(progressCell).getByText(expected === null ? '-' : `${expected}%`)
    ).toBeVisible()
    const progressBar = within(progressCell).getByRole('progressbar', {
      name: 'Progress',
    })
    if (expected === null) {
      expect(progressBar).not.toHaveAttribute('aria-valuenow')
      expect(progressBar).not.toHaveAttribute('aria-valuetext', '0%')
    } else {
      expect(progressBar).toHaveAttribute('aria-valuenow', String(expected))
    }
    client.clear()
  }
)
