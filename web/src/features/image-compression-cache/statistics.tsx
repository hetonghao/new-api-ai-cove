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
import { TriangleAlert } from 'lucide-react'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import { ErrorState } from '@/components/error-state'
import { LoadingState } from '@/components/loading-state'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { getServerErrorMessage } from '@/lib/server-error-message'

import { useSharedImageCacheStats, type CacheStats } from './api'
import { CacheEntries } from './entries'
import { CacheNodeStats, CacheStatsSummary } from './stats-summary'

export function CacheConnectionWarning(props: {
  readonly status: CacheStats['status']
}) {
  const { t } = useTranslation()
  const messages = {
    not_configured: t(
      'Shared image cache Redis is not configured. Requests are compressed without caching.'
    ),
    unavailable: t(
      'Shared image cache Redis is unavailable. Requests are compressed without caching.'
    ),
    synchronizing: t(
      'Shared image cache settings are synchronizing. This node is temporarily bypassing the cache.'
    ),
    ready: '',
  }
  if (props.status === 'ready') return null
  return (
    <Alert role='status'>
      <TriangleAlert aria-hidden='true' />
      <AlertDescription>{messages[props.status]}</AlertDescription>
    </Alert>
  )
}

export function CacheStatistics() {
  const { t } = useTranslation()
  const [pagination, setPagination] = useState({ pageIndex: 0, pageSize: 20 })
  const stats = useSharedImageCacheStats(
    pagination.pageIndex + 1,
    pagination.pageSize
  )
  const refresh = async () => {
    await stats.refetch()
  }
  if (stats.isPending) return <LoadingState />
  if (!stats.data) {
    return (
      <ErrorState
        description={getServerErrorMessage(
          stats.error,
          t('Cache statistics are unavailable. Refresh to try again.')
        )}
        onRetry={refresh}
      />
    )
  }
  const snapshot = stats.data
  if (snapshot.status !== 'ready') {
    return (
      <div className='space-y-3'>
        <CacheConnectionWarning status={snapshot.status} />
        <Button
          type='button'
          variant='outline'
          disabled={stats.isFetching}
          onClick={() => void refresh()}
        >
          {t('Refresh')}
        </Button>
      </div>
    )
  }
  return (
    <div className='min-w-0 space-y-4'>
      {stats.isError && (
        <Alert role='status'>
          <AlertDescription>
            {t('Statistics are out of date. Refresh before clearing entries.')}
          </AlertDescription>
        </Alert>
      )}
      <CacheStatsSummary
        stats={snapshot}
        updatedAt={stats.dataUpdatedAt}
        refresh={() => void refresh()}
        fetching={stats.isFetching}
      />
      <CacheNodeStats nodes={snapshot.nodes} />
      <CacheEntries
        key={JSON.stringify([snapshot.cache_id, pagination])}
        stats={snapshot}
        pagination={pagination}
        setPagination={setPagination}
        stale={stats.isError}
        refresh={refresh}
      />
    </div>
  )
}
