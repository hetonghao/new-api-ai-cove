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
import { useMemo, useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import {
  Activity,
  BarChart3,
  RefreshCw,
  Search,
  Users as UsersIcon,
} from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { formatNumber, formatQuota, formatTimestampToDate } from '@/lib/format'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import {
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { GroupBadge } from '@/components/group-badge'
import { SectionPageLayout } from '@/components/layout'
import {
  getSalesData,
  getSalesDataByUser,
  getSalesGroups,
  getSalesUsers,
  updateSalesUserGroup,
} from './api'
import type { QuotaDataPoint, SalesUser } from './types'

const ALL_GROUPS_VALUE = '__all__'
const RANGE_SECONDS = {
  '7d': 7 * 24 * 60 * 60,
  '30d': 30 * 24 * 60 * 60,
  '90d': 90 * 24 * 60 * 60,
} as const

type RangeValue = keyof typeof RANGE_SECONDS

function sumQuotaData(data: QuotaDataPoint[]) {
  return data.reduce(
    (acc, item) => {
      acc.count += item.count || 0
      acc.quota += item.quota || 0
      acc.tokenUsed += item.token_used || 0
      return acc
    },
    { count: 0, quota: 0, tokenUsed: 0 }
  )
}

function Metric({
  icon: Icon,
  label,
  value,
}: {
  icon: typeof UsersIcon
  label: string
  value: string
}) {
  return (
    <div className='border-border/80 bg-card/40 flex min-h-24 items-center gap-3 rounded-md border px-4 py-3'>
      <div className='bg-muted text-muted-foreground flex size-9 shrink-0 items-center justify-center rounded-md'>
        <Icon className='size-4' />
      </div>
      <div className='min-w-0'>
        <div className='text-muted-foreground text-xs'>{label}</div>
        <div className='truncate text-xl font-semibold tabular-nums'>
          {value}
        </div>
      </div>
    </div>
  )
}

function QuotaDataTable({
  title,
  emptyText,
  data,
  firstColumn,
}: {
  title: string
  emptyText: string
  data: QuotaDataPoint[]
  firstColumn: 'model_name' | 'username'
}) {
  const { t } = useTranslation()

  return (
    <section className='border-border/80 rounded-md border'>
      <div className='flex h-11 items-center border-b px-3'>
        <h3 className='text-sm font-medium'>{title}</h3>
      </div>
      <Table>
        <TableHeader>
          <TableRow>
            <TableHead>
              {firstColumn === 'model_name' ? t('Model') : t('User')}
            </TableHead>
            <TableHead>{t('Time')}</TableHead>
            <TableHead className='text-right'>{t('Requests')}</TableHead>
            <TableHead className='text-right'>{t('Tokens')}</TableHead>
            <TableHead className='text-right'>{t('Quota')}</TableHead>
          </TableRow>
        </TableHeader>
        <TableBody>
          {data.length === 0 ? (
            <TableRow>
              <TableCell
                colSpan={5}
                className='text-muted-foreground h-24 text-center'
              >
                {emptyText}
              </TableCell>
            </TableRow>
          ) : (
            data.slice(0, 12).map((item, index) => (
              <TableRow
                key={`${item[firstColumn] || 'unknown'}-${item.created_at}-${index}`}
              >
                <TableCell className='font-medium'>
                  {item[firstColumn] || '-'}
                </TableCell>
                <TableCell>{formatTimestampToDate(item.created_at)}</TableCell>
                <TableCell className='text-right tabular-nums'>
                  {formatNumber(item.count)}
                </TableCell>
                <TableCell className='text-right tabular-nums'>
                  {formatNumber(item.token_used)}
                </TableCell>
                <TableCell className='text-right tabular-nums'>
                  {formatQuota(item.quota)}
                </TableCell>
              </TableRow>
            ))
          )}
        </TableBody>
      </Table>
    </section>
  )
}

export function Sales() {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const [keyword, setKeyword] = useState('')
  const [groupFilter, setGroupFilter] = useState('')
  const [page, setPage] = useState(1)
  const [range, setRange] = useState<RangeValue>('30d')
  const pageSize = 20

  const dateRange = useMemo(() => {
    const end = Math.floor(Date.now() / 1000)
    return {
      start_timestamp: end - RANGE_SECONDS[range],
      end_timestamp: end,
    }
  }, [range])

  const usersQuery = useQuery({
    queryKey: ['sales-users', page, pageSize, keyword, groupFilter],
    queryFn: async () => {
      const result = await getSalesUsers({
        p: page,
        page_size: pageSize,
        keyword,
        group: groupFilter,
      })
      if (!result.success) {
        toast.error(result.message || t('Failed to load sales users'))
        return { items: [], total: 0, page, page_size: pageSize }
      }
      return (
        result.data || {
          items: [],
          total: 0,
          page,
          page_size: pageSize,
        }
      )
    },
    placeholderData: (previousData) => previousData,
  })

  const groupsQuery = useQuery({
    queryKey: ['sales-groups'],
    queryFn: getSalesGroups,
    staleTime: 5 * 60 * 1000,
  })

  const modelDataQuery = useQuery({
    queryKey: ['sales-data', dateRange],
    queryFn: async () => {
      const result = await getSalesData(dateRange)
      if (!result.success) {
        toast.error(result.message || t('Failed to load sales data'))
        return []
      }
      return result.data || []
    },
  })

  const userDataQuery = useQuery({
    queryKey: ['sales-data-users', dateRange],
    queryFn: async () => {
      const result = await getSalesDataByUser(dateRange)
      if (!result.success) {
        toast.error(result.message || t('Failed to load sales data'))
        return []
      }
      return result.data || []
    },
  })

  const updateGroupMutation = useMutation({
    mutationFn: ({ userId, group }: { userId: number; group: string }) =>
      updateSalesUserGroup(userId, group),
    onSuccess: (result) => {
      if (result.success) {
        toast.success(t('Group updated successfully'))
        queryClient.invalidateQueries({ queryKey: ['sales-users'] })
        return
      }
      toast.error(result.message || t('Failed to update user group'))
    },
    onError: () => toast.error(t('Failed to update user group')),
  })

  const users = usersQuery.data?.items || []
  const totalUsers = usersQuery.data?.total || 0
  const modelData = modelDataQuery.data || []
  const userData = userDataQuery.data || []
  const totals = sumQuotaData(modelData)
  const pageCount = Math.max(1, Math.ceil(totalUsers / pageSize))
  const groups = groupsQuery.data?.data || []

  const handleGroupChange = (user: SalesUser, group: string | null) => {
    if (!group || group === user.group) return
    updateGroupMutation.mutate({ userId: user.id, group })
  }

  return (
    <SectionPageLayout>
      <SectionPageLayout.Title>{t('Sales')}</SectionPageLayout.Title>
      <SectionPageLayout.Description>
        {t('Track invited users and sales usage data')}
      </SectionPageLayout.Description>
      <SectionPageLayout.Actions>
        <Select
          items={[
            { value: '7d', label: t('Last 7 days') },
            { value: '30d', label: t('Last 30 days') },
            { value: '90d', label: t('Last 90 days') },
          ]}
          value={range}
          onValueChange={(value) => value && setRange(value as RangeValue)}
        >
          <SelectTrigger className='w-36'>
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            <SelectGroup>
              <SelectItem value='7d'>{t('Last 7 days')}</SelectItem>
              <SelectItem value='30d'>{t('Last 30 days')}</SelectItem>
              <SelectItem value='90d'>{t('Last 90 days')}</SelectItem>
            </SelectGroup>
          </SelectContent>
        </Select>
      </SectionPageLayout.Actions>
      <SectionPageLayout.Content>
        <div className='space-y-4'>
          <div className='grid gap-3 md:grid-cols-3'>
            <Metric
              icon={UsersIcon}
              label={t('Invited Users')}
              value={formatNumber(totalUsers)}
            />
            <Metric
              icon={Activity}
              label={t('Sales Requests')}
              value={formatNumber(totals.count)}
            />
            <Metric
              icon={BarChart3}
              label={t('Sales Quota')}
              value={formatQuota(totals.quota)}
            />
          </div>

          <section className='border-border/80 rounded-md border'>
            <div className='flex flex-col gap-3 border-b p-3 md:flex-row md:items-center md:justify-between'>
              <div className='flex min-w-0 flex-1 items-center gap-2'>
                <div className='relative min-w-0 flex-1 md:max-w-sm'>
                  <Search className='text-muted-foreground pointer-events-none absolute top-1/2 left-2.5 size-4 -translate-y-1/2' />
                  <Input
                    value={keyword}
                    onChange={(event) => {
                      setKeyword(event.target.value)
                      setPage(1)
                    }}
                    placeholder={t('Search invited users')}
                    className='pl-8'
                  />
                </div>
                <Select
                  items={[
                    { value: ALL_GROUPS_VALUE, label: t('All Groups') },
                    ...groups.map((group) => ({ value: group, label: group })),
                  ]}
                  value={groupFilter || ALL_GROUPS_VALUE}
                  onValueChange={(value) => {
                    setGroupFilter(
                      value === ALL_GROUPS_VALUE ? '' : value || ''
                    )
                    setPage(1)
                  }}
                >
                  <SelectTrigger className='w-36'>
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectGroup>
                      <SelectItem value={ALL_GROUPS_VALUE}>
                        {t('All Groups')}
                      </SelectItem>
                      {groups.map((group) => (
                        <SelectItem key={group} value={group}>
                          {group}
                        </SelectItem>
                      ))}
                    </SelectGroup>
                  </SelectContent>
                </Select>
              </div>
              <Button
                type='button'
                variant='outline'
                onClick={() => usersQuery.refetch()}
                disabled={usersQuery.isFetching}
              >
                <RefreshCw className='size-4' />
                {t('Refresh')}
              </Button>
            </div>

            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>{t('Username')}</TableHead>
                  <TableHead>{t('Group')}</TableHead>
                  <TableHead className='text-right'>
                    {t('Remaining Quota')}
                  </TableHead>
                  <TableHead className='text-right'>
                    {t('Used Quota')}
                  </TableHead>
                  <TableHead className='text-right'>{t('Requests')}</TableHead>
                  <TableHead>{t('Created')}</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {users.length === 0 ? (
                  <TableRow>
                    <TableCell
                      colSpan={6}
                      className='text-muted-foreground h-28 text-center'
                    >
                      {usersQuery.isLoading
                        ? t('Loading...')
                        : t('No invited users found')}
                    </TableCell>
                  </TableRow>
                ) : (
                  users.map((user) => (
                    <TableRow key={user.id}>
                      <TableCell>
                        <div className='font-medium'>{user.username}</div>
                        {user.email && (
                          <div className='text-muted-foreground text-xs'>
                            {user.email}
                          </div>
                        )}
                      </TableCell>
                      <TableCell>
                        <div className='flex items-center gap-2'>
                          <GroupBadge group={user.group} />
                          <Select
                            items={groups.map((group) => ({
                              value: group,
                              label: group,
                            }))}
                            value={user.group}
                            onValueChange={(value) =>
                              handleGroupChange(user, value)
                            }
                            disabled={
                              updateGroupMutation.isPending &&
                              updateGroupMutation.variables?.userId === user.id
                            }
                          >
                            <SelectTrigger size='sm' className='w-28'>
                              <SelectValue />
                            </SelectTrigger>
                            <SelectContent>
                              <SelectGroup>
                                {groups.map((group) => (
                                  <SelectItem key={group} value={group}>
                                    {group}
                                  </SelectItem>
                                ))}
                              </SelectGroup>
                            </SelectContent>
                          </Select>
                        </div>
                      </TableCell>
                      <TableCell className='text-right tabular-nums'>
                        {formatQuota(user.quota)}
                      </TableCell>
                      <TableCell className='text-right tabular-nums'>
                        {formatQuota(user.used_quota)}
                      </TableCell>
                      <TableCell className='text-right tabular-nums'>
                        {formatNumber(user.request_count)}
                      </TableCell>
                      <TableCell>
                        {formatTimestampToDate(user.created_at)}
                      </TableCell>
                    </TableRow>
                  ))
                )}
              </TableBody>
            </Table>

            <div className='flex items-center justify-between border-t px-3 py-2'>
              <div className='text-muted-foreground text-sm'>
                {t('Page')} {page} / {pageCount}
              </div>
              <div className='flex gap-2'>
                <Button
                  type='button'
                  variant='outline'
                  size='sm'
                  disabled={page <= 1}
                  onClick={() => setPage((current) => Math.max(1, current - 1))}
                >
                  {t('Previous')}
                </Button>
                <Button
                  type='button'
                  variant='outline'
                  size='sm'
                  disabled={page >= pageCount}
                  onClick={() =>
                    setPage((current) => Math.min(pageCount, current + 1))
                  }
                >
                  {t('Next')}
                </Button>
              </div>
            </div>
          </section>

          <div className='grid gap-4 xl:grid-cols-2'>
            <QuotaDataTable
              title={t('Usage by Model')}
              emptyText={
                modelDataQuery.isLoading
                  ? t('Loading...')
                  : t('No sales data found')
              }
              data={modelData}
              firstColumn='model_name'
            />
            <QuotaDataTable
              title={t('Usage by User')}
              emptyText={
                userDataQuery.isLoading
                  ? t('Loading...')
                  : t('No sales data found')
              }
              data={userData}
              firstColumn='username'
            />
          </div>
        </div>
      </SectionPageLayout.Content>
    </SectionPageLayout>
  )
}
