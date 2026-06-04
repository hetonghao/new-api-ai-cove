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
/* eslint-disable react-refresh/only-export-components */
import { createContext, useContext, useState, type ReactNode } from 'react'
import type { NavigateFn } from '@/hooks/use-table-url-state'
import type { ChannelAffinityInfo } from '../types'

export type UsageLogsDataScope = 'default' | 'sales'

interface UsageLogsContextValue {
  selectedUserId: number | null
  setSelectedUserId: (userId: number | null) => void
  userInfoDialogOpen: boolean
  setUserInfoDialogOpen: (open: boolean) => void
  affinityTarget: ChannelAffinityInfo | null
  setAffinityTarget: (target: ChannelAffinityInfo | null) => void
  affinityDialogOpen: boolean
  setAffinityDialogOpen: (open: boolean) => void
  sensitiveVisible: boolean
  setSensitiveVisible: (visible: boolean) => void
  advancedFilterExpansionRequest: number
  requestAdvancedFilterExpansion: () => void
  search: Record<string, unknown>
  navigateSearch: NavigateFn
  dataScope: UsageLogsDataScope
  adminControls?: boolean
  hideSelfControl?: boolean
  userInfoEnabled: boolean
  detailsAdmin?: boolean
  affinityStatsEnabled: boolean
  queryKeyScope: string
}

const UsageLogsContext = createContext<UsageLogsContextValue | undefined>(
  undefined
)

interface UsageLogsProviderProps {
  children: ReactNode
  search: Record<string, unknown>
  navigateSearch: NavigateFn
  dataScope?: UsageLogsDataScope
  adminControls?: boolean
  hideSelfControl?: boolean
  userInfoEnabled?: boolean
  detailsAdmin?: boolean
  affinityStatsEnabled?: boolean
  queryKeyScope?: string
}

export function UsageLogsProvider({
  children,
  search,
  navigateSearch,
  dataScope = 'default',
  adminControls,
  hideSelfControl,
  userInfoEnabled = true,
  detailsAdmin,
  affinityStatsEnabled = true,
  queryKeyScope = dataScope,
}: UsageLogsProviderProps) {
  const [selectedUserId, setSelectedUserId] = useState<number | null>(null)
  const [userInfoDialogOpen, setUserInfoDialogOpen] = useState(false)
  const [affinityTarget, setAffinityTarget] =
    useState<ChannelAffinityInfo | null>(null)
  const [affinityDialogOpen, setAffinityDialogOpen] = useState(false)
  const [sensitiveVisible, setSensitiveVisible] = useState(true)
  const [advancedFilterExpansionRequest, setAdvancedFilterExpansionRequest] =
    useState(0)

  return (
    <UsageLogsContext.Provider
      value={{
        selectedUserId,
        setSelectedUserId,
        userInfoDialogOpen,
        setUserInfoDialogOpen,
        affinityTarget,
        setAffinityTarget,
        affinityDialogOpen,
        setAffinityDialogOpen,
        sensitiveVisible,
        setSensitiveVisible,
        advancedFilterExpansionRequest,
        requestAdvancedFilterExpansion: () =>
          setAdvancedFilterExpansionRequest((request) => request + 1),
        search,
        navigateSearch,
        dataScope,
        adminControls,
        hideSelfControl,
        userInfoEnabled,
        detailsAdmin,
        affinityStatsEnabled,
        queryKeyScope,
      }}
    >
      {children}
    </UsageLogsContext.Provider>
  )
}

export function useUsageLogsContext() {
  const context = useContext(UsageLogsContext)
  if (!context) {
    throw new Error('useUsageLogsContext must be used within UsageLogsProvider')
  }
  return context
}
