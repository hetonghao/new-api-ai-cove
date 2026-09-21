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
import { useQuery } from '@tanstack/react-query'
import { getRouteApi, useNavigate } from '@tanstack/react-router'
import { RefreshCw, Settings } from 'lucide-react'
import { useEffect, useState, type ReactNode } from 'react'
import { useTranslation } from 'react-i18next'

import { EmptyState } from '@/components/empty-state'
import { ErrorState } from '@/components/error-state'
import { SectionPageLayout } from '@/components/layout'
import { Button } from '@/components/ui/button'
import { Skeleton } from '@/components/ui/skeleton'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'

import { getQualityCapabilities, getQualityCases } from './api'
import { CaseManager } from './components/case-manager'
import { DashboardPanel } from './components/dashboard-panel'
import { RunsTable } from './components/runs-table'
import { SettingsDialog } from './components/settings-dialog'

const route = getRouteApi('/_authenticated/model-quality/')

export function ModelQuality() {
  const { t } = useTranslation()
  const navigate = useNavigate({ from: '/model-quality/' })
  const search = route.useSearch()
  const [settingsOpen, setSettingsOpen] = useState(false)

  const capabilitiesQuery = useQuery({
    queryKey: ['model-quality', 'capabilities'],
    queryFn: async () => {
      const response = await getQualityCapabilities()
      if (!response.success || !response.data) {
        throw new Error(response.message)
      }
      return response.data
    },
    retry: false,
  })
  const casesQuery = useQuery({
    queryKey: ['model-quality', 'cases'],
    queryFn: async () => {
      const response = await getQualityCases()
      if (!response.success || !response.data) {
        throw new Error(response.message)
      }
      return response.data
    },
    refetchInterval: (query) =>
      query.state.data?.some((qualityCase) => qualityCase.active_run_id !== '')
        ? 2000
        : 30000,
    refetchIntervalInBackground: false,
    retry: false,
  })
  const cases = casesQuery.data ?? []
  const enabledCases = cases.filter(
    (qualityCase) => qualityCase.enabled && !qualityCase.archived
  )
  const canOperate = capabilitiesQuery.data?.can_operate ?? false
  const canView = capabilitiesQuery.data?.can_view ?? canOperate

  const setSearch = (patch: Record<string, unknown>) => {
    void navigate({
      search: (prev) => ({ ...prev, ...patch }),
      replace: true,
    })
  }

  const activeCase =
    enabledCases.find((qualityCase) => qualityCase.id === search.case) ??
    enabledCases[0]

  // URL case param points at a disabled/archived/unknown case → fall back.
  // Only applies to the test panel; the cases tab manages every case.
  useEffect(() => {
    if (
      search.tab === 'panel' &&
      search.case !== undefined &&
      enabledCases.length > 0 &&
      !enabledCases.some((qualityCase) => qualityCase.id === search.case)
    ) {
      setSearch({ case: undefined, channel: undefined })
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [search.tab, search.case, enabledCases.length])

  // Public viewers only see the panel tab.
  useEffect(() => {
    if (
      capabilitiesQuery.data !== undefined &&
      !canOperate &&
      search.tab !== 'panel'
    ) {
      setSearch({ tab: 'panel' })
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [capabilitiesQuery.data, canOperate, search.tab])

  const loading = capabilitiesQuery.isPending || casesQuery.isPending
  const failed = capabilitiesQuery.isError || casesQuery.isError
  const refetchAll = () => {
    void capabilitiesQuery.refetch()
    void casesQuery.refetch()
  }

  function tabContent(content: ReactNode, skeletonHeight: string) {
    if (loading) return <Skeleton className={skeletonHeight} />
    if (failed) {
      return (
        <ErrorState
          description={t('Failed to load model quality data')}
          onRetry={refetchAll}
        />
      )
    }
    return content
  }

  function panelContent() {
    if (loading) {
      return (
        <div className='space-y-3'>
          <Skeleton className='h-8 w-64' />
          <Skeleton className='h-24 w-full' />
          <Skeleton className='h-64 w-full' />
        </div>
      )
    }
    if (failed) {
      return (
        <ErrorState
          description={t('Failed to load model quality data')}
          onRetry={refetchAll}
        />
      )
    }
    if (enabledCases.length === 0) {
      return (
        <EmptyState
          bordered
          title={t('No enabled test cases')}
          description={t('Enable a case in Test Cases to show it here.')}
          action={
            <Button
              size='sm'
              variant='outline'
              onClick={() => setSearch({ tab: 'cases' })}
            >
              {t('Test Cases')}
            </Button>
          }
        />
      )
    }
    return (
      <Tabs
        value={String(activeCase?.id ?? '')}
        onValueChange={(value) =>
          setSearch({ case: Number(value), channel: undefined })
        }
      >
        <TabsList
          variant='line'
          className='h-auto max-w-full flex-wrap justify-start gap-y-2 pb-1 group-data-horizontal/tabs:h-auto'
        >
          {enabledCases.map((qualityCase) => (
            <TabsTrigger key={qualityCase.id} value={String(qualityCase.id)}>
              {qualityCase.name}
            </TabsTrigger>
          ))}
        </TabsList>
        {activeCase && (
          <div className='mt-3'>
            <DashboardPanel
              qualityCase={activeCase}
              capabilities={capabilitiesQuery.data}
              onEdit={() => setSearch({ tab: 'cases', case: activeCase.id })}
            />
          </div>
        )}
      </Tabs>
    )
  }

  return (
    <SectionPageLayout fixedContent>
      <SectionPageLayout.Title>{t('Model Quality')}</SectionPageLayout.Title>
      <SectionPageLayout.Actions>
        <Button
          size='sm'
          variant='outline'
          onClick={refetchAll}
          aria-label={t('Refresh')}
        >
          <RefreshCw data-icon='inline-start' />
          {t('Refresh')}
        </Button>
        {capabilitiesQuery.data?.can_configure && (
          <Button
            size='sm'
            variant='outline'
            onClick={() => setSettingsOpen(true)}
          >
            <Settings data-icon='inline-start' />
            {t('Global settings')}
          </Button>
        )}
      </SectionPageLayout.Actions>
      <SectionPageLayout.Content>
        {!loading && !canView ? (
          <EmptyState
            bordered
            title={t('Model Quality')}
            description={t('Model quality is not available for your account.')}
          />
        ) : (
          <Tabs
            value={search.tab}
            onValueChange={(value) => setSearch({ tab: value })}
            className='h-full min-h-0 min-w-0'
          >
            <TabsList className='h-auto max-w-full flex-wrap justify-start gap-y-2 pb-1 group-data-horizontal/tabs:h-auto'>
              <TabsTrigger value='panel'>{t('Test Panel')}</TabsTrigger>
              {canOperate && (
                <>
                  <TabsTrigger value='cases'>{t('Test Cases')}</TabsTrigger>
                  <TabsTrigger value='batches'>
                    {t('Batch Records')}
                  </TabsTrigger>
                </>
              )}
            </TabsList>
            <TabsContent
              value='panel'
              className='-mx-1 mt-3 -mb-1 min-h-0 overflow-x-hidden overflow-y-auto p-1'
            >
              {panelContent()}
            </TabsContent>
            {canOperate && (
              <>
                <TabsContent
                  value='cases'
                  className='-mx-1 mt-3 -mb-1 min-h-0 overflow-x-hidden overflow-y-auto p-1'
                >
                  {tabContent(
                    <CaseManager
                      cases={cases}
                      capabilities={capabilitiesQuery.data}
                    />,
                    'h-64 w-full'
                  )}
                </TabsContent>
                <TabsContent
                  value='batches'
                  className='-mx-1 mt-3 -mb-1 min-h-0 overflow-x-hidden overflow-y-auto p-1'
                >
                  {tabContent(
                    <RunsTable
                      cases={cases}
                      capabilities={capabilitiesQuery.data}
                    />,
                    'h-64 w-full'
                  )}
                </TabsContent>
              </>
            )}
          </Tabs>
        )}
        <SettingsDialog open={settingsOpen} onOpenChange={setSettingsOpen} />
      </SectionPageLayout.Content>
    </SectionPageLayout>
  )
}
