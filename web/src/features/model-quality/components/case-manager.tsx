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
import { getRouteApi, useNavigate } from '@tanstack/react-router'
import { Plus } from 'lucide-react'
import { useEffect, useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { ConfirmDialog } from '@/components/confirm-dialog'
import { StatusBadge } from '@/components/status-badge'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { cn } from '@/lib/utils'

import type { QualityCapabilities, QualityCaseView } from '../types'
import { CaseEditor } from './case-editor'

const route = getRouteApi('/_authenticated/model-quality/')

type CaseManagerProps = {
  cases: readonly QualityCaseView[]
  capabilities: QualityCapabilities | undefined
}

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

export function CaseManager(props: CaseManagerProps) {
  const { t } = useTranslation()
  const navigate = useNavigate({ from: '/model-quality/' })
  const search = route.useSearch()
  const [filter, setFilter] = useState('')
  const [dirty, setDirty] = useState(false)
  const [pendingSelection, setPendingSelection] = useState<number | 'new' | null>(
    null
  )

  const filtered = useMemo(() => {
    const needle = filter.trim().toLowerCase()
    if (needle === '') return props.cases
    return props.cases.filter(
      (qualityCase) =>
        qualityCase.name.toLowerCase().includes(needle) ||
        qualityCase.config.model.toLowerCase().includes(needle)
    )
  }, [props.cases, filter])

  const selectedId = search.case
  const selectedCase =
    props.cases.find((qualityCase) => qualityCase.id === selectedId) ?? null

  // New-case editor is selected when URL has no valid case after explicit click.
  const [newCaseMode, setNewCaseMode] = useState(false)
  useEffect(() => {
    if (selectedCase) setNewCaseMode(false)
  }, [selectedCase])

  const select = (target: number | 'new') => {
    if (dirty) {
      setPendingSelection(target)
      return
    }
    applySelection(target)
  }
  const applySelection = (target: number | 'new') => {
    if (target === 'new') {
      setNewCaseMode(true)
      void navigate({
        search: (prev) => ({ ...prev, case: undefined }),
        replace: true,
      })
    } else {
      setNewCaseMode(false)
      void navigate({
        search: (prev) => ({ ...prev, case: target }),
        replace: true,
      })
    }
    setPendingSelection(null)
  }

  const showEditor = newCaseMode || selectedCase !== null

  return (
    <div className='grid grid-cols-1 gap-4 lg:grid-cols-[280px_minmax(0,1fr)]'>
      <div className='space-y-2'>
        <Button
          variant='outline'
          className='w-full'
          disabled={!(props.capabilities?.can_operate ?? false)}
          onClick={() => select('new')}
        >
          <Plus data-icon='inline-start' />
          {t('New case')}
        </Button>
        <Input
          value={filter}
          onChange={(event) => setFilter(event.target.value)}
          placeholder={t('Search cases')}
          aria-label={t('Search cases')}
        />
        <div className='border divide-border divide-y rounded-lg'>
          {filtered.length === 0 && (
            <div className='text-muted-foreground p-3 text-sm'>
              {t('No cases')}
            </div>
          )}
          {filtered.map((qualityCase) => {
            const status = caseStatus(qualityCase)
            const selected =
              !newCaseMode && selectedCase?.id === qualityCase.id
            return (
              <button
                key={qualityCase.id}
                type='button'
                aria-current={selected ? 'true' : undefined}
                onClick={() => select(qualityCase.id)}
                className={cn(
                  'hover:bg-muted/50 flex w-full items-center gap-2 px-3 py-2 text-left',
                  selected && 'bg-muted'
                )}
              >
                <span className='min-w-0 flex-1'>
                  <span className='block truncate text-sm font-medium'>
                    {qualityCase.name}
                  </span>
                  <span className='text-muted-foreground block truncate text-xs'>
                    {qualityCase.config.model}
                  </span>
                </span>
                <StatusBadge
                  variant={status === 'enabled' ? 'success' : 'neutral'}
                  copyable={false}
                  label={statusLabel(status, t)}
                />
              </button>
            )
          })}
        </div>
      </div>
      <div className='min-w-0'>
        {showEditor ? (
          <CaseEditor
            key={newCaseMode ? 'new' : (selectedCase?.id ?? 'none')}
            qualityCase={newCaseMode ? null : selectedCase}
            capabilities={props.capabilities}
            onDirtyChange={setDirty}
          />
        ) : (
          <div className='border text-muted-foreground flex min-h-40 items-center justify-center rounded-lg text-sm'>
            {t('Select a case or create a new one')}
          </div>
        )}
      </div>
      <ConfirmDialog
        open={pendingSelection !== null}
        onOpenChange={(open) => !open && setPendingSelection(null)}
        title={t('Discard unsaved changes?')}
        desc={t('Unsaved edits to this case will be lost.')}
        destructive
        handleConfirm={() => {
          if (pendingSelection !== null) {
            setDirty(false)
            applySelection(pendingSelection)
          }
        }}
      />
    </div>
  )
}
