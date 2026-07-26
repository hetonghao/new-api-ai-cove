import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import path from 'node:path'
import test from 'node:test'
import { fileURLToPath } from 'node:url'
import {
  LEGAL_CONSENT_MESSAGE,
  getPrimarySubmitButtonState,
  guardPrimarySubmitAction,
} from './legal-consent-guard.ts'

const __dirname = path.dirname(fileURLToPath(import.meta.url))
const signInSource = readFileSync(
  path.join(__dirname, '../sign-in/components/user-auth-form.tsx'),
  'utf8'
)
const signUpSource = readFileSync(
  path.join(__dirname, '../sign-up/components/sign-up-form.tsx'),
  'utf8'
)

test('sign-in primary button stays natively clickable during legal block', () => {
  assert.deepEqual(
    getPrimarySubmitButtonState({
      isActuallyDisabled: false,
      requiresLegalConsent: true,
      agreedToLegal: false,
    }),
    {
      disabled: false,
      ariaDisabled: true,
      visuallyDisabled: true,
    }
  )
})

test('sign-in primary button click shows legal consent message and intercepts submit during legal block', () => {
  const messages: string[] = []

  const result = guardPrimarySubmitAction({
    requiresLegalConsent: true,
    agreedToLegal: false,
    onBlocked: message => {
      messages.push(message)
    },
  })

  assert.deepEqual(messages, [LEGAL_CONSENT_MESSAGE])
  assert.deepEqual(result, {
    intercepted: true,
    message: LEGAL_CONSENT_MESSAGE,
  })
})

test('sign-up primary button click does not prompt and allows submit when legal consent is satisfied', () => {
  const messages: string[] = []

  const result = guardPrimarySubmitAction({
    requiresLegalConsent: true,
    agreedToLegal: true,
    onBlocked: message => {
      messages.push(message)
    },
  })

  assert.deepEqual(messages, [])
  assert.deepEqual(result, {
    intercepted: false,
    message: null,
  })
})

test('sign-up preserves real disabled state such as turnstile not ready', () => {
  assert.deepEqual(
    getPrimarySubmitButtonState({
      isActuallyDisabled: true,
      requiresLegalConsent: false,
      agreedToLegal: false,
    }),
    {
      disabled: true,
      ariaDisabled: false,
      visuallyDisabled: false,
    }
  )
})

test('sign-up keeps real disabled state even when legal consent is also blocked', () => {
  assert.deepEqual(
    getPrimarySubmitButtonState({
      isActuallyDisabled: true,
      requiresLegalConsent: true,
      agreedToLegal: false,
    }),
    {
      disabled: true,
      ariaDisabled: true,
      visuallyDisabled: true,
    }
  )
})

test('default auth forms wire the primary submit contract into the components', () => {
  assert.match(signInSource, /guardPrimarySubmitAction\(\{/)
  assert.match(signInSource, /disabled=\{primarySubmitButtonState\.disabled\}/)
  assert.match(
    signInSource,
    /aria-disabled=\{primarySubmitButtonState\.ariaDisabled\}/
  )

  assert.match(signUpSource, /guardPrimarySubmitAction\(\{/)
  assert.match(
    signUpSource,
    /isActuallyDisabled: isLoading \|\| !turnstileReady/
  )
  assert.match(signUpSource, /disabled=\{primarySubmitButtonState\.disabled\}/)
  assert.match(
    signUpSource,
    /aria-disabled=\{primarySubmitButtonState\.ariaDisabled\}/
  )
})
