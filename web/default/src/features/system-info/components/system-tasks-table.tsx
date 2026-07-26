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
import { useTranslation } from 'react-i18next'

import { Badge } from '@/components/ui/badge'
import { Progress } from '@/components/ui/progress'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import type {
  SystemTask,
  SystemTaskStatus,
} from '@/features/system-settings/types'
import { toIntlLocale } from '@/i18n/languages'
import { formatTimestampRelative, formatTimestampToDate } from '@/lib/format'
import { cn } from '@/lib/utils'

const STATUS_VARIANT: Record<SystemTaskStatus, 'secondary' | 'destructive'> = {
  pending: 'secondary',
  running: 'secondary',
  succeeded: 'secondary',
  failed: 'destructive',
}

const STATUS_CLASS_NAME: Record<SystemTaskStatus, string> = {
  pending:
    'bg-amber-50 text-amber-700 dark:bg-amber-500/15 dark:text-amber-300',
  running:
    'bg-sky-50 text-sky-700 dark:bg-sky-500/15 dark:text-sky-300 [&_span]:bg-sky-500',
  succeeded:
    'bg-emerald-50 text-emerald-700 dark:bg-emerald-500/15 dark:text-emerald-300',
  failed: '',
}

const STATUS_DOT_CLASS_NAME: Record<SystemTaskStatus, string> = {
  pending: 'bg-amber-500',
  running: 'bg-sky-500',
  succeeded: 'bg-emerald-500',
  failed: 'bg-destructive',
}

const PROGRESS_BAR_CLASS_NAME: Record<SystemTaskStatus, string> = {
  pending: '[&_[data-slot=progress-indicator]]:bg-amber-500',
  running: '[&_[data-slot=progress-indicator]]:bg-sky-500',
  succeeded: '[&_[data-slot=progress-indicator]]:bg-emerald-500',
  failed: '[&_[data-slot=progress-indicator]]:bg-destructive',
}

const TYPE_LABEL: Record<string, string> = {
  log_cleanup: 'Log cleanup',
  channel_test: 'Batch channel test',
  model_update: 'Batch upstream model update',
  midjourney_poll: 'Drawing task polling',
  async_task_poll: 'Async task polling',
  risk_record_cleanup: 'Risk record cleanup',
}

const TYPE_DISPLAY_ID: Record<string, string> = {
  midjourney_poll: 'drawing_task_poll',
}

function getProgress(task: SystemTask): number | null {
  const progress = (task.state as { progress?: unknown } | undefined)?.progress
  if (typeof progress !== 'number' || Number.isNaN(progress)) return null
  return Math.min(100, Math.max(0, progress))
}

function TaskStatusBadge(props: { readonly status: SystemTaskStatus }) {
  const { t } = useTranslation()

  return (
    <Badge
      variant={STATUS_VARIANT[props.status]}
      className={cn('shrink-0 gap-1.5', STATUS_CLASS_NAME[props.status])}
    >
      <span
        className={cn(
          'size-1.5 rounded-full',
          STATUS_DOT_CLASS_NAME[props.status]
        )}
        aria-hidden='true'
      />
      {t(props.status)}
    </Badge>
  )
}

type SystemTasksTableProps = {
  readonly tasks: readonly SystemTask[]
}

