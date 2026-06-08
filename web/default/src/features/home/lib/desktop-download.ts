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
      href: WINDOWS_DOWNLOAD_HREF,
      labelKey: 'Download Windows desktop app',
      ariaLabelKey: 'Download AI-Cove-Design for Windows',
    }
  }

  return {
    platform,
    href: MACOS_DOWNLOAD_HREF,
    labelKey: 'Download macOS desktop app',
    ariaLabelKey: 'Download AI-Cove-Design for macOS',
  }
}
