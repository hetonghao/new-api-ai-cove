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
import { useMemo } from 'react'
import { useTranslation } from 'react-i18next'
import { useAuthStore } from '@/stores/auth-store'
import { createAiCoveHelpDocsSidecarUrl } from '@/lib/ai-cove-sidecar-url'
import { parseHeaderNavModulesFromStatus } from '@/lib/nav-modules'
import { useStatus } from '@/hooks/use-status'

export type TopNavLink = {
  title: string
  href: string
  disabled?: boolean
  requiresAuth?: boolean
  external?: boolean
  newTab?: boolean
}

const NEW_API_DEFAULT_DOCS_ORIGINS = new Set([
  'https://docs.newapi.pro',
  'http://docs.newapi.pro',
])
const URI_SCHEME_PATTERN = /^[a-z][a-z\d+.-]*:/i
const AI_COVE_SIDECAR_PATH_PREFIX = '/sidecars/'

function trimTrailingSlash(value: string): string {
  return value.replace(/\/+$/, '')
}

function isNewApiDefaultDocsLink(value: string): boolean {
  try {
    const url = new URL(value)
    return NEW_API_DEFAULT_DOCS_ORIGINS.has(trimTrailingSlash(url.origin))
  } catch {
    return false
  }
}

function getLinkPathname(href: string): string {
  if (href.startsWith('/')) return href

  try {
    return new URL(href).pathname
  } catch {
    return ''
  }
}

function isAiCoveSidecarLink(href: string): boolean {
  return getLinkPathname(href).startsWith(AI_COVE_SIDECAR_PATH_PREFIX)
}

function isAiCoveHelpDocsLink(href: string): boolean {
  return getLinkPathname(href).startsWith('/sidecars/help-docs/')
}

function resolveDocsLink(docsLink: string | undefined): string {
  const normalized = docsLink?.trim()
  if (
    !normalized ||
    isNewApiDefaultDocsLink(normalized) ||
    isAiCoveHelpDocsLink(normalized)
  ) {
    return createAiCoveHelpDocsSidecarUrl()
  }
  return normalized
}

function isExternalLink(href: string): boolean {
  return isAiCoveSidecarLink(href) || URI_SCHEME_PATTERN.test(href)
}

function shouldOpenInNewTab(href: string): boolean {
  return URI_SCHEME_PATTERN.test(href) && !isAiCoveSidecarLink(href)
}

/**
 * Generate top navigation links based on HeaderNavModules configuration from backend /api/status
 * Backend format example (stringified JSON):
 * {
 *   home: true,
 *   console: true,
 *   pricing: { enabled: true, requireAuth: false },
 *   rankings: { enabled: true, requireAuth: false },
 *   docs: true,
 *   about: true
 * }
 */
export function useTopNavLinks(): TopNavLink[] {
  const { t } = useTranslation()
  const { status } = useStatus()
  const { auth } = useAuthStore()

  // Parse HeaderNavModules
  const modules = useMemo(() => {
    return parseHeaderNavModulesFromStatus(
      status as Record<string, unknown> | null
    )
  }, [status])

  // Documentation link (may be external)
  const docsLink: string | undefined = status?.docs_link as string | undefined

  const isAuthed = !!auth?.user

  const links: TopNavLink[] = []

  // Home
  if (modules?.home !== false) {
    links.push({ title: t('Home'), href: '/' })
  }

  // Console -> /dashboard (new console path)
  if (modules?.console !== false) {
    links.push({ title: t('Console'), href: '/dashboard' })
  }

  // Pricing
  const pricing = modules?.pricing
  if (pricing && typeof pricing === 'object' && pricing.enabled) {
    const requiresAuth = pricing.requireAuth && !isAuthed
    links.push({ title: t('Model Square'), href: '/pricing', requiresAuth })
  }

  // Rankings
  const rankings = modules?.rankings
  if (rankings && typeof rankings === 'object' && rankings.enabled) {
    const requiresAuth = rankings.requireAuth && !isAuthed
    links.push({ title: t('Rankings'), href: '/rankings', requiresAuth })
  }

  // Docs (supports external links)
  if (modules?.docs !== false) {
    const resolvedDocsLink = resolveDocsLink(docsLink)
    links.push({
      title: t('Docs'),
      href: resolvedDocsLink,
      external: isExternalLink(resolvedDocsLink),
      newTab: shouldOpenInNewTab(resolvedDocsLink),
    })
  }

  // About
  if (modules?.about !== false) {
    links.push({ title: t('About'), href: '/about' })
  }

  return links
}
