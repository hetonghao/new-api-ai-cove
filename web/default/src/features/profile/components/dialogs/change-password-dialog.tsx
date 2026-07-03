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
import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { Dialog } from '@/components/dialog'
import { PasswordInput } from '@/components/password-input'
import { Button } from '@/components/ui/button'
import { Label } from '@/components/ui/label'

import { updateUserProfile } from '../../api'
import {
  buildPasswordUpdatePayload,
  type PasswordDialogMode,
} from '../../lib/password-dialog'

// ============================================================================
// Change Password Dialog Component
// ============================================================================

interface ChangePasswordDialogProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  username: string
  mode: PasswordDialogMode
  onSuccess: () => Promise<void>
}

export function ChangePasswordDialog({
  open,
  onOpenChange,
  username,
  mode,
  onSuccess,
}: ChangePasswordDialogProps) {
  const { t } = useTranslation()
  const [loading, setLoading] = useState(false)
  const [formData, setFormData] = useState({
    originalPassword: '',
    newPassword: '',
    confirmPassword: '',
  })

  const handleChange = (field: string, value: string) => {
    setFormData((prev) => ({ ...prev, [field]: value }))
  }

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault()

    // Validation
    if (mode === 'change' && !formData.originalPassword) {
      toast.error(t('Please enter your current password'))
      return
    }

    if (!formData.newPassword) {
      toast.error(t('Please enter a new password'))
      return
    }

    if (formData.newPassword.length < 8) {
      toast.error(t('Password must be at least 8 characters'))
      return
    }

    if (
      mode === 'change' &&
      formData.originalPassword === formData.newPassword
    ) {
      toast.error(t('New password must be different from current password'))
      return
    }

    if (formData.newPassword !== formData.confirmPassword) {
      toast.error(t('Passwords do not match'))
      return
    }

    try {
      setLoading(true)
      const response = await updateUserProfile(
        buildPasswordUpdatePayload(mode, {
          originalPassword: formData.originalPassword,
          newPassword: formData.newPassword,
        })
      )

      if (response.success) {
        toast.success(
          mode === 'setup'
            ? t('Password set successfully')
            : t('Password changed successfully')
        )
        await onSuccess()
        onOpenChange(false)
        setFormData({
          originalPassword: '',
          newPassword: '',
          confirmPassword: '',
        })
      } else {
        toast.error(response.message || t('Failed to change password'))
      }
    } catch (_error) {
      toast.error(t('Failed to change password'))
    } finally {
      setLoading(false)
    }
  }

  const formId = 'change-password-form'

  return (
    <Dialog
      open={open}
      onOpenChange={onOpenChange}
      title={mode === 'setup' ? t('Set Password') : t('Change Password')}
      description={
        <>
          {mode === 'setup'
            ? t('Set your first password for account:')
            : t('Update your password for account:')}{' '}
          <strong>{username}</strong>
        </>
      }
      contentClassName='sm:max-w-md'
      contentHeight='auto'
      bodyClassName='space-y-4'
      footer={
        <>
          <Button
            type='button'
            variant='outline'
            onClick={() => onOpenChange(false)}
            disabled={loading}
          >
            {t('Cancel')}
          </Button>
          <Button type='submit' form={formId} disabled={loading}>
            {loading && <Loader2 className='mr-2 h-4 w-4 animate-spin' />}
            {loading
              ? mode === 'setup'
                ? t('Setting...')
                : t('Changing...')
              : mode === 'setup'
                ? t('Set Password')
                : t('Change Password')}
          </Button>
        </>
      }
    >
      <form id={formId} onSubmit={handleSubmit} className='space-y-4'>
        {mode === 'change' && (
          <div className='space-y-2'>
            <Label htmlFor='currentPassword'>{t('Current Password')}</Label>
            <PasswordInput
              id='currentPassword'
              value={formData.originalPassword}
              onChange={(e) => handleChange('originalPassword', e.target.value)}
              disabled={loading}
              required
              autoComplete='current-password'
            />
          </div>
        )}

        <div className='space-y-2'>
          <Label htmlFor='newPassword'>{t('New Password')}</Label>
          <PasswordInput
            id='newPassword'
            value={formData.newPassword}
            onChange={(e) => handleChange('newPassword', e.target.value)}
            disabled={loading}
            required
            minLength={8}
            autoComplete='new-password'
          />
          <p className='text-muted-foreground text-xs'>
            {t('Must be at least 8 characters')}
          </p>
        </div>

        <div className='space-y-2'>
          <Label htmlFor='confirmPassword'>{t('Confirm New Password')}</Label>
          <PasswordInput
            id='confirmPassword'
            value={formData.confirmPassword}
            onChange={(e) => handleChange('confirmPassword', e.target.value)}
            disabled={loading}
            required
            autoComplete='new-password'
          />
        </div>
      </form>
    </Dialog>
  )
}