export function SystemTasksTable(props: SystemTasksTableProps) {
  const { t, i18n } = useTranslation()

  return (
    <>
      <div className='overflow-hidden rounded-md border lg:hidden'>
        <div className='divide-y'>
          {props.tasks.map((task) => {
            const progress = getProgress(task)
            return (
              <article key={task.task_id} className='p-4'>
                <div className='flex min-w-0 items-start justify-between gap-3'>
                  <div className='min-w-0'>
                    <div className='font-medium break-words'>
                      {t(TYPE_LABEL[task.type] ?? task.type)}
                    </div>
                    <div className='text-muted-foreground mt-0.5 font-mono text-[11px] break-all'>
                      {TYPE_DISPLAY_ID[task.type] ?? task.type}
                    </div>
                  </div>
                  <TaskStatusBadge status={task.status} />
                </div>

                <div className='mt-3'>
                  <div className='flex items-center justify-between gap-3 text-xs'>
                    <span className='text-muted-foreground'>
                      {t('Progress')}
                    </span>
                    <span className='tabular-nums'>
                      {progress === null ? '-' : `${progress}%`}
                    </span>
                  </div>
                  <Progress
                    value={progress ?? 0}
                    className={cn(
                      'mt-1.5 w-full',
                      PROGRESS_BAR_CLASS_NAME[task.status]
                    )}
                  />
                </div>

                <dl className='mt-3 grid grid-cols-1 gap-3 text-xs sm:grid-cols-3'>
                  <div className='min-w-0'>
                    <dt className='text-muted-foreground'>{t('Executor')}</dt>
                    <dd
                      className='mt-0.5 truncate font-mono'
                      title={task.locked_by || undefined}
                    >
                      {task.locked_by || '-'}
                    </dd>
                  </div>
                  <div>
                    <dt className='text-muted-foreground'>{t('Updated')}</dt>
                    <dd
                      className='mt-0.5'
                      title={formatTimestampToDate(task.updated_at)}
                    >
                      {formatTimestampRelative(
                        task.updated_at,
                        'seconds',
                        toIntlLocale(i18n.language)
                      )}
                    </dd>
                  </div>
                  <div className='min-w-0'>
                    <dt className='text-muted-foreground'>{t('Detail')}</dt>
                    <dd
                      className='text-destructive mt-0.5 truncate'
                      title={task.error || undefined}
                    >
                      {task.error || '-'}
                    </dd>
                  </div>
                </dl>
              </article>
            )
          })}
        </div>
      </div>

      <div className='hidden overflow-x-auto rounded-md border lg:block'>
        <Table className='min-w-[900px]'>
          <TableHeader>
            <TableRow className='bg-muted/40 hover:bg-muted/40'>
              <TableHead className='h-9 w-[260px] px-4 text-xs'>
                {t('Type')}
              </TableHead>
              <TableHead className='h-9 w-[130px] text-xs'>
                {t('Status')}
              </TableHead>
              <TableHead className='h-9 w-[180px] text-xs'>
                {t('Progress')}
              </TableHead>
              <TableHead className='h-9 min-w-[260px] text-xs'>
                {t('Executor')}
              </TableHead>
              <TableHead className='h-9 w-[190px] text-xs'>
                {t('Updated')}
              </TableHead>
              <TableHead className='h-9 w-[220px] pr-4 text-xs'>
                {t('Detail')}
              </TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {props.tasks.map((task) => {
              const progress = getProgress(task)
              return (
                <TableRow key={task.task_id} className='hover:bg-muted/30'>
                  <TableCell className='px-4 py-3 align-middle'>
                    <div className='space-y-0.5'>
                      <div className='font-medium'>
                        {t(TYPE_LABEL[task.type] ?? task.type)}
                      </div>
                      <div className='text-muted-foreground font-mono text-[11px]'>
                        {TYPE_DISPLAY_ID[task.type] ?? task.type}
                      </div>
                    </div>
                  </TableCell>
                  <TableCell className='py-3 align-middle'>
                    <TaskStatusBadge status={task.status} />
                  </TableCell>
                  <TableCell className='py-3 align-middle'>
                    <div className='flex items-center gap-2'>
                      <Progress
                        value={progress ?? 0}
                        className={cn(
                          'w-24',
                          PROGRESS_BAR_CLASS_NAME[task.status]
                        )}
                      />
                      <span className='text-muted-foreground w-10 text-right text-xs tabular-nums'>
                        {progress === null ? '-' : `${progress}%`}
                      </span>
                    </div>
                  </TableCell>
                  <TableCell className='text-muted-foreground max-w-[280px] truncate py-3 align-middle font-mono text-xs'>
                    {task.locked_by || '-'}
                  </TableCell>
                  <TableCell
                    className='text-muted-foreground py-3 align-middle text-xs whitespace-nowrap'
                    title={formatTimestampToDate(task.updated_at)}
                  >
                    {formatTimestampRelative(
                      task.updated_at,
                      'seconds',
                      toIntlLocale(i18n.language)
                    )}
                  </TableCell>
                  <TableCell
                    className='text-destructive max-w-[220px] truncate py-3 pr-4 align-middle text-xs'
                    title={task.error || undefined}
                  >
                    {task.error || '-'}
                  </TableCell>
                </TableRow>
              )
            })}
          </TableBody>
        </Table>
      </div>
    </>
  )
}
