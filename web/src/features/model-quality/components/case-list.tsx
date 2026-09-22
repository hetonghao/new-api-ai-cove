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
import { Drag01Icon } from '@hugeicons/core-free-icons'
import { HugeiconsIcon } from '@hugeicons/react'
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { Reorder, useDragControls } from 'motion/react'
import {
  useEffect,
  useMemo,
  useState,
  type KeyboardEvent,
  type PointerEvent,
  type ReactNode,
} from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { StatusBadge } from '@/components/status-badge'
import { Button } from '@/components/ui/button'
import { handleServerError } from '@/lib/handle-server-error'
import { cn } from '@/lib/utils'

import { reorderQualityCases } from '../api'
import type { QualityCaseView } from '../types'

type CaseStatus = 'enabled' | 'archived' | 'disabled'

function caseStatus(qualityCase: QualityCaseView): CaseStatus {
  if (qualityCase.enabled && !qualityCase.archived) return 'enabled'
  if (qualityCase.archived) return 'archived'
  return 'disabled'
}

function statusLabel(status: CaseStatus, t: (key: string) => string): string {
  if (status === 'enabled') return t('Enabled')
  if (status === 'archived') return t('Archived')
  return t('Disabled')
}

type CaseRowProps = {
  qualityCase: QualityCaseView
  selected: boolean
  onSelect: (id: number) => void
  handle?: ReactNode
}

function CaseRow(props: CaseRowProps) {
  const { t } = useTranslation()
  const status = caseStatus(props.qualityCase)
  return (
    <div className='flex items-center gap-1 px-1 py-1'>
      {props.handle}
      <button
        type='button'
        aria-current={props.selected ? 'true' : undefined}
        onClick={() => props.onSelect(props.qualityCase.id)}
        className={cn(
          'hover:bg-muted/50 flex min-w-0 flex-1 items-center gap-2 rounded-md px-2 py-1.5 text-left',
          props.selected && 'bg-muted'
        )}
      >
        <span className='min-w-0 flex-1'>
          <span className='block truncate text-sm font-medium'>
            {props.qualityCase.name}
          </span>
          <span className='text-muted-foreground block truncate text-xs'>
            {props.qualityCase.config.model}
          </span>
        </span>
        <StatusBadge
          variant={status === 'enabled' ? 'success' : 'neutral'}
          copyable={false}
          label={statusLabel(status, t)}
        />
      </button>
    </div>
  )
}

type SortableCaseItemProps = {
  qualityCase: QualityCaseView
  index: number
  selected: boolean
  onSelect: (id: number) => void
  onMove: (index: number, direction: 'up' | 'down') => void
  onDragEnd: () => void
}

function SortableCaseItem(props: SortableCaseItemProps) {
  const { t } = useTranslation()
  const dragControls = useDragControls()

  const handleDragStart = (event: PointerEvent<HTMLButtonElement>) => {
    dragControls.start(event)
  }

  const handleDragKeyDown = (event: KeyboardEvent<HTMLButtonElement>) => {
    if (event.key === 'ArrowUp') {
      event.preventDefault()
      props.onMove(props.index, 'up')
    }
    if (event.key === 'ArrowDown') {
      event.preventDefault()
      props.onMove(props.index, 'down')
    }
  }

  return (
    <Reorder.Item
      value={props.qualityCase}
      dragListener={false}
      dragControls={dragControls}
      onDragEnd={props.onDragEnd}
      className='bg-background'
    >
      <CaseRow
        qualityCase={props.qualityCase}
        selected={props.selected}
        onSelect={props.onSelect}
        handle={
          <Button
            type='button'
            variant='ghost'
            size='icon-sm'
            className='text-muted-foreground cursor-grab touch-none active:cursor-grabbing'
            aria-label={t('Drag {{group}} to reorder', {
              group: props.qualityCase.name,
            })}
            onPointerDown={handleDragStart}
            onKeyDown={handleDragKeyDown}
          >
            <HugeiconsIcon icon={Drag01Icon} strokeWidth={2} aria-hidden='true' />
          </Button>
        }
      />
    </Reorder.Item>
  )
}

type CaseListProps = {
  cases: readonly QualityCaseView[]
  filter: string
  selectedId: number | undefined
  newCaseMode: boolean
  canOperate: boolean
  onSelect: (id: number) => void
}

export function CaseList(props: CaseListProps) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const [ordered, setOrdered] = useState<QualityCaseView[]>([
    ...props.cases,
  ])

  useEffect(() => {
    setOrdered([...props.cases])
  }, [props.cases])

  const reorderMutation = useMutation({
    mutationFn: (ids: number[]) => reorderQualityCases(ids),
    retry: false,
    onSuccess: (response) => {
      if (!response.success) {
        toast.error(response.message || t('Request failed'))
      }
      void queryClient.invalidateQueries({
        queryKey: ['model-quality', 'cases'],
      })
    },
    onError: (error) => {
      handleServerError(error, t('Failed to save case order'))
      void queryClient.invalidateQueries({
        queryKey: ['model-quality', 'cases'],
      })
    },
  })

  const persist = (next: QualityCaseView[]) => {
    setOrdered(next)
    reorderMutation.mutate(next.map((item) => item.id))
  }

  const move = (index: number, direction: 'up' | 'down') => {
    const target = direction === 'up' ? index - 1 : index + 1
    if (target < 0 || target >= ordered.length) return
    const next = [...ordered]
    ;[next[index], next[target]] = [next[target], next[index]]
    persist(next)
  }

  const canReorder = props.canOperate && props.filter.trim() === ''
  const visible = useMemo(() => {
    const needle = props.filter.trim().toLowerCase()
    if (needle === '') return ordered
    return ordered.filter(
      (qualityCase) =>
        qualityCase.name.toLowerCase().includes(needle) ||
        qualityCase.config.model.toLowerCase().includes(needle)
    )
  }, [ordered, props.filter])

  const isSelected = (qualityCase: QualityCaseView) =>
    !props.newCaseMode && props.selectedId === qualityCase.id

  return (
    <div className='border divide-border divide-y rounded-lg'>
      {visible.length === 0 && (
        <div className='text-muted-foreground p-3 text-sm'>
          {t('No cases')}
        </div>
      )}
      {canReorder ? (
        <Reorder.Group
          axis='y'
          values={ordered}
          onReorder={setOrdered}
          className='divide-border divide-y'
        >
          {ordered.map((qualityCase, index) => (
            <SortableCaseItem
              key={qualityCase.id}
              qualityCase={qualityCase}
              index={index}
              selected={isSelected(qualityCase)}
              onSelect={props.onSelect}
              onMove={move}
              onDragEnd={() => persist(ordered)}
            />
          ))}
        </Reorder.Group>
      ) : (
        visible.map((qualityCase) => (
          <CaseRow
            key={qualityCase.id}
            qualityCase={qualityCase}
            selected={isSelected(qualityCase)}
            onSelect={props.onSelect}
          />
        ))
      )}
    </div>
  )
}
