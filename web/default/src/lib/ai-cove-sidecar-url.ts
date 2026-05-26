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

const AI_COVE_DESIGN_SIDECAR_PATH = '/sidecars/gpt-image-canvas/'
const AI_COVE_HELP_DOCS_SIDECAR_PATH = '/sidecars/help-docs/'
const DEFAULT_LOCAL_SIDECAR_PORT = 4174

type AiCoveSidecarEnv = {
  VITE_AI_COVE_GATEWAY_BASE_URL?: string
  VITE_AI_COVE_SIDECAR_BASE_URL?: string
  VITE_REACT_APP_SERVER_URL?: string
  VITE_REACT_APP_SIDECAR_BASE_URL?: string
}

type AiCoveSidecarTheme = 'dark' | 'light'

function getEnv(): AiCoveSidecarEnv {
  return (import.meta.env ?? {}) as AiCoveSidecarEnv
}

function getCurrentOrigin(): string {
  if (typeof window !== 'undefined') {
    return window.location.origin
  }
  return 'http://localhost:3000'
}

function isLoopbackHostname(hostname: string): boolean {
  return (
    hostname === '127.0.0.1' ||
    hostname === 'localhost' ||
    hostname === '::1'
  )
}

function isLocalHost(): boolean {
  if (typeof window === 'undefined') {
    return false
  }
  return isLoopbackHostname(window.location.hostname)
}

function createCurrentHostOrigin(port?: number | string): string {
  if (typeof window === 'undefined') {
    const fallbackPort = port ? String(port) : '3000'
    return `http://127.0.0.1:${fallbackPort}`
  }

  const url = new URL(window.location.href)
  if (port) {
    url.port = String(port)
  }
  url.pathname = ''
  url.search = ''
  url.hash = ''
  return url.origin
}

function rehostLocalUrl(raw: string | undefined): string {
  if (!raw || typeof window === 'undefined') {
    return ''
  }

  try {
    const url = new URL(raw)
    if (!isLoopbackHostname(url.hostname)) {
      return url.toString().replace(/\/+$/u, '')
    }
    url.hostname = window.location.hostname
    return url.toString().replace(/\/+$/u, '')
  } catch {
    return raw
  }
}

function getGatewayBaseUrl(): string {
  const env = getEnv()
  if (isLocalHost()) {
    return (
      rehostLocalUrl(env.VITE_AI_COVE_GATEWAY_BASE_URL) ||
      rehostLocalUrl(env.VITE_REACT_APP_SERVER_URL) ||
      getCurrentOrigin()
    )
  }
  return (
    env.VITE_AI_COVE_GATEWAY_BASE_URL ||
    env.VITE_REACT_APP_SERVER_URL ||
    getCurrentOrigin()
  )
}

function getSidecarOrigin(): string {
  const env = getEnv()
  if (isLocalHost()) {
    return (
      rehostLocalUrl(env.VITE_AI_COVE_SIDECAR_BASE_URL) ||
      rehostLocalUrl(env.VITE_REACT_APP_SIDECAR_BASE_URL) ||
      createCurrentHostOrigin(DEFAULT_LOCAL_SIDECAR_PORT)
    )
  }
  return (
    env.VITE_AI_COVE_SIDECAR_BASE_URL ||
    env.VITE_REACT_APP_SIDECAR_BASE_URL ||
    getCurrentOrigin()
  )
}

function normalizeUserId(userId: number | string | null | undefined): string {
  if (typeof userId === 'number' && Number.isFinite(userId)) {
    return String(userId)
  }

  if (typeof userId === 'string') {
    return userId.trim()
  }

  return ''
}

function normalizeTheme(theme: AiCoveSidecarTheme | null | undefined): string {
  return theme === 'dark' || theme === 'light' ? theme : ''
}

export function createAiCoveDesignSidecarUrl(
  userId?: number | string | null,
  theme?: AiCoveSidecarTheme | null
): string {
  const url = new URL(AI_COVE_DESIGN_SIDECAR_PATH, getSidecarOrigin())
  const params = new URLSearchParams({
    base_url: getGatewayBaseUrl(),
    ui_mode: 'embedded',
  })

  const normalizedUserId = normalizeUserId(userId)
  if (normalizedUserId) {
    params.set('user_id', normalizedUserId)
  }

  const normalizedTheme = normalizeTheme(theme)
  if (normalizedTheme) {
    params.set('theme', normalizedTheme)
  }

  url.search = params.toString()
  return url.toString()
}

export function createAiCoveHelpDocsSidecarUrl(): string {
  return new URL(AI_COVE_HELP_DOCS_SIDECAR_PATH, getSidecarOrigin()).toString()
}
