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
import type { UpdateUserRequest } from '../types'

export type PasswordDialogMode = 'change' | 'setup'

export interface PasswordDialogFormValues {
  originalPassword: string
  newPassword: string
}

export function getPasswordDialogMode(
  hasPassword: boolean
): PasswordDialogMode {
  return hasPassword ? 'change' : 'setup'
}

export function buildPasswordUpdatePayload(
  mode: PasswordDialogMode,
  values: PasswordDialogFormValues
): UpdateUserRequest {
  if (mode === 'setup') {
    return {
      password: values.newPassword,
    }
  }

  return {
    original_password: values.originalPassword,
    password: values.newPassword,
  }
}
