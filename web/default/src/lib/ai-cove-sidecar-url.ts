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
const DEFAULT_LOCAL_SIDECAR_ORIGIN = 'http://127.0.0.1:4174'
const DEFAULT_LOCAL_GATEWAY_ORIGIN = 'http://127.0.0.1:38080'

type AiCoveSidecarEnv = {
  VITE_AI_COVE_GATEWAY_BASE_URL?: string
  VITE_AI_COVE_SIDECAR_BASE_URL?: string
  VITE_REACT_APP_SERVER_URL?: string
  VITE_REACT_APP_SIDECAR_BASE_URL?: string
}

function getEnv(): AiCoveSidecarEnv {
  return (import.meta.env ?? {}) as AiCoveSidecarEnv
}

function getCurrentOrigin(): string {
  if (typeof window !== 'undefined') {
    return window.location.origin
  }
  return 'http://localhost:3000'
}

function isLocalHost(): boolean {
  if (typeof window === 'undefined') {
    return false
  }
  return (
    window.location.hostname === '127.0.0.1' ||
    window.location.hostname === 'localhost' ||
    window.location.hostname === '::1'
  )
}

function getGatewayBaseUrl(): string {
  const env = getEnv()
  return (
    env.VITE_AI_COVE_GATEWAY_BASE_URL ||
    env.VITE_REACT_APP_SERVER_URL ||
    (isLocalHost() ? DEFAULT_LOCAL_GATEWAY_ORIGIN : undefined) ||
    getCurrentOrigin()
  )
}

function getSidecarOrigin(): string {
  const env = getEnv()
  return (
    env.VITE_AI_COVE_SIDECAR_BASE_URL ||
    env.VITE_REACT_APP_SIDECAR_BASE_URL ||
    (isLocalHost() ? DEFAULT_LOCAL_SIDECAR_ORIGIN : undefined) ||
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

export function createAiCoveDesignSidecarUrl(
  userId?: number | string | null
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

  url.search = params.toString()
  return url.toString()
}

export function createAiCoveHelpDocsSidecarUrl(): string {
  return new URL(AI_COVE_HELP_DOCS_SIDECAR_PATH, getSidecarOrigin()).toString()
}
