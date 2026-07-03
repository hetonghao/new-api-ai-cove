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
import { Loader2 } from 'lucide-react'
import { useCallback, useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { Dialog } from '@/components/dialog'
import { Button } from '@/components/ui/button'
import { Label } from '@/components/ui/label'
import { formatCompactNumber, formatQuota } from '@/lib/format'

import { getUserInfo } from '../../api'
import type { UserInfo } from '../../types'

type FetchUserInfo = (userId: number) => Promise<{
  success: boolean
  message?: string
  data?: UserInfo
}>

interface UserInfoDialogProps {
  userId: number | null
  open: boolean
  onOpenChange: (open: boolean) => void
  onFilterByUsername?: (username: string) => void
  fetchUserInfo?: FetchUserInfo
}

export function UserInfoDialog({
  userId,
  open,
  onOpenChange,
  onFilterByUsername,
  fetchUserInfo: fetchUserInfoProp = getUserInfo,
}: UserInfoDialogProps) {
  const { t } = useTranslation()
  const [userInfo, setUserInfo] = useState<UserInfo | null>(null)
  const [isLoading, setIsLoading] = useState(false)

  const fetchUserInfo = useCallback(
    async (id: number) => {
      setIsLoading(true)
      setUserInfo(null)
      try {
        const result = await fetchUserInfoProp(id)
        if (result.success) {
          setUserInfo(result.data || null)
        } else {
          toast.error(result.message || t('Failed to fetch user information'))
        }
      } catch (error) {
        // eslint-disable-next-line no-console
        console.error('Failed to fetch user info:', error)
        toast.error(t('Failed to fetch user information'))
      } finally {
        setIsLoading(false)
      }
    },
    [fetchUserInfoProp, t]
  )

  useEffect(() => {
    if (open && userId) {
      fetchUserInfo(userId)
    }
  }, [open, userId, fetchUserInfo])

  const InfoItem = ({
    label,
    value,
  }: {
    label: string
    value: string | number
  }) => (
    <div className='space-y-1.5'>
      <Label className='text-muted-foreground text-xs'>{label}</Label>
      <div className='text-sm font-semibold'>{value}</div>
    </div>
  )

  return (
    <Dialog
      open={open}
      onOpenChange={onOpenChange}
      title={t('User Information')}
      contentClassName='max-w-md'
    >
      {isLoading ? (
        <div className='flex items-center justify-center py-8'>
          <Loader2 className='h-6 w-6 animate-spin' />
        </div>
      ) : userInfo ? (
        <div className='space-y-4'>
          <div className='grid grid-cols-2 gap-4'>
            <InfoItem label={t('Username')} value={userInfo.username} />
            <InfoItem label={t('Group')} value={userInfo.group ?? '-'} />
            <InfoItem
              label={t('Remaining Quota')}
              value={formatQuota(userInfo.quota)}
            />
            <InfoItem
              label={t('Used Quota')}
              value={formatQuota(userInfo.used_quota)}
            />
            <InfoItem
              label={t('Request Count')}
              value={formatCompactNumber(userInfo.request_count)}
            />
          </div>
          {onFilterByUsername && (
            <Button
              type='button'
              variant='outline'
              className='w-full'
              onClick={() => {
                onFilterByUsername(userInfo.username)
                onOpenChange(false)
              }}
            >
              {t('Filter logs by this user')}
            </Button>
          )}
        </div>
      ) : (
        <div className='text-muted-foreground py-8 text-center'>
          {t('No user information available')}
        </div>
      )}
    </Dialog>
  )
}
