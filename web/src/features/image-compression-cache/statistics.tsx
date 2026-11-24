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
import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import { ErrorState } from '@/components/error-state'
import { LoadingState } from '@/components/loading-state'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Label } from '@/components/ui/label'
import { NativeSelect, NativeSelectOption } from '@/components/ui/native-select'
import { getServerErrorMessage } from '@/lib/server-error-message'

import { getCacheStats, listCacheNodes, nodesKey, statsKey } from './api'
import { CacheEntries } from './entries'
import { CacheStatsSummary } from './stats-summary'

const refreshInterval = 5000

type NodesQuery = ReturnType<typeof useCacheNodes>

function useCacheNodes() {
  return useQuery({
    queryKey: nodesKey,
    queryFn: listCacheNodes,
    refetchInterval: refreshInterval,
    meta: { errorToast: false },
  })
}

export function CacheStatistics() {
  const nodes = useCacheNodes()
  if (nodes.isPending) return <LoadingState />
  if (!nodes.data) {
    return (
      <ErrorState
        description={getServerErrorMessage(nodes.error)}
        onRetry={() => void nodes.refetch()}
      />
    )
  }
  return (
    <NodeStatistics nodes={nodes} initialNode={nodes.data.local_node_name} />
  )
}

function NodeStatistics(props: {
  readonly nodes: NodesQuery
  readonly initialNode: string
}) {
  const { t } = useTranslation()
  const [nodeName, setNodeName] = useState(props.initialNode)
  const nodes = props.nodes.data?.nodes ?? []
  return (
    <div className='space-y-4'>
      <div className='flex flex-wrap items-center gap-2'>
        <Label htmlFor='compression-cache-node'>{t('Node')}</Label>
        <NativeSelect
          id='compression-cache-node'
          value={nodeName}
          onChange={(event) => setNodeName(event.target.value)}
        >
          {!nodes.some((node) => node.node_name === nodeName) && (
            <NativeSelectOption value={nodeName}>{nodeName}</NativeSelectOption>
          )}
          {nodes.map((node) => (
            <NativeSelectOption key={node.node_name} value={node.node_name}>
              {node.node_name}
              {node.status === 'unavailable' ? ` · ${t('Unavailable')}` : null}
              {node.status === 'stale' ? ` · ${t('Stale')}` : null}
            </NativeSelectOption>
          ))}
        </NativeSelect>
      </div>
      {props.nodes.isError && (
        <Alert>
          <AlertDescription>
            {t('The node list could not be refreshed.')}
          </AlertDescription>
        </Alert>
      )}
      <NodeSnapshot key={nodeName} nodeName={nodeName} />
    </div>
  )
}

function NodeSnapshot(props: { readonly nodeName: string }) {
  const { t } = useTranslation()
  const [pagination, setPagination] = useState({ pageIndex: 0, pageSize: 20 })
  const stats = useQuery({
    queryKey: [...statsKey, props.nodeName, pagination],
    queryFn: ({ signal }) =>
      getCacheStats(
        {
          node_name: props.nodeName,
          page: pagination.pageIndex + 1,
          page_size: pagination.pageSize,
        },
        signal
      ),
    refetchInterval: refreshInterval,
    retry: false,
    meta: { errorToast: false },
  })
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
  return (
    <>
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
      <CacheEntries
        key={JSON.stringify([snapshot.instance_id, pagination])}
        stats={snapshot}
        pagination={pagination}
        setPagination={setPagination}
        stale={stats.isError}
        refresh={refresh}
      />
    </>
  )
}
