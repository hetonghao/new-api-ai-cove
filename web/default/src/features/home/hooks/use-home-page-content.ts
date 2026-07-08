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
import { useEffect, useState } from 'react'

import { isHttpUrl } from '@/lib/content-format'

import { getHomePageContent } from '../api'
import { getInitialHomePageContentState } from '../lib/home-page-content-state'
import type { HomePageContentResult } from '../types'

const STORAGE_KEY = 'home_page_content'

function readCachedHomePageContent(): string | null {
  try {
    if (typeof window === 'undefined') return null
    return window.localStorage.getItem(STORAGE_KEY)
  } catch {
    return null
  }
}

/**
 * Hook to load and manage custom home page content
 * Supports both Markdown/HTML content and iframe URLs
 */
export function useHomePageContent(): HomePageContentResult {
  const [{ content, isLoaded }, setState] = useState(() =>
    getInitialHomePageContentState(readCachedHomePageContent())
  )

  useEffect(() => {
    let mounted = true

    const loadContent = async () => {
      try {
        const response = await getHomePageContent()
        const { success, data } = response

        if (!mounted) return

        if (success && data) {
          setState({ content: data, isLoaded: true })
          localStorage.setItem(STORAGE_KEY, data)
        } else {
          // Clear content if API returns empty
          setState({ content: '', isLoaded: true })
          localStorage.removeItem(STORAGE_KEY)
        }
      } catch (error) {
        if (!mounted) return
        // eslint-disable-next-line no-console
        console.error('Failed to load home page content:', error)
        const [{ toast }, i18next] = await Promise.all([
          import('sonner'),
          import('i18next'),
        ])
        if (!mounted) return
        toast.error(i18next.default.t('Failed to load home page content'))
      } finally {
        if (mounted) {
          setState((state) =>
            state.isLoaded ? state : { ...state, isLoaded: true }
          )
        }
      }
    }

    loadContent()

    return () => {
      mounted = false
    }
  }, [])

  const isUrl = isHttpUrl(content)

  return { content, isLoaded, isUrl }
}
