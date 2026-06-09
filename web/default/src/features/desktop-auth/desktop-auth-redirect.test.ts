import assert from 'node:assert/strict'
import test from 'node:test'
import {
  buildDesktopAuthCallbackUrl,
  isAllowedDesktopRedirectUri,
} from './desktop-auth-redirect.ts'

test('allows only local AI Cove Design desktop callback URLs', () => {
  assert.equal(
    isAllowedDesktopRedirectUri(
      'http://127.0.0.1:8787/api/desktop-auth/callback'
    ),
    true
  )
  assert.equal(
    isAllowedDesktopRedirectUri(
      'http://localhost:8787/api/desktop-auth/callback'
    ),
    true
  )
  assert.equal(
    isAllowedDesktopRedirectUri(
      'https://ai-cove.com/api/desktop-auth/callback'
    ),
    false
  )
  assert.equal(isAllowedDesktopRedirectUri('https://evil.example/callback'), false)
  assert.equal(isAllowedDesktopRedirectUri('not a url'), false)
})

test('builds a desktop callback URL without leaking tokens to non-local URLs', () => {
  const callback = buildDesktopAuthCallbackUrl({
    redirectUri: 'http://127.0.0.1:8787/api/desktop-auth/callback',
    nonce: 'nonce-1',
    token: 'desktop-token',
    userId: 42,
  })

  assert.equal(
    callback,
    'http://127.0.0.1:8787/api/desktop-auth/callback?nonce=nonce-1&token=desktop-token&user_id=42'
  )

  assert.throws(() =>
    buildDesktopAuthCallbackUrl({
      redirectUri: 'https://evil.example/callback',
      nonce: 'nonce-1',
      token: 'desktop-token',
      userId: 42,
    })
  )
})
