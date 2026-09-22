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
import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { ConfirmDialog } from '@/components/confirm-dialog'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'

import type { QualityCapabilities, QualityCaseView } from '../types'
import { CaseEditor } from './case-editor'
import { CaseList } from './case-list'

const route = getRouteApi('/_authenticated/model-quality/')

type CaseManagerProps = {
  cases: readonly QualityCaseView[]
  capabilities: QualityCapabilities | undefined
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
        <CaseList
          cases={props.cases}
          filter={filter}
          selectedId={selectedId}
          newCaseMode={newCaseMode}
          canOperate={props.capabilities?.can_operate ?? false}
          onSelect={select}
        />
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
