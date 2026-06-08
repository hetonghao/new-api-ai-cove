export interface DesktopAuthCallbackInput {
  redirectUri: string
  nonce: string
  token: string
  userId: string | number
}

const ALLOWED_DESKTOP_CALLBACK_PATH = '/api/desktop-auth/callback'
const ALLOWED_DESKTOP_CALLBACK_HOSTS = new Set(['127.0.0.1', 'localhost'])

export function isAllowedDesktopRedirectUri(value: string): boolean {
  try {
    const url = new URL(value)
    return (
      url.protocol === 'http:' &&
      ALLOWED_DESKTOP_CALLBACK_HOSTS.has(url.hostname) &&
      url.pathname === ALLOWED_DESKTOP_CALLBACK_PATH
    )
  } catch {
    return false
  }
}

export function buildDesktopAuthCallbackUrl(input: DesktopAuthCallbackInput) {
  const nonce = input.nonce.trim()
  const token = input.token.trim()
  const userId = String(input.userId).trim()

  if (
    !isAllowedDesktopRedirectUri(input.redirectUri) ||
    !nonce ||
    !token ||
    !userId
  ) {
    throw new Error('Invalid AI-Cove-Design desktop auth callback.')
  }

  const callbackUrl = new URL(input.redirectUri)
  callbackUrl.searchParams.set('nonce', nonce)
  callbackUrl.searchParams.set('token', token)
  callbackUrl.searchParams.set('user_id', userId)
  return callbackUrl.toString()
}
