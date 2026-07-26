import assert from 'node:assert/strict'
import test from 'node:test'
import {
  buildPasswordUpdatePayload,
  getPasswordDialogMode,
} from './password-dialog.ts'

test('uses change mode when user already has password', () => {
  assert.equal(getPasswordDialogMode(true), 'change')
})

test('uses setup mode when user has no password', () => {
  assert.equal(getPasswordDialogMode(false), 'setup')
})

test('builds change-password payload with original password', () => {
  assert.deepEqual(
    buildPasswordUpdatePayload('change', {
      originalPassword: 'old-pass-123',
      newPassword: 'new-pass-123',
    }),
    {
      original_password: 'old-pass-123',
      password: 'new-pass-123',
    }
  )
})

test('builds first-password payload without original password', () => {
  assert.deepEqual(
    buildPasswordUpdatePayload('setup', {
      originalPassword: '',
      newPassword: 'new-pass-123',
    }),
    {
      password: 'new-pass-123',
    }
  )
})
