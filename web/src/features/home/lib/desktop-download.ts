export type DesktopDownloadPlatform = 'macos' | 'windows'

export type DesktopDownloadEnvironment = {
  userAgentDataPlatform?: string | null
  platform?: string | null
  userAgent?: string | null
}

export type DesktopDownloadTarget = {
  platform: DesktopDownloadPlatform
  href: string
  labelKey: string
  ariaLabelKey: string
}

const MACOS_DOWNLOAD_HREF = '/downloads/ai-cove-design-desktop-macos.dmg'
const WINDOWS_DOWNLOAD_HREF = '/downloads/ai-cove-design-desktop-windows.exe'
const TURBO_MACOS_DOWNLOAD_HREF =
  '/downloads/turbo/ai-cove-turbo-macos.dmg?v=0.1.0-beta.1-build.5'
const TURBO_WINDOWS_DOWNLOAD_HREF =
  '/downloads/turbo/ai-cove-turbo-windows.exe?v=0.1.0-beta.1-build.5'

function getDesktopDownloadVersion() {
  if (typeof __AI_COVE_DESIGN_DESKTOP_DOWNLOAD_VERSION__ !== 'string') {
    return ''
  }
  return __AI_COVE_DESIGN_DESKTOP_DOWNLOAD_VERSION__.trim()
}

export function withDesktopDownloadVersion(
  href: string,
  version = getDesktopDownloadVersion()
) {
  const normalizedVersion = version.trim()
  if (!normalizedVersion) return href
  return `${href}?v=${encodeURIComponent(normalizedVersion)}`
}

function getNavigatorEnvironment(): DesktopDownloadEnvironment {
  if (typeof navigator === 'undefined') {
    return {}
  }

  const navigatorWithUserAgentData = navigator as Navigator & {
    userAgentData?: { platform?: string }
  }

  return {
    userAgentDataPlatform: navigatorWithUserAgentData.userAgentData?.platform,
    platform: navigator.platform,
    userAgent: navigator.userAgent,
  }
}

function detectPlatformValue(value: string | null | undefined) {
  const normalized = value?.trim().toLowerCase()
  if (!normalized) return null
  if (normalized.includes('win')) return 'windows'
  if (normalized.includes('mac') || normalized.includes('darwin')) {
    return 'macos'
  }
  return null
}

export function detectDesktopDownloadPlatform(
  environment: DesktopDownloadEnvironment = getNavigatorEnvironment()
): DesktopDownloadPlatform {
  const detected =
    detectPlatformValue(environment.userAgentDataPlatform) ??
    detectPlatformValue(environment.platform) ??
    detectPlatformValue(environment.userAgent)

  return detected ?? 'macos'
}

export function getDesktopDownloadTarget(
  environment?: DesktopDownloadEnvironment
): DesktopDownloadTarget {
  const platform = detectDesktopDownloadPlatform(environment)

  if (platform === 'windows') {
    return {
      platform,
      href: withDesktopDownloadVersion(WINDOWS_DOWNLOAD_HREF),
      labelKey: 'Download AI Cove Design Windows desktop app',
      ariaLabelKey: 'Download AI Cove Design for Windows',
    }
  }

  return {
    platform,
    href: withDesktopDownloadVersion(MACOS_DOWNLOAD_HREF),
    labelKey: 'Download AI Cove Design macOS desktop app',
    ariaLabelKey: 'Download AI Cove Design for macOS',
  }
}

export function getTurboDesktopDownloadTarget(
  environment?: DesktopDownloadEnvironment
): DesktopDownloadTarget {
  const platform = detectDesktopDownloadPlatform(environment)

  if (platform === 'windows') {
    return {
      platform,
      href: TURBO_WINDOWS_DOWNLOAD_HREF,
      labelKey: 'Download AI Cove Turbo Windows desktop app',
      ariaLabelKey: 'Download AI Cove Turbo for Windows',
    }
  }

  return {
    platform,
    href: TURBO_MACOS_DOWNLOAD_HREF,
    labelKey: 'Download AI Cove Turbo macOS desktop app',
    ariaLabelKey: 'Download AI Cove Turbo for macOS',
  }
}
