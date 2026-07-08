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
import { useEffect } from 'react'

import type { SystemStatusData } from '@/lib/api'
import { DEFAULT_SYSTEM_NAME, DEFAULT_LOGO } from '@/lib/constants'
import { applyFaviconToDom } from '@/lib/dom-utils'
import {
  useSystemConfigStore,
  type CurrencyConfig,
  type SystemConfig,
  DEFAULT_CURRENCY_CONFIG,
} from '@/stores/system-config-store'

function toNumber(value: unknown, fallback: number): number {
  if (typeof value === 'number' && !Number.isNaN(value)) return value
  if (typeof value === 'string') {
    const parsed = Number(value)
    if (!Number.isNaN(parsed)) return parsed
  }
  return fallback
}

/**
 * Map `/api/status` response data to our persisted system config structure
 */
export function mapStatusDataToConfig(
  data: SystemStatusData | null | undefined
): Partial<SystemConfig> {
  if (!data) return {}

  const currency: CurrencyConfig = {
    displayInCurrency: true,
    quotaDisplayType: 'USD',
    quotaPerUnit: toNumber(
      data.quota_per_unit,
      DEFAULT_CURRENCY_CONFIG.quotaPerUnit
    ),
    usdExchangeRate: 1,
    customCurrencySymbol: '$',
    customCurrencyExchangeRate: 1,
  }

  return {
    systemName: data.system_name || DEFAULT_SYSTEM_NAME,
    logo: data.logo || DEFAULT_LOGO,
    footerHtml:
      typeof data.footer_html === 'string' ? data.footer_html : undefined,
    demoSiteEnabled: data.demo_site_enabled,
    displayTokenStatEnabled: data.display_token_stat_enabled,
    currency,
  }
}

export function syncStatusToSystemConfig(
  data: SystemStatusData | null | undefined
): void {
  const { setConfig, setLoading } = useSystemConfigStore.getState()
  if (data) {
    setConfig(mapStatusDataToConfig(data))
  }
  setLoading(false)
}

// Preload image and return cleanup function
function preloadImage(
  src: string,
  onLoad: () => void,
  onError: () => void
): () => void {
  const img = new Image()
  img.addEventListener('load', onLoad)
  img.addEventListener('error', onError)
  img.src = src

  return () => {
    img.removeEventListener('load', onLoad)
    img.removeEventListener('error', onError)
  }
}

/**
 * System configuration hook with auto-loading and logo preloading
 */
export function useSystemConfig() {
  const {
    config,
    loading,
    loadedLogoUrl,
    setLoadedLogoUrl,
  } = useSystemConfigStore()

  // Preload logo image when URL changes
  useEffect(() => {
    const { logo } = config

    // Skip if logo is already loaded
    if (!logo || logo === loadedLogoUrl) return

    // Preload new logo
    return preloadImage(
      logo,
      () => {
        setLoadedLogoUrl(logo)
        applyFaviconToDom(logo)
      },
      () => {
        if (logo !== DEFAULT_LOGO) {
          // eslint-disable-next-line no-console
          console.error('Failed to load logo:', logo)
        }
        // Mark as loaded even on error to prevent infinite retry
        setLoadedLogoUrl(logo)
      }
    )
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [config.logo, loadedLogoUrl, setLoadedLogoUrl])

  return {
    ...config,
    loading,
    logoLoaded: config.logo === loadedLogoUrl && !!loadedLogoUrl,
  }
}
